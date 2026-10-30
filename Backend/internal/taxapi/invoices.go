package taxapi

import (
	"TAXSOLUTION/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Loại danh sách hóa đơn.
type Kind string

const (
	Sold     Kind = "sold"     // bán ra
	Purchase Kind = "purchase" // mua vào
)

// Source là nhóm API: hóa đơn có mã khởi tạo từ máy tính tiền (MTT) hay hóa đơn điện tử (HDDT).
type Source string

const (
	MTT  Source = "/sco-query" // tab "Hóa đơn có mã khởi tạo từ máy tính tiền"
	HDDT Source = "/query"     // tab "Hóa đơn điện tử"
)

// ListParams là điều kiện lấy danh sách hóa đơn.
type ListParams struct {
	From, To  time.Time     // theo ngày lập, tính cả ngày đầu và ngày cuối
	Kind      Kind          // Sold / Purchase
	Source    Source        // MTT (/sco-query) hoặc HDDT (/query)
	TTXLy     *int          // lọc theo kết quả kiểm tra (ttxly==0..8); nil = Tất cả
	Size      int           // số dòng mỗi trang (mặc định 15)
	PageDelay time.Duration // nghỉ giữa các trang
}

type listResp struct {
	Datas []json.RawMessage `json:"datas"`
	State string            `json:"state"`
	Total int               `json:"total"`
}

const maxPages = 500

const (
	// maxRangeDaysMTT: cổng thuế chỉ cho tìm tối đa 10 ngày mỗi lần với hóa đơn MTT (/sco-query).
	maxRangeDaysMTT = 10
	// maxRangeDaysHDDT: hóa đơn điện tử (/query) cho phép tìm cả tháng (~30-31 ngày) mỗi lần.
	maxRangeDaysHDDT = 30
)

// ListInvoices lấy toàn bộ hóa đơn trong khoảng ngày, phân trang theo `state`.
// Mỗi trang xong sẽ đẩy một models.StreamData vào stream (nếu stream != nil); cuối cùng
// đẩy thêm một bản IsDone=true. KHÔNG dựa vào `total` để dừng vì giá trị này dao động.
func (c *Client) ListInvoices(ctx context.Context, p ListParams, stream chan<- models.StreamData) ([]Item, error) {
	if p.Size <= 0 {
		p.Size = 50
	}
	if p.Kind == "" {
		p.Kind = Sold
	}
	if p.Source == "" {
		p.Source = MTT
	}
	if p.PageDelay <= 0 {
		if p.Source == HDDT {
			p.PageDelay = 300 * time.Millisecond
		} else {
			p.PageDelay = time.Second
		}
	}
	endpoint := string(p.Source) + "/invoices/" + string(p.Kind)
	withCaptcha := p.Source == MTT

	maxDays := maxRangeDaysMTT
	if p.Source == HDDT {
		maxDays = maxRangeDaysHDDT
	}
	ranges := splitDateRange(p.From, p.To, maxDays)

	send := func(sd models.StreamData) {
		if stream == nil {
			return
		}
		select {
		case stream <- sd:
		case <-ctx.Done():
		}
	}

	seen := map[string]struct{}{}
	var items []Item
	total := 0
	totalAll := 0            // tổng theo cổng thuế cộng dồn qua các đoạn đã chạy xong
	var all []models.Invoice // tích lũy mọi trang, gửi kèm tin IsDone để giao diện nhận đủ dữ liệu
	pageNo := 0

	for ri, rg := range ranges {
		search := fmt.Sprintf("tdlap=ge=%sT00:00:00;tdlap=le=%sT23:59:59",
			rg[0].Format("02/01/2006"), rg[1].Format("02/01/2006"))
		if p.TTXLy != nil {
			search = fmt.Sprintf("%s;ttxly==%d", search, *p.TTXLy)
		}
		rangeLabel := fmt.Sprintf("đoạn %d/%d (%s - %s)", ri+1, len(ranges),
			rg[0].Format("02/01"), rg[1].Format("02/01"))
		state := ""
		total = 0
		finished := false

		for page := 1; page <= maxPages; page++ {
			pageNo++
			q := url.Values{}
			q.Set("sort", "tdlap:desc")
			q.Set("size", strconv.Itoa(p.Size))
			if state != "" {
				q.Set("state", state)
			}
			q.Set("search", search)

			body, err := c.get(ctx, endpoint, q, withCaptcha)
			if err != nil {
				send(models.StreamData{CurrentPage: pageNo, TotalSoFar: len(items), TotalOnTax: totalAll + total,
					Status: "Lỗi lấy dữ liệu: " + err.Error(), IsDone: true})
				return items, fmt.Errorf("%s trang %d: %w", rangeLabel, page, err)
			}
			var resp listResp
			if err := json.Unmarshal(body, &resp); err != nil {
				return items, fmt.Errorf("%s trang %d: phản hồi không phải JSON hợp lệ", rangeLabel, page)
			}
			if resp.Total > 0 {
				total = resp.Total
			}

			var batch []models.Invoice
			for _, raw := range resp.Datas {
				it, ok := parseItem(raw)
				if !ok {
					continue
				}
				if _, dup := seen[it.Key()]; dup {
					continue
				}
				seen[it.Key()] = struct{}{}
				if it.TTXLy == 8 {
					it.Source = MTT
				} else {
					it.Source = p.Source
				}
				items = append(items, it)
				batch = append(batch, it.ToInvoice(len(items), p.Kind))
			}
			c.logf("%s - Trang %d: %d dòng, mới %d, tổng %d", rangeLabel, page, len(resp.Datas), len(batch), len(items))
			all = append(all, batch...)

			last := len(resp.Datas) < p.Size || resp.State == "" || resp.State == state
			lastRange := ri == len(ranges)-1
			send(models.StreamData{
				CurrentPage: pageNo, TotalSoFar: len(items), TotalOnTax: totalAll + total,
				Status: fmt.Sprintf("Đã lấy %d hóa đơn (%s)", len(items), rangeLabel),
			})
			if last {
				finished = true
				if lastRange {
					send(models.StreamData{Invoices: all, CurrentPage: pageNo, TotalSoFar: len(items), TotalOnTax: totalAll + total,
						Status: fmt.Sprintf("Hoàn tất: %d hóa đơn", len(items)), IsDone: true})
					return items, nil
				}
				break
			}
			state = resp.State
			if !sleep(ctx, p.PageDelay) {
				return items, ctx.Err()
			}
		}
		if !finished {
			return items, fmt.Errorf("%s: vượt quá %d trang, dừng để tránh vòng lặp vô hạn", rangeLabel, maxPages)
		}
		totalAll += total
		// Nghỉ giữa các đoạn để tránh bị cổng thuế giới hạn tốc độ.
		if !sleep(ctx, p.PageDelay) {
			return items, ctx.Err()
		}
	}
	return items, nil
}

// splitDateRange chia [from, to] (tính cả hai đầu) thành các đoạn liên tiếp, mỗi đoạn tối đa n ngày.
// Nếu from > to thì trả về một đoạn duy nhất để cổng thuế tự báo lỗi.
func splitDateRange(from, to time.Time, n int) [][2]time.Time {
	if n <= 0 || from.After(to) {
		return [][2]time.Time{{from, to}}
	}
	var out [][2]time.Time
	for start := from; !start.After(to); start = start.AddDate(0, 0, n) {
		end := start.AddDate(0, 0, n-1)
		if end.After(to) {
			end = to
		}
		out = append(out, [2]time.Time{start, end})
	}
	return out
}
