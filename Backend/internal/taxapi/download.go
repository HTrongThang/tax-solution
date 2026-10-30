package taxapi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// incompleteMarker: file đánh dấu thư mục tải chưa đủ, chứa chữ ký danh sách hóa đơn để lần tải sau tải bù đúng thư mục này.
const incompleteMarker = ".incomplete"

// Signature: chữ ký của danh sách hóa đơn (theo thứ tự + tên file). Cùng danh sách → cùng chữ ký.
func Signature(items []Item) string {
	h := sha1.New()
	for i, it := range items {
		fmt.Fprintln(h, it.FileName(i+1))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// FindResumeDir tìm thư mục tải dở gần nhất trong parent (tên bắt đầu bằng prefix) có cùng danh sách hóa đơn; rỗng nếu không có.
func FindResumeDir(parent, prefix string, items []Item) string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return ""
	}
	sig := Signature(items)
	var names []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names))) // tên chứa ngày giờ → mới nhất trước
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(parent, n, incompleteMarker))
		if err == nil && strings.TrimSpace(string(b)) == sig {
			return filepath.Join(parent, n)
		}
	}
	return ""
}

// Nghỉ nhẹ giữa các lần tải để cổng ít chặn hơn.
const downloadGap = 300 * time.Millisecond

// FileName trả về tên gốc (không đuôi) theo quy ước của app: "04_1_C26MBK_4123_01102026".
// stt: số thứ tự dòng (bắt đầu từ 1).
func (it Item) FileName(stt int) string {
	return fmt.Sprintf("%02d_%s_%s_%s_%s", stt, numStr(it.KHMSHDon), it.KHHDon, numStr(it.SHDon), it.dateVN("02012006"))
}

// DownloadXML tải file zip của một hóa đơn (theo nhóm API src) và lưu .xml/.html vào dir, đặt tên theo FileName.
// Trả về danh sách file đã lưu.
func (c *Client) DownloadXML(ctx context.Context, src Source, it Item, stt int, dir string) ([]string, error) {
	q := url.Values{}
	q.Set("nbmst", it.NBMST)
	q.Set("khhdon", it.KHHDon)
	q.Set("shdon", numStr(it.SHDon))
	q.Set("khmshdon", numStr(it.KHMSHDon))

	primarySrc := it.Source
	if primarySrc == "" {
		primarySrc = src
	}
	if primarySrc == "" {
		primarySrc = MTT
	}
	// Hóa đơn có ttxly == 8 là hóa đơn khởi tạo từ máy tính tiền → ưu tiên endpoint /sco-query
	if it.TTXLy == 8 {
		primarySrc = MTT
	}

	tryFetch := func(s Source) (map[string][]byte, error) {
		body, err := c.get(ctx, string(s)+"/invoices/export-xml", q, false)
		if err != nil {
			return nil, err
		}
		return unzipInvoice(body)
	}

	files, err := tryFetch(primarySrc)
	if err != nil && ctx.Err() == nil {
		// Thử chéo sang nhánh còn lại (/query <-> /sco-query) nếu danh sách lẫn cả 2 loại hoặc phản hồi không phải zip
		altSrc := HDDT
		if primarySrc == HDDT {
			altSrc = MTT
		}
		if altFiles, altErr := tryFetch(altSrc); altErr == nil {
			files, err = altFiles, nil
		}
	}
	if err != nil {
		return nil, err
	}
	base := it.FileName(stt)
	var saved []string
	for ext, data := range files {
		out := filepath.Join(dir, base+ext)
		if err := os.WriteFile(out, data, 0o600); err != nil {
			return saved, fmt.Errorf("không ghi được %s: %w", filepath.Base(out), err)
		}
		saved = append(saved, out)
	}
	return saved, nil
}

// maxDownloadRounds: số lượt tải lại tối đa cho các hóa đơn còn lỗi (lượt 1 là lượt tải đầu tiên).
const maxDownloadRounds = 30 // ~30 phút kiên nhẫn chờ cổng hết chặn; người dùng vẫn bấm "Hủy" để dừng sớm

// roundGaps: nghỉ trước mỗi lượt tải lại để cổng hết quá tải/chặn tạm thời.
var roundGaps = []time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second, 60 * time.Second}

// maxAdaptiveGap: nhịp nghỉ tối đa giữa 2 hóa đơn khi cổng liên tục từ chối.
const maxAdaptiveGap = 6 * time.Second

