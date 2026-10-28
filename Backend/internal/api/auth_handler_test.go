package api

import (
	"TAXSOLUTION/internal/auth"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeAuthCaller struct {
	resp       *auth.Response
	err        error
	calls      int
	lastAction string
	lastFields map[string]any
}

func (f *fakeAuthCaller) Call(_ context.Context, action string, fields map[string]any) (*auth.Response, error) {
	f.calls++
	f.lastAction = action
	f.lastFields = fields
	return f.resp, f.err
}

func newAuthTestRouter(f *fakeAuthCaller) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := auth.NewService(f)
	r := gin.New()
	registerAuthRoutesWith(r, func() *auth.Service { return svc })
	return r
}

func doJSON(r *gin.Engine, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func TestAuthLoginOK(t *testing.T) {
	f := &fakeAuthCaller{resp: &auth.Response{Result: auth.ResultOK, FullName: "A", MaxTaxAccounts: 3, AllowAI: true, EndDate: "31/12/2099"}}
	r := newAuthTestRouter(f)

	w, out := doJSON(r, http.MethodPost, "/api/auth/login", `{"username":"u1","password":"p"}`)
	if w.Code != http.StatusOK || out["status"] != "success" || out["result"] != "OK" {
		t.Fatalf("login = %d %v", w.Code, out)
	}
	data := out["data"].(map[string]any)
	if data["username"] != "u1" || data["maxTaxAccounts"].(float64) != 3 || data["allowAI"] != true {
		t.Errorf("data sai: %v", data)
	}

	// /session phải thấy phiên vừa tạo
	w, out = doJSON(r, http.MethodGet, "/api/auth/session", "")
	if w.Code != http.StatusOK || out["data"].(map[string]any)["username"] != "u1" {
		t.Errorf("session = %d %v", w.Code, out)
	}
}

func TestAuthLoginErrorsMapToHTTPStatus(t *testing.T) {
	cases := []struct {
		result auth.Result
		code   int
	}{
		{auth.ResultWrongCredentials, http.StatusUnauthorized},
		{auth.ResultPending, http.StatusForbidden},
		{auth.ResultLocked, http.StatusForbidden},
		{auth.ResultWrongDevice, http.StatusForbidden},
		{auth.ResultExpired, http.StatusForbidden},
		{auth.ResultNotStarted, http.StatusForbidden},
		{auth.ResultUnauthorized, http.StatusInternalServerError},
	}
	for _, c := range cases {
		r := newAuthTestRouter(&fakeAuthCaller{resp: &auth.Response{Result: c.result}})
		w, out := doJSON(r, http.MethodPost, "/api/auth/login", `{"username":"u1","password":"p"}`)
		if w.Code != c.code || out["status"] != "error" || out["result"] != string(c.result) {
			t.Errorf("%s: %d %v, want %d", c.result, w.Code, out, c.code)
		}
		if out["message"] == "" || out["message"] == nil {
			t.Errorf("%s: thiếu message tiếng Việt", c.result)
		}
	}
}

func TestAuthLoginNetworkErrorIs503(t *testing.T) {
	r := newAuthTestRouter(&fakeAuthCaller{err: auth.ErrNetwork})
	w, out := doJSON(r, http.MethodPost, "/api/auth/login", `{"username":"u1","password":"p"}`)
	if w.Code != http.StatusServiceUnavailable || out["result"] != "NETWORK_ERROR" {
		t.Errorf("login = %d %v", w.Code, out)
	}
}

func TestAuthLoginNotConfiguredIs500(t *testing.T) {
	r := newAuthTestRouter(&fakeAuthCaller{err: auth.ErrNotConfigured})
	w, out := doJSON(r, http.MethodPost, "/api/auth/login", `{"username":"u1","password":"p"}`)
	if w.Code != http.StatusInternalServerError || out["result"] != "NOT_CONFIGURED" {
		t.Errorf("login = %d %v", w.Code, out)
	}
}

func TestAuthLoginBadJSON(t *testing.T) {
	r := newAuthTestRouter(&fakeAuthCaller{})
	w, _ := doJSON(r, http.MethodPost, "/api/auth/login", `không phải json`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", w.Code)
	}
}

func TestAuthFounderCannotLoginViaSheet(t *testing.T) {
	f := &fakeAuthCaller{resp: &auth.Response{Result: auth.ResultOK}}
	r := newAuthTestRouter(f)
	w, out := doJSON(r, http.MethodPost, "/api/auth/login", `{"username":"Founder","password":"p"}`)
	if w.Code != http.StatusUnauthorized || out["result"] != "WRONG_CREDENTIALS" {
		t.Errorf("login = %d %v", w.Code, out)
	}
	if f.calls != 0 {
		t.Errorf("Founder không được gọi lên máy chủ xác thực, calls = %d", f.calls)
	}
}

func TestAuthCheckWithoutSessionIs401(t *testing.T) {
	r := newAuthTestRouter(&fakeAuthCaller{})
	w, out := doJSON(r, http.MethodPost, "/api/auth/check", ``)
	if w.Code != http.StatusUnauthorized || out["result"] != "NO_SESSION" {
		t.Errorf("check = %d %v", w.Code, out)
	}
}

func TestAuthCheckExpiredReturns403AndEndsSession(t *testing.T) {
	f := &fakeAuthCaller{resp: &auth.Response{Result: auth.ResultOK}}
	r := newAuthTestRouter(f)
	doJSON(r, http.MethodPost, "/api/auth/login", `{"username":"u1","password":"p"}`)

	f.resp = &auth.Response{Result: auth.ResultExpired, EndDate: "01/01/2020"}
	w, out := doJSON(r, http.MethodPost, "/api/auth/check", `{"force":true}`)
	if w.Code != http.StatusForbidden || out["result"] != "EXPIRED" {
		t.Fatalf("check = %d %v", w.Code, out)
	}
	if !strings.Contains(out["message"].(string), "01/01/2020") {
		t.Errorf("message thiếu ngày hết hạn: %v", out["message"])
	}
	if w, _ := doJSON(r, http.MethodGet, "/api/auth/session", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("sau khi hết hạn, session phải 401, got %d", w.Code)
	}
}

func TestAuthLogout(t *testing.T) {
	f := &fakeAuthCaller{resp: &auth.Response{Result: auth.ResultOK}}
	r := newAuthTestRouter(f)
	doJSON(r, http.MethodPost, "/api/auth/login", `{"username":"u1","password":"p"}`)
	if w, _ := doJSON(r, http.MethodPost, "/api/auth/logout", ``); w.Code != http.StatusOK {
		t.Errorf("logout = %d", w.Code)
	}
	if w, _ := doJSON(r, http.MethodGet, "/api/auth/session", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("sau logout, session phải 401, got %d", w.Code)
	}
}

func TestAuthHWID(t *testing.T) {
	r := newAuthTestRouter(&fakeAuthCaller{})
	w, out := doJSON(r, http.MethodGet, "/api/auth/hwid", "")
	hwid, _ := out["data"].(map[string]any)["hwid"].(string)
	if w.Code != http.StatusOK || !strings.HasPrefix(hwid, "TS-") {
		t.Errorf("hwid = %d %v", w.Code, out)
	}
}


const adminBody = `{"adminToken":"tok","username":"nv01","password":"mk123456","fullName":"Nguyen A",` +
	`"hwid":"TS-1AAA-4368-F43E","maxTaxAccounts":3,"allowAI":true,"startDate":"01/11/2026","endDate":"31/10/2027"}`

func TestAdminCreateOK(t *testing.T) {
	f := &fakeAuthCaller{resp: &auth.Response{Result: auth.ResultOK}}
	r := newAuthTestRouter(f)
	w, out := doJSON(r, http.MethodPost, "/api/auth/admin/create", adminBody)
	if w.Code != http.StatusOK || out["status"] != "success" {
		t.Fatalf("admin create = %d %v", w.Code, out)
	}
	if f.lastAction != "adminCreate" || f.lastFields["username"] != "nv01" || f.lastFields["maxTaxAccounts"] != 3 ||
		f.lastFields["allowAI"] != true || f.lastFields["adminToken"] != "tok" {
		t.Errorf("chuyển tiếp sai: %s %v", f.lastAction, f.lastFields)
	}
	if f.lastFields["passHash"] != auth.HashPassword("mk123456") {
		t.Error("mật khẩu phải được băm trước khi gửi")
	}
	// Không được tạo phiên đăng nhập
	if w, _ := doJSON(r, http.MethodGet, "/api/auth/session", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("admin create không được tạo session, got %d", w.Code)
	}
}

func TestAdminCreateDefaultMaxTax(t *testing.T) {
	f := &fakeAuthCaller{resp: &auth.Response{Result: auth.ResultOK}}
	r := newAuthTestRouter(f)
	doJSON(r, http.MethodPost, "/api/auth/admin/create", `{"adminToken":"tok","username":"nv01","password":"x"}`)
	if f.lastFields["maxTaxAccounts"] != 5 {
		t.Errorf("mặc định phải là 5, got %v", f.lastFields["maxTaxAccounts"])
	}
}

func TestAdminCreateZeroMaxTaxIsKept(t *testing.T) {
	f := &fakeAuthCaller{resp: &auth.Response{Result: auth.ResultOK}}
	r := newAuthTestRouter(f)
	doJSON(r, http.MethodPost, "/api/auth/admin/create", `{"adminToken":"tok","username":"nv01","password":"x","maxTaxAccounts":0}`)
	if f.lastFields["maxTaxAccounts"] != 0 {
		t.Errorf("0 phải được giữ nguyên, got %v", f.lastFields["maxTaxAccounts"])
	}
}

func TestAdminCreateValidationIs400WithoutCalling(t *testing.T) {
	f := &fakeAuthCaller{resp: &auth.Response{Result: auth.ResultOK}}
	r := newAuthTestRouter(f)
	w, out := doJSON(r, http.MethodPost, "/api/auth/admin/create",
		`{"adminToken":"tok","username":"nv01","password":"x","hwid":"abc"}`)
	if w.Code != http.StatusBadRequest || out["result"] != "BAD_REQUEST" || !strings.Contains(out["message"].(string), "Mã máy") {
		t.Errorf("= %d %v", w.Code, out)
	}
	if f.calls != 0 {
		t.Errorf("dữ liệu sai không được gọi lên máy chủ, calls = %d", f.calls)
	}
}

func TestAdminCreateUnauthorizedIs403(t *testing.T) {
	r := newAuthTestRouter(&fakeAuthCaller{resp: &auth.Response{Result: auth.ResultUnauthorized}})
	w, out := doJSON(r, http.MethodPost, "/api/auth/admin/create", adminBody)
	if w.Code != http.StatusForbidden || !strings.Contains(out["message"].(string), "Founder") {
		t.Errorf("= %d %v", w.Code, out)
	}
}

func TestAdminCreateExistsIs400(t *testing.T) {
	r := newAuthTestRouter(&fakeAuthCaller{resp: &auth.Response{Result: auth.ResultExists}})
	w, out := doJSON(r, http.MethodPost, "/api/auth/admin/create", adminBody)
	if w.Code != http.StatusBadRequest || out["result"] != "EXISTS" || !strings.Contains(out["message"].(string), "đã tồn tại") {
		t.Errorf("= %d %v", w.Code, out)
	}
}

func TestAdminCreateNetworkErrorIs503(t *testing.T) {
	r := newAuthTestRouter(&fakeAuthCaller{err: auth.ErrNetwork})
	w, out := doJSON(r, http.MethodPost, "/api/auth/admin/create", adminBody)
	if w.Code != http.StatusServiceUnavailable || out["result"] != "NETWORK_ERROR" {
		t.Errorf("= %d %v", w.Code, out)
	}
}

func TestAdminCreateBadJSON(t *testing.T) {
	r := newAuthTestRouter(&fakeAuthCaller{})
	if w, _ := doJSON(r, http.MethodPost, "/api/auth/admin/create", `{`); w.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", w.Code)
	}
}
