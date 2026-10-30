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
// Mặc định bật (hoặc cấu hình TAX_MODE=api trong .env; đặt TAX_MODE=chrome để tắt).
// Hỗ trợ:
//   - Tab "Hóa đơn điện tử" (/api/query/invoices/sold & /purchase) kèm toàn bộ bộ lọc Kết quả kiểm tra (ttxly==0..8).
//   - Tab "Từ máy tính tiền" (/api/sco-query/invoices/sold & /purchase) cả Bán ra và Mua vào.

// apiState là trạng thái của chế độ API: kết quả tìm kiếm gần nhất (để tải XML) và
// hàm hủy tác vụ đang chạy.
type apiState struct {
	mu     sync.Mutex
	items  []taxapi.Item
	kind   string        // "ban_ra" / "mua_vao" của lần tìm gần nhất
	source taxapi.Source // taxapi.HDDT hoặc taxapi.MTT
	cancel context.CancelFunc
}

func (s *apiState) setItems(items []taxapi.Item, kind string, src taxapi.Source) {
	s.mu.Lock()
	s.items, s.kind, s.source = items, kind, src
	s.mu.Unlock()
}

func (s *apiState) getItems(kind string) ([]taxapi.Item, taxapi.Source) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) == 0 {
		return nil, ""
	}
	return append([]taxapi.Item(nil), s.items...), s.source
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

// apiModeOn: có bật chế độ API không (mặc định bật, trừ khi đặt TAX_MODE=chrome/off/false).
func apiModeOn() bool {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("TAX_MODE")))
	switch mode {
	case "chrome", "ui", "off", "false", "0":
		return false
	default:
		return true
	}
}

// apiSearchSupported: yêu cầu tìm kiếm này có thể chạy bằng API không.
func apiSearchSupported(invoiceType, category, checkResult string) bool {
	if !apiModeOn() {
		return false
	}
	if invoiceType != "ban_ra" && invoiceType != "mua_vao" {
		return false
	}
	if category == "may_tinh_tien" {
		// Tab Máy tính tiền (/sco-query) yêu cầu giải captcha từng trang bằng AI.
		// Nếu tài khoản không bật AI thì dùng luồng Chrome để hiện popup nhập tay.
		return canUseAI()
	}
	// Tab Hóa đơn điện tử (/query): hỗ trợ tất cả Kết quả kiểm tra (Tất cả, ttxly==0..8).
	return category == "dien_tu" || category == ""
}

func (h *Handler) newAPIClient() *taxapi.Client {
	cl := taxapi.New(h.Crawler)
	cl.Logf = func(format string, args ...any) { fmt.Printf("🌐 [API] "+format+"\n", args...) }
	return cl
}

// apiSearch: tìm hóa đơn bằng API và đẩy kết quả về giao diện qua SSE (cùng định dạng luồng cũ).
func (h *Handler) apiSearch(c *gin.Context, fromDate, toDate, invoiceType, category, checkResult string) {
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
	h.api.setItems(nil, "", "")

	h.Crawler.IsRunning = true
	h.Crawler.StopRequested = false
	h.Crawler.CurrentStatus = "Đang lấy danh sách hóa đơn..."

	streamChan := make(chan models.StreamData, 5)
	go func() {
		defer close(streamChan)
		defer cancel()
		defer func() { h.Crawler.IsRunning = false }()

		kind := taxapi.Sold
		if invoiceType == "mua_vao" {
			kind = taxapi.Purchase
		}
		src := taxapi.HDDT
		if category == "may_tinh_tien" {
			src = taxapi.MTT
		}

		var ttxlyPtr *int
		if src == taxapi.HDDT {
			if code, ok := taxapi.ParseCheckResult(checkResult); ok {
				ttxlyPtr = &code
			} else if kind == taxapi.Purchase {
				// Tab Mua vào bắt buộc có Kết quả kiểm tra (*) -> mặc định 5 (Đã cấp mã hóa đơn)
				defCode := 5
				ttxlyPtr = &defCode
			}
		}

		items, err := h.newAPIClient().ListInvoices(ctx, taxapi.ListParams{
			From:   from,
			To:     to,
			Kind:   kind,
			Source: src,
			TTXLy:  ttxlyPtr,
		}, streamChan)
		if err != nil {
			fmt.Printf("⚠️ [API] Lấy danh sách lỗi: %v\n", err)
		}
		// Lưu danh sách hóa đơn đã lấy được để nút tải XML / Tải chi tiết sử dụng ngay
		if len(items) > 0 {
			h.api.setItems(items, invoiceType, src)
			h.Crawler.TotalInvoices = len(items)
		}
	}()

	// Lắng nghe request context (client ngắt kết nối) chứ KHÔNG phải ctx nội bộ: khi tìm xong goroutine
	// gọi cancel() ngay, nếu select theo ctx đó thì tin nhắn IsDone cuối nằm trong kênh có thể bị bỏ rơi
	// và giao diện cứ quay "Đang tìm kiếm" mãi. Kênh đóng sau khi goroutine xong nên vòng lặp vẫn kết thúc.
	reqCtx := c.Request.Context()
	c.Stream(func(w io.Writer) bool {
		select {
		case <-reqCtx.Done():
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
func (h *Handler) apiDownloadReady(invoiceType string) ([]taxapi.Item, taxapi.Source) {
	if !apiModeOn() {
		return nil, ""
	}
	return h.api.getItems(invoiceType)
}

// apiDownload: tải XML của danh sách đã tìm vào finalDir (chạy ngầm). Giao diện theo dõi
// qua /api/tax/status như luồng cũ.
func (h *Handler) apiDownload(items []taxapi.Item, src taxapi.Source, finalDir string) {
	ctx, cancel := context.WithCancel(context.Background())
	h.api.setCancel(cancel)
	// Đặt trạng thái chạy NGAY (đồng bộ) trước khi trả response: C# poll /status tức thì, nếu để trong
	// goroutine có thể thấy is_running=false và kết thúc luôn → "không tải được".
	h.Crawler.IsRunning = true
	h.Crawler.StopRequested = false
	h.Crawler.TotalInvoices = 0
	h.Crawler.CurrentStatus = fmt.Sprintf("Đang tải XML 0/%d...", len(items))
	go func() {
		defer cancel()
		fmt.Printf("🌐 [API] Bắt đầu tải XML %d hóa đơn (nhánh %s) vào: %s\n", len(items), src, finalDir)

		ok, err := h.newAPIClient().DownloadAll(ctx, src, items, finalDir, func(done, ok, total int) {
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
