package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func validReq() AdminCreateRequest {
	return AdminCreateRequest{
		AdminToken: "tok", Username: "nv01", Password: "mk123456", FullName: "Nguyen A",
		HWID: "TS-1AAA-4368-F43E", MaxTaxAccounts: 5, AllowAI: true,
		StartDate: "01/11/2026", EndDate: "31/10/2027",
	}
}

func TestAdminCreateSendsHashedPasswordAndFields(t *testing.T) {
	f := &fakeCaller{resp: &Response{Result: ResultOK}}
	svc := NewService(f)

	resp, err := svc.AdminCreate(context.Background(), validReq())
	if err != nil || resp.Result != ResultOK {
		t.Fatalf("AdminCreate = %+v, %v", resp, err)
	}
	if f.calls[0] != "adminCreate" {
		t.Fatalf("action = %s", f.calls[0])
	}
	got := f.fields[0]
	if got["passHash"] != HashPassword("mk123456") {
		t.Error("passHash không phải SHA-256 của mật khẩu")
	}
	for k, v := range got {
		if s, ok := v.(string); ok && s == "mk123456" {
			t.Errorf("mật khẩu thô bị gửi đi ở trường %q", k)
		}
	}
	if got["adminToken"] != "tok" || got["username"] != "nv01" || got["hwid"] != "TS-1AAA-4368-F43E" ||
		got["maxTaxAccounts"] != 5 || got["allowAI"] != true ||
		got["startDate"] != "01/11/2026" || got["endDate"] != "31/10/2027" {
		t.Errorf("fields sai: %v", got)
	}
	if svc.Current() != nil {
		t.Error("AdminCreate không được tạo phiên đăng nhập")
	}
}

func TestAdminCreateNormalizesInput(t *testing.T) {
	f := &fakeCaller{resp: &Response{Result: ResultOK}}
	r := validReq()
	r.Username = "  nv01  "
	r.HWID = " ts-1aaa-4368-f43e "
	if _, err := NewService(f).AdminCreate(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if f.fields[0]["username"] != "nv01" || f.fields[0]["hwid"] != "TS-1AAA-4368-F43E" {
		t.Errorf("chưa chuẩn hóa: %v", f.fields[0])
	}
}

func TestAdminCreateEmptyHWIDAndDatesAllowed(t *testing.T) {
	f := &fakeCaller{resp: &Response{Result: ResultOK}}
	r := validReq()
	r.HWID, r.StartDate, r.EndDate = "", "", ""
	if _, err := NewService(f).AdminCreate(context.Background(), r); err != nil {
		t.Fatalf("để trống Mã máy/ngày phải hợp lệ: %v", err)
	}
}

func TestAdminCreateValidationRejectsWithoutCalling(t *testing.T) {
	long := strings.Repeat("a", maxUsernameLen+1)
	cases := map[string]func(*AdminCreateRequest){
		"thiếu token":       func(r *AdminCreateRequest) { r.AdminToken = " " },
		"thiếu username":    func(r *AdminCreateRequest) { r.Username = "  " },
		"thiếu mật khẩu":    func(r *AdminCreateRequest) { r.Password = "" },
		"username quá dài":  func(r *AdminCreateRequest) { r.Username = long },
		"username ký tự lạ": func(r *AdminCreateRequest) { r.Username = "a\x00b" },
		"hwid sai":          func(r *AdminCreateRequest) { r.HWID = "ABC" },
		"hwid sai chữ":      func(r *AdminCreateRequest) { r.HWID = "TS-ZZZZ-4368-F43E" },
		"MST âm":            func(r *AdminCreateRequest) { r.MaxTaxAccounts = -1 },
		"MST quá lớn":       func(r *AdminCreateRequest) { r.MaxTaxAccounts = maxTaxLimit + 1 },
		"ngày bắt đầu sai":  func(r *AdminCreateRequest) { r.StartDate = "2026-11-01" },
		"ngày kết thúc sai": func(r *AdminCreateRequest) { r.EndDate = "31/13/2027" },
		"kết thúc < bắt đầu": func(r *AdminCreateRequest) {
			r.StartDate, r.EndDate = "01/01/2027", "01/01/2026"
		},
	}
	for name, mutate := range cases {
		f := &fakeCaller{resp: &Response{Result: ResultOK}}
		r := validReq()
		mutate(&r)
		_, err := NewService(f).AdminCreate(context.Background(), r)
		if !IsValidationError(err) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
		}
		if len(f.calls) != 0 {
			t.Errorf("%s: không được gọi máy chủ khi dữ liệu sai", name)
		}
	}
}

