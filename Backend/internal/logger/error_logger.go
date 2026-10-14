package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var mu sync.Mutex

// GetLogDir trả về thư mục %AppData%\TaxSolution\logs\ chuẩn trên Windows
func GetLogDir() string {
	appData := os.Getenv("APPDATA")
	if appData != "" {
		return filepath.Join(appData, "TaxSolution", "logs")
	}
	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, "AppData", "Roaming", "TaxSolution", "logs")
	}
	return filepath.Join(".", "logs")
}

// GetErrorLogPath trả về đường dẫn đầy đủ tới file error.log
func GetErrorLogPath() string {
	return filepath.Join(GetLogDir(), "error.log")
}

// cleanField loại bỏ ký tự xuống dòng và ký tự | để đảm bảo log luôn đúng 1 dòng
func cleanField(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "-")
	return strings.TrimSpace(s)
}

// checkMonthlyCleanupInternal kiểm tra và xóa/làm sạch file nếu sang tháng mới, quá 30 ngày hoặc quá 10MB
func checkMonthlyCleanupInternal(filePath string) {
	info, err := os.Stat(filePath)
	if err != nil {
		return // File chưa tồn tại, không cần làm gì
	}

	now := time.Now()
	modTime := info.ModTime()

	// 1. Bước sang tháng mới (khác tháng hoặc khác năm)
	isNewMonth := now.Year() != modTime.Year() || now.Month() != modTime.Month()
	// 2. File đã tồn tại quá 30 ngày
	isOlderThan30Days := now.Sub(modTime) > 30*24*time.Hour
	// 3. File vượt quá 10MB (bảo vệ chống phình to bất thường)
	isOversized := info.Size() > 10*1024*1024

	if isNewMonth || isOlderThan30Days || isOversized {
		if errRem := os.Remove(filePath); errRem != nil {
			_ = os.Truncate(filePath, 0)
		}
	}
}

// CheckMonthlyCleanup gọi kiểm tra và dọn dẹp file log (thường gọi khi khởi động ứng dụng)
func CheckMonthlyCleanup() {
	mu.Lock()
	defer mu.Unlock()
	checkMonthlyCleanupInternal(GetErrorLogPath())
}

// LogError ghi 1 bản ghi lỗi chuẩn 1 hàng vào file error.log
// Format: [YYYY-MM-DD HH:mm:ss] [ERROR] MST: ... | Type: ... | Date: ... | Step: ... | Error: ... | Detail: ...
func LogError(mst, invoiceType, dateRange, step, errStr, detail string) {
	mu.Lock()
	defer mu.Unlock()

	if strings.TrimSpace(mst) == "" {
		mst = "N/A"
	}
	if strings.TrimSpace(invoiceType) == "" {
		invoiceType = "N/A"
	}
	if strings.TrimSpace(dateRange) == "" {
		dateRange = "N/A"
	}
	if strings.TrimSpace(step) == "" {
		step = "Unknown"
	}
	if strings.TrimSpace(errStr) == "" {
		errStr = "Unknown error"
	}
	if strings.TrimSpace(detail) == "" {
		detail = "None"
	}

	dir := GetLogDir()
	_ = os.MkdirAll(dir, 0755)

	filePath := filepath.Join(dir, "error.log")

	// Tự động kiểm tra và dọn dẹp nếu đã sang tháng mới trước khi ghi
	checkMonthlyCleanupInternal(filePath)

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("⚠️ Không thể mở file log lỗi: %v\n", err)
		return
	}
	defer f.Close()

	nowStr := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] [ERROR] MST: %s | Type: %s | Date: %s | Step: %s | Error: %s | Detail: %s\n",
		nowStr,
		cleanField(mst),
		cleanField(invoiceType),
		cleanField(dateRange),
		cleanField(step),
		cleanField(errStr),
		cleanField(detail),
	)

	_, _ = f.WriteString(line)
}

// LogErrorf tiện ích ghi lỗi với error và format detail
func LogErrorf(mst, invoiceType, dateRange, step string, err error, detailFormat string, a ...interface{}) {
	errStr := "Unknown error"
	if err != nil {
		errStr = err.Error()
	}
	detail := ""
	if detailFormat != "" {
		detail = fmt.Sprintf(detailFormat, a...)
	}
	LogError(mst, invoiceType, dateRange, step, errStr, detail)
}
