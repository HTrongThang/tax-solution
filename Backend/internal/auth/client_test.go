package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(url string) *Client {
	return &Client{URL: url, APIKey: "secret-key", HTTP: &http.Client{}, allowAnyURL: true}
}

func TestCallSendsActionAndKey(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"result":"OK","fullName":"Nguyen A","maxTaxAccounts":5,"allowAI":true,"endDate":"31/12/2026"}`))
	}))
	defer srv.Close()

	resp, err := testClient(srv.URL).Call(context.Background(), "login", map[string]any{"username": "u1"})
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if resp.Result != ResultOK || resp.FullName != "Nguyen A" || resp.MaxTaxAccounts != 5 || !resp.AllowAI || resp.EndDate != "31/12/2026" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if got["action"] != "login" || got["apiKey"] != "secret-key" || got["username"] != "u1" {
		t.Errorf("unexpected payload: %v", got)
	}
}

func TestCallBusinessResultIsNotError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":"WRONG_DEVICE"}`))
	}))
	defer srv.Close()

	resp, err := testClient(srv.URL).Call(context.Background(), "check", nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if resp.Result != ResultWrongDevice {
		t.Errorf("result = %s, want WRONG_DEVICE", resp.Result)
	}
}

func TestCallHTTPErrorIsNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := testClient(srv.URL).Call(context.Background(), "check", nil)
	if !errors.Is(err, ErrNetwork) {
		t.Errorf("err = %v, want ErrNetwork", err)
	}
}

func TestCallInvalidJSONIsNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer srv.Close()

	_, err := testClient(srv.URL).Call(context.Background(), "check", nil)
	if !errors.Is(err, ErrNetwork) {
		t.Errorf("err = %v, want ErrNetwork", err)
	}
}

func TestCallUnreachableDoesNotLeakKey(t *testing.T) {
	c := testClient("http://127.0.0.1:1") // cổng đóng
	_, err := c.Call(context.Background(), "check", nil)
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v, want ErrNetwork", err)
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Errorf("error leaks api key: %v", err)
	}
}

func TestNotConfigured(t *testing.T) {
	cases := map[string]*Client{
		"thiếu key": {URL: "https://script.google.com/macros/s/x/exec"},
		"thiếu url": {APIKey: "k"},
		"http":      {URL: "http://script.google.com/macros/s/x/exec", APIKey: "k"},
		"sai host":  {URL: "https://evil.example.com/exec", APIKey: "k"},
	}
	for name, c := range cases {
		if c.Configured() {
			t.Errorf("%s: Configured() = true, want false", name)
		}
		if _, err := c.Call(context.Background(), "check", nil); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("%s: err = %v, want ErrNotConfigured", name, err)
		}
	}

	ok := &Client{URL: "https://script.google.com/macros/s/x/exec", APIKey: "k"}
	if !ok.Configured() {
		t.Error("cấu hình hợp lệ nhưng Configured() = false")
	}
}

