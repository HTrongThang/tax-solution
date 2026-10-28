package main

import (
	"fmt"
	"os"
	"path/filepath"

	"TAXSOLUTION/internal/api"
	"TAXSOLUTION/internal/captcha"
	"TAXSOLUTION/internal/crawler"
	"TAXSOLUTION/internal/logger"

	"github.com/gin-gonic/gin"
)

func main() {
	// Nạp biến môi trường từ .env
	envPaths := []string{".env", "../.env", "Backend/.env"}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		envPaths = append(envPaths,
			filepath.Join(exeDir, ".env"),
			filepath.Join(exeDir, "..", ".env"),
			filepath.Join(exeDir, "..", "..", ".env"),
		)
	}
	captcha.LoadEnv(envPaths...)
	fmt.Printf("🤖 Giải captcha bằng AI: enabled=%v, có GEMINI_API_KEY=%v\n", captcha.Enabled(), captcha.HasKey())

	// 0. Tự động kiểm tra dọn dẹp file log nếu đã sang tháng mới hoặc quá hạn
	logger.CheckMonthlyCleanup()

	// 1. Khởi tạo service
	crawlerSvc := crawler.NewCrawlerService()

	// 2. Khởi tạo handler
	apiHandler := api.NewHandler(crawlerSvc)

	// 3. Khởi tạo Gin
	r := gin.Default()

	// 4. GỌI SETUP ROUTER TỪ PACKAGE API
	api.SetupRouter(r, apiHandler)

	// 5. Chạy server trên loopback nội bộ cổng 18386 để tránh bị chiếm dụng
	port := os.Getenv("PORT")
	if port == "" {
		port = "18386"
	}
	r.Run("127.0.0.1:" + port)
}
