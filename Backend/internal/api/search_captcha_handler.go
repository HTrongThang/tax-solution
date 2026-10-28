package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"TAXSOLUTION/internal/crawler"

	"github.com/gin-gonic/gin"
)

// Mã captcha cổng thuế chỉ gồm chữ và số; giới hạn độ dài để chặn dữ liệu bất thường.
var searchCaptchaCodePattern = regexp.MustCompile(`^[A-Za-z0-9]{1,12}$`)

// SubmitSearchCaptcha: nhận mã captcha người dùng nhập trên app cho lần tìm kiếm hóa đơn MTT đang chờ.
func (h *Handler) SubmitSearchCaptcha(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Dữ liệu gửi lên không hợp lệ"})
		return
	}
	code := strings.TrimSpace(req.Code)
	if !searchCaptchaCodePattern.MatchString(code) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Mã captcha chỉ gồm chữ và số"})
		return
	}

	if err := h.Crawler.SubmitSearchCaptcha(code); err != nil {
		status := http.StatusConflict
		if errors.Is(err, crawler.ErrNoSearchCaptchaPending) {
			status = http.StatusGone
		}
		c.JSON(status, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Đã gửi mã captcha"})
}

// RefreshSearchCaptcha: bấm "đổi mã" trên popup captcha của cổng thuế và trả ảnh mới (data URL).
func (h *Handler) RefreshSearchCaptcha(c *gin.Context) {
	img, err := h.Crawler.RefreshSearchCaptcha()
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, crawler.ErrNoSearchCaptchaPending) {
			status = http.StatusGone
		}
		c.JSON(status, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": img})
}
