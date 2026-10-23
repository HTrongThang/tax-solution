// apitest: chương trình THỬ NGHIỆM độc lập, không liên quan tới bot hiện tại.
// Mục đích: kiểm tra xem có lấy được danh sách hóa đơn bằng cách gọi thẳng API
// của cổng thuế (không bấm giao diện) hay không, đo tốc độ và tần suất lỗi 504.
//
// Cách chạy (PowerShell), token lấy từ cookie "jwt" trong DevTools (Application > Cookies):
//
//	$env:TAX_JWT = "<giá trị cookie jwt>"
//	$env:TAX_FROM = "01/10/2026"      # dd/MM/yyyy
//	$env:TAX_TO   = "21/10/2026"      # dd/MM/yyyy
//	go run ./cmd/apitest
//
// Biến tùy chọn: TAX_COOKIE (chuỗi cookie đầy đủ nếu API đòi), TAX_MAX_PAGES (mặc định 50),
// TAX_PAGE_DELAY_MS (nghỉ giữa các trang, mặc định 1000), TAX_SIZE (mặc định 15).
// TAX_OUT (file JSON lưu toàn bộ dữ liệu thô lấy được, mặc định apitest_output.json).
// Token KHÔNG được ghi vào log hay vào file.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// Chỉ gọi đúng host của cổng thuế (không nhận URL từ người dùng).
	baseURL     = "https://hoadondientu.gdt.gov.vn/api/sco-query/invoices/sold"
	maxBodySize = 8 << 20 // giới hạn 8 MB mỗi phản hồi
)

// Chờ giữa các lần thử lại khi gặp 5xx / lỗi mạng (cùng một `state`).
var retryDelays = []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}

type invoice struct {
	NBMST    string `json:"nbmst"`
	KHMSHDon any    `json:"khmshdon"`
	KHHDon   string `json:"khhdon"`
	SHDon    any    `json:"shdon"`
}

type pageResp struct {
	Datas []json.RawMessage `json:"datas"`
	State string            `json:"state"`
	Time  int               `json:"time"`
	Total int               `json:"total"`
}

func main() {
	token := strings.TrimSpace(os.Getenv("TAX_JWT"))
	if token == "" {
		fatal("Thiếu biến môi trường TAX_JWT (giá trị cookie 'jwt').")
	}
	from, err := time.Parse("02/01/2006", os.Getenv("TAX_FROM"))
	if err != nil {
		fatal("TAX_FROM phải có dạng dd/MM/yyyy.")
	}
	to, err := time.Parse("02/01/2006", os.Getenv("TAX_TO"))
	if err != nil {
		fatal("TAX_TO phải có dạng dd/MM/yyyy.")
	}
	size := envInt("TAX_SIZE", 15)
	maxPages := envInt("TAX_MAX_PAGES", 50)
	pageDelay := time.Duration(envInt("TAX_PAGE_DELAY_MS", 1000)) * time.Millisecond
	cookie := os.Getenv("TAX_COOKIE")

	search := fmt.Sprintf("tdlap=ge=%s;tdlap=le=%s",
		from.Format("02/01/2006")+"T00:00:00",
		to.Format("02/01/2006")+"T23:59:59")

	client := &http.Client{Timeout: 70 * time.Second}

	seen := map[string]struct{}{}
	var all []json.RawMessage // dữ liệu thô của các hóa đơn (đã loại trùng) để xem/lưu
	state := ""
	total, minTotal, maxTotal := 0, 0, 0
	reachedEnd := false
	started := time.Now()
	retries, failures := 0, 0

	for page := 1; page <= maxPages; page++ {
		t0 := time.Now()
		resp, attempts, err := fetchWithRetry(client, token, cookie, search, size, state)
		retries += attempts
		if err != nil {
			failures++
			fmt.Printf("Trang %d: LỖI sau khi thử lại (%v). Dừng, state hiện tại được giữ để chạy tiếp.\n", page, err)
			break
		}
		if resp.Total > 0 {
			total = resp.Total
			if minTotal == 0 || total < minTotal {
				minTotal = total
			}
			if total > maxTotal {
				maxTotal = total
			}
		}

		added := 0
		for _, raw := range resp.Datas {
			var inv invoice
			if json.Unmarshal(raw, &inv) != nil {
				continue
			}
			key := fmt.Sprintf("%s|%v|%s|%v", inv.NBMST, inv.KHMSHDon, inv.KHHDon, inv.SHDon)
			if _, dup := seen[key]; !dup {
				seen[key] = struct{}{}
				all = append(all, raw)
				added++
			}
		}
		fmt.Printf("Trang %2d: %2d dòng, mới %2d, tổng đã có %3d/%d, %v (thử lại %d)\n",
			page, len(resp.Datas), added, len(seen), total, time.Since(t0).Round(time.Millisecond), attempts)

		// Hết dữ liệu thật sự: trang rỗng, trang ngắn hơn `size`, hoặc không còn con trỏ.
		// KHÔNG dựa vào `total` để dừng vì giá trị này dao động giữa các trang.
		if len(resp.Datas) < size || resp.State == "" || resp.State == state {
			reachedEnd = true
			break
		}
		state = resp.State
		time.Sleep(pageDelay)
	}

	fmt.Println("────────────────────────────────────────")
	saveAndPreview(all, os.Getenv("TAX_OUT"))
	fmt.Printf("Đã lấy: %d hóa đơn (sau khi loại trùng)\n", len(seen))
	fmt.Printf("`total` cổng báo dao động: %d → %d\n", minTotal, maxTotal)
	fmt.Printf("Tổng thời gian: %v | số lần thử lại: %d | số trang lỗi hẳn: %d\n",
		time.Since(started).Round(time.Second), retries, failures)
	if reachedEnd {
		fmt.Println("Đã đi tới trang cuối tự nhiên (hết dữ liệu).")
	} else {
		fmt.Println("CẢNH BÁO: chưa tới trang cuối (do lỗi hoặc giới hạn số trang) — dữ liệu CHƯA đầy đủ.")
	}
	if maxTotal != minTotal {
		fmt.Println("LƯU Ý: `total` không ổn định nên không dùng làm căn cứ duy nhất để kết luận đủ/thiếu.")
	}
}

