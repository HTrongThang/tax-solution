// Standalone test tool: solve a CAPTCHA image using the Gemini API.
//
// Usage (from d:\tax-solution\Backend):
//
//	$env:GEMINI_API_KEY="your-key"
//	go run ./cmd/captchatest -img cmd/current_captcha.png
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// loadEnv đọc các file .env theo thứ tự ưu tiên và nạp vào os.Environ nếu chưa có
func loadEnv(paths ...string) {
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
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


const (
	defaultModel = "gemini-3.8-flash"
	prompt       = "Extract ONLY the 6-character alphanumeric CAPTCHA code inside the gray box " +
		"(ignore the label 'Mã captcha', strikethrough lines, and refresh icon). " +
		"Return ONLY the 6 uppercase characters, no spaces."
)

var nonAlnum = regexp.MustCompile(`[^A-Z0-9]`)

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func solveCaptcha(ctx context.Context, apiKey, model string, img []byte) (raw, cleaned string, err error) {
	mimeType := http.DetectContentType(img)
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
						"data":      base64.StdEncoding.EncodeToString(img),
					}},
				},
			},
		},
		"generationConfig": map[string]any{"temperature": 0},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	// Send key via header instead of query string so it doesn't leak into logs/URLs.
	req.Header.Set("x-goog-api-key", apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", &apiError{Status: resp.StatusCode, Body: string(respBody)}
	}

	var gr geminiResponse
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return "", "", fmt.Errorf("parse response: %w", err)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return "", "", fmt.Errorf("empty response: %s", respBody)
	}

	raw = gr.Candidates[0].Content.Parts[0].Text
	cleaned = nonAlnum.ReplaceAllString(strings.ToUpper(raw), "")
	return raw, cleaned, nil
}

type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("API error (%d): %s", e.Status, e.Body) }

// retryable reports whether the error is a transient overload / rate-limit error.
func retryable(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && (ae.Status == http.StatusServiceUnavailable || ae.Status == http.StatusTooManyRequests)
}

// solveWithRetry retries transient errors with exponential backoff (1s, 2s, 4s, ...).
func solveWithRetry(ctx context.Context, apiKey, model string, img []byte, maxAttempts int) (string, string, error) {
	delay := time.Second
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		raw, cleaned, err := solveCaptcha(ctx, apiKey, model, img)
		if err == nil {
			return raw, cleaned, nil
		}
		lastErr = err
		if !retryable(err) || attempt == maxAttempts {
			break
		}
		fmt.Fprintf(os.Stderr, "attempt %d failed (%v), retrying in %s...\n", attempt, err.(*apiError).Status, delay)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
		delay *= 2
	}
	return "", "", lastErr
}

func main() {
	// Nạp từ file .env (thử thư mục hiện tại và thư mục gốc project)
	loadEnv(".env", "../.env", "Backend/.env")

	defaultM := defaultModel
	if envM := os.Getenv("GEMINI_MODEL"); envM != "" {
		defaultM = envM
	}

	imgPath := flag.String("img", "cmd/current_captcha.png", "path to captcha image")
	model := flag.String("model", defaultM, "Gemini model name")
	retries := flag.Int("retries", 5, "max attempts on 429/503 errors")
	flag.Parse()

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Lỗi: Chưa cấu hình GEMINI_API_KEY trong file .env hoặc biến môi trường!")
		os.Exit(1)
	}

	img, err := os.ReadFile(*imgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read image: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	start := time.Now()
	raw, cleaned, err := solveWithRetry(ctx, apiKey, *model, img, *retries)
	if err != nil {
		fmt.Fprintf(os.Stderr, "solve failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Image   : %s (%d bytes)\n", *imgPath, len(img))
	fmt.Printf("Model   : %s\n", *model)
	fmt.Printf("Raw     : %q\n", raw)
	fmt.Printf("Captcha : %s (len=%d)\n", cleaned, len(cleaned))
	fmt.Printf("Elapsed : %s\n", time.Since(start).Round(time.Millisecond))
}
