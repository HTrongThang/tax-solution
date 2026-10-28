package api

import (
	"TAXSOLUTION/internal/auth"
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// authService tạo muộn (lần gọi đầu) để chắc chắn .env đã được nạp xong.
var authService = sync.OnceValue(func() *auth.Service {
	return auth.NewService(auth.NewFromEnv())
})

// registerAuthRoutes đăng ký các API xác thực tài khoản phần mềm (nhân viên).
func registerAuthRoutes(r *gin.Engine) {
	registerAuthRoutesWith(r, authService)
}

func registerAuthRoutesWith(r *gin.Engine, svc func() *auth.Service) {
	g := r.Group("/api/auth")
	{
		g.GET("/hwid", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"hwid": auth.HardwareID()}})
		})
		g.POST("/login", func(c *gin.Context) { authLogin(c, svc()) })
		g.POST("/restore", func(c *gin.Context) {
			var req struct {
				Username       string `json:"username"`
				FullName       string `json:"fullName"`
				MaxTaxAccounts int    `json:"maxTaxAccounts"`
				AllowAI        bool   `json:"allowAI"`
				EndDate        string `json:"endDate"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
				return
			}
			svc().RestoreSession(req.Username, req.FullName, req.MaxTaxAccounts, req.AllowAI, req.EndDate)
			c.JSON(http.StatusOK, gin.H{"status": "success", "result": "OK"})
		})
		g.POST("/register", func(c *gin.Context) { authRegister(c, svc()) })
		g.POST("/check", func(c *gin.Context) { authCheck(c, svc()) })
		g.POST("/logout", func(c *gin.Context) {
			svc().Logout()
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
		g.POST("/admin/create", func(c *gin.Context) { authAdminCreate(c, svc()) })
		g.GET("/session", func(c *gin.Context) {
			sess := svc().Current()
			if sess == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "result": "NO_SESSION", "message": "Chưa đăng nhập."})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": sessionData(sess)})
		})
	}
}

func authLogin(c *gin.Context, svc *auth.Service) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "result": string(auth.ResultBadRequest), "message": "Dữ liệu JSON không đúng định dạng"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	resp, err := svc.Login(ctx, req.Username, req.Password)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	writeAuthResponse(c, resp, svc)
}

func authRegister(c *gin.Context, svc *auth.Service) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		FullName string `json:"fullName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "result": string(auth.ResultBadRequest), "message": "Dữ liệu JSON không đúng định dạng"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	resp, err := svc.Register(ctx, auth.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		FullName: req.FullName,
	})
	if err != nil {
		if auth.IsValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "result": string(auth.ResultBadRequest), "message": err.Error()})
			return
		}
		writeAuthError(c, err)
		return
	}

	if resp.Result == auth.ResultOK {
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"result":  "OK",
			"message": "Đã gửi yêu cầu cấp tài khoản! Vui lòng liên hệ Admin (Zalo: 0356.587.127) để được kích hoạt.",
		})
		return
	}
	c.JSON(authHTTPStatus(resp.Result), gin.H{"status": "error", "result": string(resp.Result), "message": auth.Describe(resp)})
}

func authCheck(c *gin.Context, svc *auth.Service) {
	var req struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req) // body có thể trống

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	resp, err := svc.Check(ctx, req.Force)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	writeAuthResponse(c, resp, svc)
}

// authAdminCreate: Founder cấp tài khoản nhân viên (ghi lên Google Sheet, trạng thái ACTIVE).
// adminToken là SHA-256 mật khẩu Founder; script đối chiếu với FOUNDER_HASH.
func authAdminCreate(c *gin.Context, svc *auth.Service) {
	var req struct {
		AdminToken     string `json:"adminToken"`
		Username       string `json:"username"`
		Password       string `json:"password"`
		FullName       string `json:"fullName"`
		HWID           string `json:"hwid"`
		MaxTaxAccounts *int   `json:"maxTaxAccounts"`
		AllowAI        bool   `json:"allowAI"`
		StartDate      string `json:"startDate"`
		EndDate        string `json:"endDate"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "result": string(auth.ResultBadRequest), "message": "Dữ liệu JSON không đúng định dạng"})
		return
	}
	maxTax := 5 // mặc định
	if req.MaxTaxAccounts != nil {
		maxTax = *req.MaxTaxAccounts
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	resp, err := svc.AdminCreate(ctx, auth.AdminCreateRequest{
		AdminToken: req.AdminToken, Username: req.Username, Password: req.Password,
		FullName: req.FullName, HWID: req.HWID, MaxTaxAccounts: maxTax, AllowAI: req.AllowAI,
		StartDate: req.StartDate, EndDate: req.EndDate,
	})
	if err != nil {
		if auth.IsValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "result": string(auth.ResultBadRequest), "message": err.Error()})
			return
		}
		writeAuthError(c, err)
		return
	}

	switch resp.Result {
	case auth.ResultOK:
		c.JSON(http.StatusOK, gin.H{"status": "success", "result": "OK", "message": "Đã cấp tài khoản thành công."})
	case auth.ResultUnauthorized:
		// Script từ chối: sai khóa API hoặc sai mật khẩu Founder
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "result": string(resp.Result), "message": "Không có quyền cấp tài khoản (sai khóa API hoặc mật khẩu Founder)."})
	default:
		c.JSON(authHTTPStatus(resp.Result), gin.H{"status": "error", "result": string(resp.Result), "message": auth.Describe(resp)})
	}
}

func writeAuthResponse(c *gin.Context, resp *auth.Response, svc *auth.Service) {
	out := gin.H{
		"result":  string(resp.Result),
		"message": auth.Describe(resp),
	}
	if resp.Result == auth.ResultOK {
		out["status"] = "success"
		if sess := svc.Current(); sess != nil {
			out["data"] = sessionData(sess)
		}
		c.JSON(http.StatusOK, out)
		return
	}
	out["status"] = "error"
	c.JSON(authHTTPStatus(resp.Result), out)
}

func writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrNoSession):
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "result": "NO_SESSION", "message": "Chưa đăng nhập."})
	case errors.Is(err, auth.ErrNotConfigured):
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "result": "NOT_CONFIGURED", "message": "Phần mềm chưa được cấu hình máy chủ xác thực."})
	case errors.Is(err, auth.ErrNetwork):
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "result": "NETWORK_ERROR", "message": "Cần kết nối Internet để sử dụng phần mềm. Vui lòng kiểm tra mạng và thử lại."})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "result": "ERROR", "message": "Có lỗi xảy ra, vui lòng thử lại sau."})
	}
}

func authHTTPStatus(r auth.Result) int {
	switch r {
	case auth.ResultWrongCredentials:
		return http.StatusUnauthorized
	case auth.ResultPending, auth.ResultLocked, auth.ResultWrongDevice, auth.ResultNotStarted, auth.ResultExpired:
		return http.StatusForbidden
	case auth.ResultExists, auth.ResultBadRequest:
		return http.StatusBadRequest
	case auth.ResultUnauthorized:
		return http.StatusInternalServerError // sai khóa API: lỗi cấu hình, không phải lỗi của người dùng
	default:
		return http.StatusBadGateway
	}
}

func sessionData(s *auth.Session) gin.H {
	return gin.H{
		"username":       s.Username,
		"fullName":       s.FullName,
		"maxTaxAccounts": s.MaxTaxAccounts,
		"allowAI":        s.AllowAI,
		"endDate":        s.EndDate,
		"hwid":           auth.HardwareID(),
	}
}

