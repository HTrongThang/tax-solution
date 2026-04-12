package api

import (
	"TAXSOLUTION/internal/crawler"
	"TAXSOLUTION/internal/models"
	"fmt"
	"net/http"
	"os"
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
		taxRoutes.POST("/crawl", h.StartCrawling)
		taxRoutes.GET("/captcha", h.GetCaptcha)
		taxRoutes.GET("/searchInvoice", h.GetInvoices)
		taxRoutes.POST("/start-crawl", h.StartCrawl)
		taxRoutes.GET("/status", h.CheckCrawlStatus)
	}
}

func (h *Handler) StartCrawling(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Đang bắt đầu cào dữ liệu..."})
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
func (h *Handler) GetCaptcha(c *gin.Context) {
	// 1. Gọi hàm Crawler của Thu
	// Biến base64Data sẽ nhận giá trị "encoded" mà hàm GetCaptcha trả về
	base64Data, err := h.Crawler.GetCaptcha()

	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status:  "error",
			Message: "Không thể lấy Captcha: " + err.Error(),
		})
		return
	}

	// 2. Trả về cho .NET
	// Thu bỏ base64Data vào trường Data của model Response
	c.JSON(http.StatusOK, models.Response{
		Status:  "success",
		Message: "Đã lấy mã captcha thành công!",
		Data:    base64Data, // .NET sẽ đọc cái này để hiện ảnh lên PictureBox
	})
}
func (h *Handler) LoginTax(c *gin.Context) {
	var req models.LoginRequest

	// 1. Nhận JSON từ .NET/Postman
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status:  "error",
			Message: "Dữ liệu JSON không đúng định dạng",
		})
		return
	}

	// 2. Gọi hàm Login và hứng kết quả lỗi
	err := h.Crawler.Login(req.TaxCode, req.Password, req.Captcha)

	// 3. Xử lý trả lỗi về FE
	if err != nil {
		errStr := err.Error()
		statusType := "error"

		// Kiểm tra xem có phải lỗi Captcha không để báo hiệu cho .NET
		if strings.Contains(errStr, "SAI_CAPTCHA") {
			statusType = "error_captcha"
		}

		c.JSON(http.StatusUnauthorized, models.Response{
			Status:  statusType, // Trả về mã loại lỗi riêng
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

	// 1. Lấy dữ liệu từ URL (Query Params)
	fromDate := c.Query("from") // .NET gửi: ?from=01/01/2026
	toDate := c.Query("to")     // .NET gửi: &to=31/01/2026

	// Kiểm tra xem có truyền ngày vào không
	if fromDate == "" || toDate == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status:  "error",
			Message: "Vui lòng cung cấp đầy đủ từ ngày và đến ngày (dd/mm/yyyy)",
		})
		return
	}

	// 2. Gọi Crawler xử lý
	invoices, err := h.Crawler.SearchAndGetInvoices(fromDate, toDate)

	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status:  "error",
			Message: "Lỗi khi tra cứu: " + err.Error(),
		})
		return
	}

	// 3. Trả kết quả về cho .NET
	c.JSON(http.StatusOK, models.Response{
		Status:  "success",
		Message: fmt.Sprintf("Tìm thấy %d hóa đơn", len(invoices)),
		Data:    invoices,
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

	// 2. Lấy đường dẫn từ Postman (Query Param: output_dir)
	// Nếu Thắng gọi: .../start-crawl?output_dir=D:\MyInvoices
	outputDir := c.Query("output_dir")

	// Nếu Thắng không truyền gì, lúc này mới dùng giá trị mặc định
	if outputDir == "" {
		outputDir = "C:\\TaxData\\Downloads"
		fmt.Println("⚠️ Không có output_dir, sử dụng mặc định: " + outputDir)
	}

	// 3. TỰ ĐỘNG TẠO THƯ MỤC (Quan trọng)
	// Giúp Bot không bị crash nếu thư mục Thắng truyền vào chưa có trên ổ cứng
	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.CrawlResponse{
			Status:  "error",
			Message: "Không thể tạo thư mục lưu trữ: " + err.Error(),
		})
		return
	}

	// 4. Gán đường dẫn vào Crawler để các hàm con (ExtractAndMove) sử dụng
	h.Crawler.OutputDir = outputDir

	// 5. KÍCH HOẠT CHẠY NGẦM (GOROUTINE)
	// Giải phóng cho Postman/ .NET ngay lập tức
	go func() {
		fmt.Printf("🤖 Bot bắt đầu làm việc ngầm. Kết quả sẽ lưu tại: %s\n", outputDir)

		// Gọi hàm cào đa trang của Thắng
		h.Crawler.CrawlAllPages()

		fmt.Println("🏁 Bot đã hoàn thành toàn bộ hóa đơn!")
	}()

	// 6. Trả lời ngay cho người gọi
	c.JSON(http.StatusOK, models.CrawlResponse{
		Status:  "success",
		Message: "Tiến trình cào dữ liệu đã bắt đầu chạy ngầm.",
		TaskID:  fmt.Sprintf("TASK_%d", time.Now().Unix()), // Tạo một ID để dễ theo dõi log
	})
}
func (h *Handler) CheckCrawlStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"is_running":     h.Crawler.IsRunning,
		"current_status": h.Crawler.CurrentStatus,
		"total_invoices": h.Crawler.TotalInvoices,
	})
}
