package api

import (
	"TAXSOLUTION/internal/captcha"
	"TAXSOLUTION/internal/crawler"
	"TAXSOLUTION/internal/models"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	Crawler *crawler.CrawlerService
}

func NewHandler(c *crawler.CrawlerService) *Handler {
	return &Handler{Crawler: c}
}

// --- ĐẶT HÀM SETUPROUTER Ở ĐÂY ---
func SetupRouter(r *gin.Engine, h *Handler) {
	taxRoutes := r.Group("/api/tax")
	{
		taxRoutes.GET("/init", h.InitTaxPage)
		taxRoutes.POST("/login", h.LoginTax)
		taxRoutes.GET("/captcha", h.GetCaptcha)
		taxRoutes.GET("/searchInvoice", h.GetInvoices)
		taxRoutes.POST("/start-crawl", h.StartCrawl)
		taxRoutes.GET("/status", h.CheckCrawlStatus)
		taxRoutes.POST("/logout", h.LogoutTax)
		taxRoutes.POST("/shutdown", h.ShutdownTax)
		taxRoutes.POST("/stop", h.StopTax)
		taxRoutes.GET("/showLogin", h.ShowLoginDialog)
		taxRoutes.POST("/search-captcha", h.SubmitSearchCaptcha)
		taxRoutes.POST("/search-captcha/refresh", h.RefreshSearchCaptcha)
		taxRoutes.GET("/captcha-ai", h.GetCaptchaAI)
		taxRoutes.POST("/captcha-ai", h.SetCaptchaAI)
	}
}

// GetCaptchaAI: trả về trạng thái công tắc giải captcha bằng AI.
func (h *Handler) GetCaptchaAI(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"enabled": captcha.Enabled()})
}

// SetCaptchaAI: bật/tắt giải captcha bằng AI (lưu vào .env).
func (h *Handler) SetCaptchaAI(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Dữ liệu không hợp lệ"})
		return
	}
	if err := captcha.SetEnabled(req.Enabled); err != nil {
		fmt.Printf("⚠️ Không ghi được .env: %v\n", err)
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "enabled": captcha.Enabled()})
}

func (h *Handler) InitTaxPage(c *gin.Context) {
	// Gọi hàm Init mới của Thu
	err := h.Crawler.ShowLogin()

	if err != nil {
		c.JSON(500, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Trình duyệt đã mở và hiện Dialog đăng nhập!",
	})
}

// ShowLoginDialog: Mở dialog đăng nhập trên tab hiện tại (sau logout)
func (h *Handler) ShowLoginDialog(c *gin.Context) {
	err := h.Crawler.ShowLoginDialog()
	if err != nil {
		c.JSON(500, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Dialog đăng nhập đã hiện!",
	})
}