// DownloadAll tải XML cho toàn bộ items vào dir. Hóa đơn lỗi được tải lại ở các lượt sau
// (tối đa maxDownloadRounds lượt) cho tới khi thành công hết. progress được gọi sau mỗi lần thử
// với done = ok + số hóa đơn đang lỗi (luôn ≤ total, lỗi = done - ok); trả về số hóa đơn tải thành công.
func (c *Client) DownloadAll(ctx context.Context, src Source, items []Item, dir string, progress func(done, ok, total int)) (int, error) {
	if len(items) == 0 {
		return 0, ErrNoData
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, fmt.Errorf("không tạo được thư mục %s: %w", dir, err)
	}
	total := len(items)
	ok := 0
	defer func() {
		// Chưa đủ (lỗi/hủy) → đánh dấu để lần sau tải bù; đủ rồi → gỡ dấu.
		marker := filepath.Join(dir, incompleteMarker)
		if ok < total {
			_ = os.WriteFile(marker, []byte(Signature(items)), 0o600)
		} else {
			_ = os.Remove(marker)
		}
	}()
	gap := downloadGap            // tự giãn khi lỗi, co lại khi thành công
	pending := make([]int, total) // chỉ số các hóa đơn cần tải (giữ nguyên STT khi tải lại)
	for i := range pending {
		pending[i] = i
	}

	noProgressRounds := 0
	var lastRoundErr error
	for round := 1; round <= maxDownloadRounds && len(pending) > 0; round++ {
		if round > 1 {
			wait := roundGaps[min(round-2, len(roundGaps)-1)]
			c.logf("↻ Lượt %d/%d: tải lại %d hóa đơn lỗi sau %v", round, maxDownloadRounds, len(pending), wait)
			if !sleep(ctx, wait) {
				return ok, ctx.Err()
			}
		}
		okBeforeRound := ok
		var failed []int
		for n, i := range pending {
			if err := ctx.Err(); err != nil {
				return ok, err
			}
			it := items[i]
			if st, err := os.Stat(filepath.Join(dir, it.FileName(i+1)+".xml")); err == nil && st.Size() > 0 {
				ok++ // đã có từ lần tải trước (tải bù) → không gọi lại cổng thuế
				if progress != nil {
					untried := 0
					if round > 1 {
						untried = len(pending) - n - 1
					}
					progress(ok+len(failed)+untried, ok, total)
				}
				continue
			}
			if _, err := c.DownloadXML(ctx, src, it, i+1, dir); err != nil {
				lastRoundErr = err
				c.logf("[%s] LỖI (lượt %d): %v", it.FileName(i+1), round, err)
				failed = append(failed, i)
				gap = min(gap*2, maxAdaptiveGap) // bị từ chối → giãn nhịp
			} else {
				ok++
				gap = max(gap/2, downloadGap)
			}
			if progress != nil {
				// đã thử xong n+1 hóa đơn của lượt này; số còn lỗi = failed + chưa thử trong lượt
				// Lượt 1: done = đã thử xong. Lượt tải lại: các hóa đơn chưa thử lại vẫn đang là lỗi.
				untried := 0
				if round > 1 {
					untried = len(pending) - n - 1
				}
				progress(ok+len(failed)+untried, ok, total)
			}
			if !sleep(ctx, gap) {
				return ok, ctx.Err()
			}
		}
		pending = failed
		if len(pending) > 0 {
			if ok == okBeforeRound {
				noProgressRounds++
				// Nếu 2 lượt liên tiếp không tải thêm được hóa đơn nào thì dừng sớm tránh treo 30 phút
				if noProgressRounds >= 2 {
					c.logf("⚠️ Dừng tải lại sau %d lượt liên tiếp không có tiến triển (còn %d hóa đơn lỗi)", noProgressRounds, len(pending))
					break
				}
			} else {
				noProgressRounds = 0
			}
		}
	}
	if len(pending) > 0 {
		c.logf("⚠️ Còn %d hóa đơn không tải được", len(pending))
		if ok == 0 && lastRoundErr != nil {
			return 0, lastRoundErr
		}
	}
	return ok, nil
}

// unzipInvoice trích file .xml và .html đầu tiên trong zip (khóa kết quả là đuôi file).
// Chỉ đọc nội dung vào bộ nhớ, không dùng tên file trong zip để ghi đĩa (chống zip-slip).
func unzipInvoice(data []byte) (map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("phản hồi không phải file zip hợp lệ (%d byte)", len(data))
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		ext := strings.ToLower(filepath.Ext(filepath.Base(f.Name)))
		if f.FileInfo().IsDir() || (ext != ".xml" && ext != ".html") {
			continue
		}
		if _, have := out[ext]; have {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxBodySize))
		rc.Close()
		if err != nil {
			continue
		}
		out[ext] = b
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("zip không chứa file .xml/.html")
	}
	return out, nil
}