// fetchWithRetry thử lại với CÙNG một `state` khi gặp 5xx hoặc lỗi mạng.
// Trả về số lần đã phải thử lại.
func fetchWithRetry(c *http.Client, token, cookie, search string, size int, state string) (*pageResp, int, error) {
	var lastErr error
	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		resp, status, err := fetchPage(c, token, cookie, search, size, state)
		if err == nil {
			return resp, attempt, nil
		}
		lastErr = err
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return nil, attempt, fmt.Errorf("bị từ chối: %v", err)
		}
		if attempt < len(retryDelays) {
			fmt.Printf("  ! %v — thử lại sau %v\n", err, retryDelays[attempt])
			time.Sleep(retryDelays[attempt])
		}
	}
	return nil, len(retryDelays), lastErr
}

func fetchPage(c *http.Client, token, cookie, search string, size int, state string) (*pageResp, int, error) {
	q := url.Values{}
	q.Set("sort", "tdlap:desc")
	q.Set("size", strconv.Itoa(size))
	if state != "" {
		q.Set("state", state)
	}
	q.Set("search", search)

	req, err := http.NewRequest(http.MethodGet, baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "vi")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Referer", "https://hoadondientu.gdt.gov.vn/tra-cuu/tra-cuu-hoa-don")
	req.Header.Set("Origin", "https://hoadondientu.gdt.gov.vn")
	req.Header.Set("End-Point", "/tra-cuu/tra-cuu-hoa-don")
	req.Header.Set("Request-Id", newRequestID())
	req.Header.Set("Authorization", "Bearer "+token)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	res, err := c.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("lỗi mạng: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxBodySize))
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("đọc phản hồi lỗi: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		snippet := strings.Join(strings.Fields(string(body)), " ")
		if len(snippet) > 160 {
			snippet = snippet[:160]
		}
		return nil, res.StatusCode, fmt.Errorf("HTTP %d [%s]", res.StatusCode, snippet)
	}

	var pr pageResp
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, res.StatusCode, errors.New("phản hồi không phải JSON hợp lệ")
	}
	return &pr, res.StatusCode, nil
}

// saveAndPreview in ra bản ghi đầu tiên (để thấy cổng trả về những trường nào) và
// lưu toàn bộ dữ liệu thô vào file JSON để mở xem. Dữ liệu chứa thông tin hóa đơn thật
// nên file chỉ cho chủ sở hữu đọc (0600).
func saveAndPreview(all []json.RawMessage, outPath string) {
	if len(all) == 0 {
		fmt.Println("Không có dữ liệu để hiển thị/lưu.")
		return
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, all[0], "", "  ") == nil {
		fmt.Printf("Mẫu bản ghi đầu tiên (%d bản ghi tổng cộng):\n%s\n", len(all), pretty.String())
	}
	if strings.TrimSpace(outPath) == "" {
		outPath = "apitest_output.json"
	}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		fmt.Printf("Không thể mã hóa dữ liệu để lưu: %v\n", err)
		return
	}
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		fmt.Printf("Không thể ghi file %s: %v\n", outPath, err)
		return
	}
	fmt.Printf("Đã lưu toàn bộ dữ liệu thô vào: %s\n", outPath)
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// newRequestID tạo chuỗi dạng UUID v4 cho header Request-Id (giống trình duyệt).
func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
