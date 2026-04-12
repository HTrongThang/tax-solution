package main

import (
	"TAXSOLUTION/internal/api"
	"TAXSOLUTION/internal/crawler"

	"github.com/gin-gonic/gin" // Cần thêm cái này
)

func main() {
	// 1. Khởi tạo service
	crawlerSvc := crawler.NewCrawlerService()

	// 2. Khởi tạo handler
	apiHandler := api.NewHandler(crawlerSvc)

	// 3. Khởi tạo Gin
	r := gin.Default()

	// 4. GỌI SETUP ROUTER TỪ PACKAGE API
	api.SetupRouter(r, apiHandler)

	// 5. Chạy server
	r.Run(":8080")
}