func (h *Handler) GetCaptcha(c *gin.Context) {
	// 1. Gọi hàm Crawler
	base64Data, err := h.Crawler.GetCaptcha()

	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status:  "error",
			Message: "Không thể lấy Captcha: " + err.Error(),
		})
		return
	}

	if !captcha.Enabled() {
		c.JSON(http.StatusOK, models.Response{
			Status:  "success",
			Message: "Đã lấy mã captcha thành công!",
			Data:    base64Data,
		})
		return
	}

	// 2. Tự động nhận diện trước bằng Gemini AI nếu có thể
	var aiCode string
	cleanBase64 := base64Data
	if idx := strings.Index(cleanBase64, ","); idx != -1 {
		cleanBase64 = cleanBase64[idx+1:]
	}
	if imgBytes, errDec := base64.StdEncoding.DecodeString(cleanBase64); errDec == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		solved, errSolve := captcha.SolveCaptchaImage(ctx, imgBytes)
		cancel()
		if errSolve == nil && len(solved) == 6 {
			aiCode = solved
			fmt.Printf("🤖 [GetCaptcha] Hệ thống đã giải trước mã: %s\n", aiCode)
		} else {
			fmt.Printf("⚠️ [GetCaptcha] Hệ thống chưa giải được mã: %v\n", errSolve)
		}
	}

	// 3. Trả về cho FE (kèm CaptchaCode để prefill nếu FE cần)
	c.JSON(http.StatusOK, models.Response{
		Status:      "success",
		Message:     "Đã lấy mã captcha thành công!",
		Data:        base64Data,
		CaptchaCode: aiCode,
	})
}
func (h *Handler) LoginTax(c *gin.Context) {
	var req models.LoginRequest

	// 1. Nhận JSON
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status:  "error",
			Message: "Dữ liệu JSON không đúng định dạng",
		})
		return
	}

	// --- BẮT ĐẦU ĐO Ở ĐÂY ---
	startTime := time.Now()
	fmt.Printf("---> [GO] Bắt đầu quá trình Login cho MST: %s\n", req.TaxCode)

	// 2. Gọi hàm Login (Bước giả lập tốn thời gian nhất)
	err := h.Crawler.Login(req.TaxCode, req.Password, req.Captcha)

	// --- KẾT THÚC ĐO ---
	duration := time.Since(startTime)
	fmt.Printf("---> [GO] Hoàn tất Login sau: %v\n", duration)
	// ----------------------

	// 3. Xử lý trả lỗi về FE
	if err != nil {
		errStr := err.Error()
		statusType := "error"

		if strings.Contains(errStr, "SAI_CAPTCHA") {
			statusType = "error_captcha"
		} else if strings.Contains(errStr, "SAI_THONG_TIN") {
			statusType = "error_credentials"
		}

		c.JSON(http.StatusUnauthorized, models.Response{
			Status:  statusType,
			Message: errStr,
		})
		return
	}

	// 4. Nếu thành công
	c.JSON(http.StatusOK, models.Response{
		Status:  "success",
		Message: "Đăng nhập thành công!",
	})
}
func (h *Handler) GetInvoices(c *gin.Context) {
	fromDate := c.Query("from")
	toDate := c.Query("to")
	invoiceType := c.DefaultQuery("type", "ban_ra")
	category := c.DefaultQuery("category", "dien_tu")
	checkResult := c.DefaultQuery("check_result", "")

	if fromDate == "" || toDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Thiếu ngày tháng"})
		return
	}

	// Deduplication: Từ chối nếu đang có tác vụ search/crawl chạy
	if h.Crawler.IsRunning {
		c.JSON(http.StatusConflict, gin.H{
			"message": "Hệ thống đang bận xử lý tác vụ khác. Vui lòng đợi hoàn thành trước.",
		})
		return
	}

	// Headers SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	streamChan := make(chan models.StreamData, 5)
	ctx := c.Request.Context()

	go func() {
		defer close(streamChan)
		// Gọi crawler (truyền ctx để hỗ trợ ngắt kết nối)
		_, _ = h.Crawler.SearchAndGetInvoices(ctx, fromDate, toDate, invoiceType, category, checkResult, streamChan)
	}()

	// Đẩy dữ liệu về Client
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
func (h *Handler) StartCrawl(c *gin.Context) {
	// 1. Kiểm tra trạng thái Bot (Phải có trình duyệt mới cào được)
	if h.Crawler.Page == nil {
		c.JSON(http.StatusBadRequest, models.CrawlResponse{
			Status:  "error",
			Message: "Trình duyệt chưa được khởi tạo! Vui lòng gọi /init và /login trước.",
		})
		return
	}

	// 2. Đọc output_dir, mst, invoice_type từ JSON body
	var req struct {
		Path        string `json:"path"`
		MST         string `json:"mst"`
		InvoiceType string `json:"invoice_type"`
	}

	outputDir := ""
	mst := ""
	invoiceType := "BanRa"
	if err := c.ShouldBindJSON(&req); err == nil {
		outputDir = req.Path
		mst = req.MST
		// Chuyển đổi invoice_type thành tên đẹp cho thư mục
		switch req.InvoiceType {
		case "mua_vao":
			invoiceType = "MuaVao"
		default:
			invoiceType = "BanRa"
		}
	}

	// Nếu không có giá trị hợp lệ, mới dùng fallback
	if outputDir == "" {
		outputDir = "C:\\TaxData\\Downloads"
		fmt.Println("⚠️ Không có output_dir, sử dụng mặc định: " + outputDir)
	}

	// 3. TẠO THƯ MỤC CON theo format: MST_ChieuHD_NgayGio
	timeStr := time.Now().Format("20060102_150405")
	subFolder := fmt.Sprintf("%s_%s_%s", mst, invoiceType, timeStr)
	if mst == "" {
		subFolder = fmt.Sprintf("%s_%s", invoiceType, timeStr)
	}
	finalDir := filepath.Join(outputDir, subFolder)

	// Tự động tạo thư mục (Quan trọng)
	// Giúp Bot không bị crash nếu thư mục truyền vào chưa có trên ổ cứng
	err := os.MkdirAll(finalDir, os.ModePerm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.CrawlResponse{
			Status:  "error",
			Message: "Không thể tạo thư mục lưu trữ: " + err.Error(),
		})
		return
	}

	// 4. Gán đường dẫn vào Crawler để các hàm con (ExtractAndMove) sử dụng
	h.Crawler.OutputDir = finalDir

	// 5. KÍCH HOẠT CHẠY NGẦM (GOROUTINE)
	// Giải phóng cho Postman/ .NET ngay lập tức
	go func() {
		fmt.Printf("🤖 Hệ thống dụng bắt đầu làm việc ngầm. Kết quả sẽ lưu tại: %s\n", finalDir)

		// Gọi hàm cào đa trang của Thắng
		h.Crawler.CrawlAllPages()

		// Lượt tải không có file nào (lỗi/dừng/không có dữ liệu) → xóa thư mục rỗng. os.Remove chỉ xóa được thư mục rỗng nên không thể mất dữ liệu.
		if errRm := os.Remove(finalDir); errRm == nil {
			fmt.Printf("🧹 Đã xóa thư mục rỗng: %s\n", finalDir)
		}

		fmt.Println("🏁 Hệ thống đã hoàn thành toàn bộ hóa đơn!")
	}()

	// 6. Trả lời ngay cho người gọi
	c.JSON(http.StatusOK, models.CrawlResponse{
		Status:    "success",
		Message:   "Tiến trình cào dữ liệu đã bắt đầu chạy ngầm.",
		TaskID:    fmt.Sprintf("TASK_%d", time.Now().Unix()), // Tạo một ID để dễ theo dõi log
		OutputDir: finalDir,
	})
}
func (h *Handler) CheckCrawlStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"is_running":     h.Crawler.IsRunning,
		"current_status": h.Crawler.CurrentStatus,
		"total_invoices": h.Crawler.TotalInvoices,
	})
}

// StopTax: Dừng tiến trình cào hoặc tìm kiếm
func (h *Handler) StopTax(c *gin.Context) {
	h.Crawler.Stop()
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Đã gửi lệnh dừng tiến trình.",
	})
}

func (h *Handler) LogoutTax(c *gin.Context) {
	// Soft logout: đăng xuất trang thuế nhưng GIỮ Chrome mở
	err := h.Crawler.SoftLogout()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Không thể đăng xuất: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Đã đăng xuất trang thuế. Bấm Đăng nhập để chọn công ty tiếp!",
	})
}

// ShutdownTax: Đóng hoàn toàn Chrome. Gọi khi thoát app.
func (h *Handler) ShutdownTax(c *gin.Context) {
	err := h.Crawler.Shutdown()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Không thể đóng trình duyệt: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Đã đóng trình duyệt hoàn toàn.",
	})
}