func TestAdminCreateFounderNameIsExistsWithoutCalling(t *testing.T) {
	for _, name := range []string{"Founder", "FOUNDER", " founder "} {
		f := &fakeCaller{resp: &Response{Result: ResultOK}}
		r := validReq()
		r.Username = name
		resp, err := NewService(f).AdminCreate(context.Background(), r)
		if err != nil || resp.Result != ResultExists {
			t.Errorf("%q: %+v, %v; want EXISTS", name, resp, err)
		}
		if len(f.calls) != 0 {
			t.Errorf("%q: không được gọi máy chủ", name)
		}
	}
}

func TestAdminCreatePassesThroughServerResults(t *testing.T) {
	for _, want := range []Result{ResultExists, ResultUnauthorized} {
		f := &fakeCaller{resp: &Response{Result: want}}
		resp, err := NewService(f).AdminCreate(context.Background(), validReq())
		if err != nil || resp.Result != want {
			t.Errorf("%s: %+v, %v", want, resp, err)
		}
	}
}

func TestAdminCreateNetworkError(t *testing.T) {
	f := &fakeCaller{err: ErrNetwork}
	if _, err := NewService(f).AdminCreate(context.Background(), validReq()); !errors.Is(err, ErrNetwork) {
		t.Errorf("err = %v, want ErrNetwork", err)
	}
}

func TestIsValidationError(t *testing.T) {
	if IsValidationError(ErrNetwork) {
		t.Error("ErrNetwork không phải ValidationError")
	}
	if !IsValidationError(&ValidationError{"x"}) {
		t.Error("ValidationError phải được nhận ra")
	}
}

func TestRegisterSendsHashedPasswordAndMachineHWID(t *testing.T) {
	f := &fakeCaller{resp: &Response{Result: ResultOK}}
	svc := NewService(f)

	resp, err := svc.Register(context.Background(), RegisterRequest{
		Username: "  khach01  ",
		Password: "secret",
		FullName: "  Cong ty A - 0909  ",
	})
	if err != nil || resp.Result != ResultOK {
		t.Fatalf("Register = %+v, %v", resp, err)
	}
	if f.calls[0] != "register" {
		t.Fatalf("action = %s", f.calls[0])
	}
	got := f.fields[0]
	if got["username"] != "khach01" || got["fullName"] != "Cong ty A - 0909" {
		t.Errorf("fields = %v", got)
	}
	if got["passHash"] != HashPassword("secret") {
		t.Errorf("passHash = %v", got["passHash"])
	}
	if got["hwid"] != HardwareID() {
		t.Errorf("hwid = %v, want %v", got["hwid"], HardwareID())
	}
}

func TestRegisterValidationAndFounderRejected(t *testing.T) {
	f := &fakeCaller{resp: &Response{Result: ResultOK}}
	svc := NewService(f)
	if _, err := svc.Register(context.Background(), RegisterRequest{Username: "", Password: "x"}); !IsValidationError(err) {
		t.Errorf("empty username should fail: %v", err)
	}
	resp, err := svc.Register(context.Background(), RegisterRequest{Username: "Founder", Password: "x"})
	if err != nil || resp.Result != ResultExists {
		t.Errorf("Founder should return EXISTS: %+v, %v", resp, err)
	}
	if len(f.calls) != 0 {
		t.Errorf("should not call server on invalid/Founder register")
	}
}

