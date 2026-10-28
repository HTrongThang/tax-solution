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
	Purchase Kind = "purchase" // mua vào (endpoint này chưa được kiểm chứng trên cổng thật)
)

// Source là nhóm API: hóa đơn có mã khởi tạo từ máy tính tiền (MTT) hay hóa đơn điện tử.
type Source string

const (
	MTT  Source = "/sco-query" // tab "Hóa đơn có mã khởi tạo từ máy tính tiền" (đã kiểm chứng)
	HDDT Source = "/query"     // tab "Hóa đơn điện tử" (CHƯA kiểm chứng)
)

// ListParams là điều kiện lấy danh sách hóa đơn.
type ListParams struct {
	From, To  time.Time // theo ngày lập, tính cả ngày đầu và ngày cuối
	Kind      Kind
	Source    Source        // mặc định MTT
	Size      int           // số dòng mỗi trang (mặc định 15)
	PageDelay time.Duration // nghỉ giữa các trang (mặc định 1s)
}

type listResp struct {
	Datas []json.RawMessage `json:"datas"`
	State string            `json:"state"`
	Total int               `json:"total"`
}

const maxPages = 500

// ListInvoices lấy toàn bộ hóa đơn trong khoảng ngày, phân trang theo `state`.
// Mỗi trang xong sẽ đẩy một models.StreamData vào stream (nếu stream != nil); cuối cùng
// đẩy thêm một bản IsDone=true. KHÔNG dựa vào `total` để dừng vì giá trị này dao động.
func (c *Client) ListInvoices(ctx context.Context, p ListParams, stream chan<- models.StreamData) ([]Item, error) {
	if p.Size <= 0 {
		p.Size = 15
	}
	if p.PageDelay <= 0 {
		p.PageDelay = time.Second
	}
	if p.Kind == "" {
		p.Kind = Sold
	}
	if p.Source == "" {
		p.Source = MTT
	}
	search := fmt.Sprintf("tdlap=ge=%sT00:00:00;tdlap=le=%sT23:59:59",
		p.From.Format("02/01/2006"), p.To.Format("02/01/2006"))
	endpoint := string(p.Source) + "/invoices/" + string(p.Kind)

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
	state := ""
	total := 0

	for page := 1; page <= maxPages; page++ {
		q := url.Values{}
		q.Set("sort", "tdlap:desc")
		q.Set("size", strconv.Itoa(p.Size))
		if state != "" {
			q.Set("state", state)
		}
		q.Set("search", search)

		body, err := c.get(ctx, endpoint, q, true)
		if err != nil {
			send(models.StreamData{CurrentPage: page, TotalSoFar: len(items), TotalOnTax: total,
				Status: "Lỗi lấy dữ liệu: " + err.Error(), IsDone: true})
			return items, fmt.Errorf("trang %d: %w", page, err)
		}
		var resp listResp
		if err := json.Unmarshal(body, &resp); err != nil {
			return items, fmt.Errorf("trang %d: phản hồi không phải JSON hợp lệ", page)
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
			items = append(items, it)
			batch = append(batch, it.ToInvoice(len(items)))
		}
		c.logf("Trang %d: %d dòng, mới %d, tổng %d/%d", page, len(resp.Datas), len(batch), len(items), total)

		last := len(resp.Datas) < p.Size || resp.State == "" || resp.State == state
		send(models.StreamData{
			Invoices: batch, CurrentPage: page, TotalSoFar: len(items), TotalOnTax: total,
			Status: fmt.Sprintf("Đã lấy %d/%d hóa đơn", len(items), total),
		})
		if last {
			send(models.StreamData{CurrentPage: page, TotalSoFar: len(items), TotalOnTax: total,
				Status: fmt.Sprintf("Hoàn tất: %d hóa đơn", len(items)), IsDone: true})
			return items, nil
		}
		state = resp.State
		if !sleep(ctx, p.PageDelay) {
			return items, ctx.Err()
		}
	}
	return items, fmt.Errorf("vượt quá %d trang, dừng để tránh vòng lặp vô hạn", maxPages)
}
