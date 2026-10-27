package captcha

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultModel = "gemini-3.5-flash-lite"
	prompt       = "Extract ONLY the 6-character alphanumeric CAPTCHA code inside the gray box " +
		"(ignore the label 'Mã captcha', strikethrough lines, and refresh icon). " +
		"Return ONLY the 6 uppercase characters, no spaces."
)

var fallbackModels = []string{
	"gemini-3.5-flash-lite",
	"gemini-3.1-flash-lite",
	"gemini-3.5-flash",
}

var nonAlnum = regexp.MustCompile(`[^A-Z0-9]`)

// ErrDisabled được trả về khi người dùng tắt giải captcha bằng AI.
var ErrDisabled = errors.New("giải captcha bằng hệ thống đang tắt (CAPTCHA_AI_ENABLED=false)")

// Enabled cho biết có dùng AI để giải captcha hay không.
// Bật/tắt bằng CAPTCHA_AI_ENABLED trong .env (mặc định: bật nếu có GEMINI_API_KEY).
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CAPTCHA_AI_ENABLED"))) {
	case "false", "0", "off", "no":
		return false
	}
	return strings.TrimSpace(os.Getenv("GEMINI_API_KEY")) != ""
}

// envFile là file .env đầu tiên đọc được, dùng để lưu lại công tắc AI khi người dùng đổi trên giao diện.
var (
	envFile string
	envMu   sync.Mutex
)

// SetEnabled bật/tắt giải captcha bằng AI ngay lập tức và ghi lại vào .env để giữ sau khi khởi động lại.
func SetEnabled(enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	_ = os.Setenv("CAPTCHA_AI_ENABLED", val)

	envMu.Lock()
	defer envMu.Unlock()
	if envFile == "" {
		return nil
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	found := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "CAPTCHA_AI_ENABLED=") {
			lines[i] = "CAPTCHA_AI_ENABLED=" + val
			found = true
		}
	}
	if !found {
		lines = append(lines, "CAPTCHA_AI_ENABLED="+val)
	}
	return os.WriteFile(envFile, []byte(strings.Join(lines, "\n")), 0o600)
}

// LoadEnv đọc các file .env và nạp vào os.Environ nếu chưa được set.
func LoadEnv(paths ...string) {
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		if envFile == "" {
			envFile = p
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, `"'`)
				if _, exists := os.LookupEnv(key); !exists {
					_ = os.Setenv(key, val)
				}
			}
		}
		f.Close()
	}
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("Gemini API error (%d): %s", e.Status, e.Body)
}

func isQuotaExhausted(err error) bool {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusTooManyRequests && strings.Contains(ae.Body, "RESOURCE_EXHAUSTED")
	}
	return false
}

func isRetryable(err error) bool {
	var ae *apiError
	// Nếu hết hẳn quota trong ngày (RESOURCE_EXHAUSTED) thì không retry model đó nữa, chuyển model khác
	if isQuotaExhausted(err) {
		return false
	}
	return errors.As(err, &ae) && (ae.Status == http.StatusServiceUnavailable || ae.Status == http.StatusTooManyRequests)
}

// SolveCaptchaImage giải mã một ảnh captcha (raw bytes hoặc base64).
// Tự động retry hoặc chuyển sang model dự phòng nếu model chính hết quota.
func SolveCaptchaImage(ctx context.Context, imgBytes []byte) (string, error) {
	if !Enabled() {
		return "", ErrDisabled
	}
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", errors.New("chưa cấu hình GEMINI_API_KEY trong .env")
	}

	configuredModel := os.Getenv("GEMINI_MODEL")
	if configuredModel == "" {
		configuredModel = defaultModel
	}

	// Danh sách các model để thử (bắt đầu bằng model cấu hình, sau đó là các model dự phòng)
	modelsToTry := []string{configuredModel}
	for _, m := range fallbackModels {
		if m != configuredModel {
			modelsToTry = append(modelsToTry, m)
		}
	}

	mimeType := http.DetectContentType(imgBytes)
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/png"
	}

	payload := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": prompt},
					map[string]any{"inline_data": map[string]any{
						"mime_type": mimeType,
						"data":      base64.StdEncoding.EncodeToString(imgBytes),
					}},
				},
			},
		},
		"generationConfig": map[string]any{"temperature": 0},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("lỗi đóng gói JSON: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	var lastErr error

	for _, currentModel := range modelsToTry {
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", currentModel)
		delay := time.Second
		const maxAttempts = 3

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			cleaned, err := func() (string, error) {
				reqCtx, reqCancel := context.WithTimeout(ctx, 12*time.Second)
				defer reqCancel()

				req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
				if err != nil {
					return "", err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("x-goog-api-key", apiKey)

				resp, err := client.Do(req)
				if err != nil {
					return "", err
				}
				defer resp.Body.Close()

				respBody, rErr := io.ReadAll(resp.Body)
				if rErr != nil {
					return "", rErr
				}
				if resp.StatusCode != http.StatusOK {
					return "", &apiError{Status: resp.StatusCode, Body: string(respBody)}
				}

				var gr geminiResponse
				if err := json.Unmarshal(respBody, &gr); err != nil {
					return "", fmt.Errorf("lỗi giải mã JSON Gemini: %w", err)
				}
				if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
					return "", errors.New("Gemini không trả về kết quả")
				}
				raw := gr.Candidates[0].Content.Parts[0].Text
				return nonAlnum.ReplaceAllString(strings.ToUpper(raw), ""), nil
			}()

			if err == nil {
				return cleaned, nil
			}
			lastErr = err

			if isQuotaExhausted(lastErr) {
				fmt.Printf("⚠️ Model %s hết quota (429), tự động chuyển sang model dự phòng...\n", currentModel)
				break
			}

			if !isRetryable(lastErr) || attempt == maxAttempts {
				break
			}

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			delay *= 2
		}
	}

	return "", lastErr
}
