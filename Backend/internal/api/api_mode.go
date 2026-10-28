package api

import (
	"TAXSOLUTION/internal/models"
	"TAXSOLUTION/internal/taxapi"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Chế độ gọi thẳng API cổng thuế (thay cho điều khiển giao diện bằng Chrome).
// Bật bằng biến môi trường / .env:  TAX_MODE=api
// Hiện hỗ trợ: hóa đơn có mã khởi tạo từ máy tính tiền (MTT) BÁN RA, không lọc theo kết quả kiểm tra.
// Tab "Hóa đơn điện tử" (endpoint /query) chưa kiểm chứng nên vẫn dùng luồng Chrome.
// Các trường hợp khác tự dùng luồng Chrome như cũ.

// apiState là trạng thái của chế độ API: kết quả tìm kiếm gần nhất (để tải XML) và
// hàm hủy tác vụ đang chạy.
type apiState struct {
	mu     sync.Mutex
	items  []taxapi.Item
	kind   string // "ban_ra" / "mua_vao" của lần tìm gần nhất
	cancel context.CancelFunc
}

func (s *apiState) setItems(items []taxapi.Item, kind string) {
	s.mu.Lock()
	s.items, s.kind = items, kind
	s.mu.Unlock()
}

func (s *apiState) getItems(kind string) []taxapi.Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kind != kind {
		return nil
	}
	return append([]taxapi.Item(nil), s.items...)
}

func (s *apiState) setCancel(c context.CancelFunc) {
	s.mu.Lock()
	s.cancel = c
	s.mu.Unlock()
}

// stop hủy tác vụ API đang chạy (nếu có).
func (s *apiState) stop() {
	s.mu.Lock()
	c := s.cancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

// apiModeOn: có bật chế độ API không.
func apiModeOn() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TAX_MODE")), "api")
}

// apiSearchSupported: yêu cầu tìm kiếm này có thể chạy bằng API không.
func apiSearchSupported(invoiceType, category, checkResult string) bool {
	noFilter := strings.TrimSpace(checkResult) == "" ||
		strings.EqualFold(strings.TrimSpace(checkResult), "Tất cả")
	return apiModeOn() &&
		invoiceType == "ban_ra" &&
		category == "may_tinh_tien" &&
		noFilter
}

func (h *Handler) newAPIClient() *taxapi.Client {
	cl := taxapi.New(h.Crawler)
	cl.Logf = func(format string, args ...any) { fmt.Printf("🌐 [API] "+format+"\n", args...) }
	return cl
}

// apiSearch: tìm hóa đơn bằng API và đẩy kết quả về giao diện qua SSE (cùng định dạng luồng cũ).
func (h *Handler) apiSearch(c *gin.Context, fromDate, toDate, invoiceType string) {
	from, err1 := time.Parse("02/01/2006", fromDate)
	to, err2 := time.Parse("02/01/2006", toDate)
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Ngày phải có dạng dd/MM/yyyy"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	ctx, cancel := context.WithCancel(c.Request.Context())
	h.api.setCancel(cancel)
	h.api.setItems(nil, "")

	h.Crawler.IsRunning = true
	h.Crawler.StopRequested = false
	h.Crawler.CurrentStatus = "Đang lấy danh sách hóa đơn..."

	streamChan := make(chan models.StreamData, 5)
	go func() {
		defer close(streamChan)
		defer cancel()
		defer func() { h.Crawler.IsRunning = false }()
		items, err := h.newAPIClient().ListInvoices(ctx, taxapi.ListParams{From: from, To: to, Kind: taxapi.Sold, Source: taxapi.MTT}, streamChan)
		if err != nil {
			fmt.Printf("⚠️ [API] Lấy danh sách lỗi: %v\n", err)
		}
		// Chỉ nhớ danh sách khi lấy trọn vẹn, để nút tải XML không tải thiếu mà không báo.
		if err == nil {
			h.api.setItems(items, invoiceType)
			h.Crawler.TotalInvoices = len(items)
		}
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case data, ok := <-streamChan:
			if !ok {
				return false
			}
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			return true
		}
	})
}

// apiDownloadReady: có danh sách từ lần tìm bằng API để tải XML không.
func (h *Handler) apiDownloadReady(invoiceType string) []taxapi.Item {
	if !apiModeOn() {
		return nil
	}
	return h.api.getItems(invoiceType)
}

// apiDownload: tải XML của danh sách đã tìm vào finalDir (chạy ngầm). Giao diện theo dõi
// qua /api/tax/status như luồng cũ.
func (h *Handler) apiDownload(items []taxapi.Item, finalDir string) {
	ctx, cancel := context.WithCancel(context.Background())
	h.api.setCancel(cancel)
	go func() {
		defer cancel()
		h.Crawler.IsRunning = true
		h.Crawler.StopRequested = false
		h.Crawler.TotalInvoices = 0
		h.Crawler.CurrentStatus = fmt.Sprintf("Đang tải XML 0/%d...", len(items))
		fmt.Printf("🌐 [API] Bắt đầu tải XML %d hóa đơn vào: %s\n", len(items), finalDir)

		ok, err := h.newAPIClient().DownloadAll(ctx, taxapi.MTT, items, finalDir, func(done, ok, total int) {
			h.Crawler.TotalInvoices = ok
			if failed := done - ok; failed > 0 {
				h.Crawler.CurrentStatus = fmt.Sprintf("Đã tải %d/%d (%d hóa đơn lỗi, tự động tải lại)...", ok, total, failed)
			} else {
				h.Crawler.CurrentStatus = fmt.Sprintf("Đang tải XML %d/%d...", done, total)
			}
		})

		switch {
		case err != nil && ctx.Err() != nil:
			h.Crawler.CurrentStatus = fmt.Sprintf("Đã dừng (tải được %d/%d)", ok, len(items))
		case err != nil:
			h.Crawler.CurrentStatus = "Lỗi: " + err.Error()
		default:
			if ok < len(items) {
				h.Crawler.CurrentStatus = fmt.Sprintf("Hoàn tất: tải %d/%d hóa đơn (còn %d hóa đơn không tải được, xem log)", ok, len(items), len(items)-ok)
			} else {
				h.Crawler.CurrentStatus = fmt.Sprintf("Hoàn tất: tải %d/%d hóa đơn", ok, len(items))
			}
		}
		fmt.Printf("🌐 [API] %s\n", h.Crawler.CurrentStatus)

		// Thư mục của lượt tải này nếu rỗng thì xóa (os.Remove chỉ xóa được thư mục rỗng).
		if errRm := os.Remove(finalDir); errRm == nil {
			fmt.Printf("🧹 Đã xóa thư mục rỗng: %s\n", finalDir)
		}
		h.Crawler.IsRunning = false
	}()
}
