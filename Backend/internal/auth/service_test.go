package auth

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

// fakeCaller ghi lại các lần gọi và trả về kết quả định sẵn.
type fakeCaller struct {
	calls  []string
	fields []map[string]any
	resp   *Response
	err    error
}

func (f *fakeCaller) Call(_ context.Context, action string, fields map[string]any) (*Response, error) {
	f.calls = append(f.calls, action)
	f.fields = append(f.fields, fields)
	return f.resp, f.err
}

func okResp() *Response {
	return &Response{Result: ResultOK, FullName: "Nguyen A", MaxTaxAccounts: 5, AllowAI: true, EndDate: "31/12/2099"}
}

func TestHashPassword(t *testing.T) {
	// SHA-256("test123")
	want := "ecd71870d1963316a97e3ac3408c9835ad8cf0f3c1bc703527c30265534f75ae"
	if got := HashPassword("test123"); got != want {
		t.Errorf("HashPassword = %s, want %s", got, want)
	}
}

func TestHardwareIDFormatAndStable(t *testing.T) {
	re := regexp.MustCompile(`^TS-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}$`)
	id := HardwareID()
	if !re.MatchString(id) {
		t.Errorf("HardwareID = %q, sai định dạng", id)
	}
	if HardwareID() != id {
		t.Error("HardwareID không ổn định giữa các lần gọi")
	}
	if buildHardwareID("guid-1") == buildHardwareID("guid-2") {
		t.Error("hai MachineGuid khác nhau phải cho hai Mã máy khác nhau")
	}
	if buildHardwareID("guid-1") != buildHardwareID(" guid-1 ") {
		t.Error("khoảng trắng thừa không được làm đổi Mã máy")
	}
}

func TestLoginSendsHashedPasswordNotPlain(t *testing.T) {
	f := &fakeCaller{resp: okResp()}
	svc := NewService(f)

	resp, err := svc.Login(context.Background(), "  u1  ", "matkhau")
	if err != nil || resp.Result != ResultOK {
		t.Fatalf("Login = %+v, %v", resp, err)
	}
	if f.calls[0] != "login" {
		t.Errorf("action = %s, want login", f.calls[0])
	}
	got := f.fields[0]
	if got["username"] != "u1" {
		t.Errorf("username = %v, want u1 (đã trim)", got["username"])
	}
	if got["passHash"] != HashPassword("matkhau") {
		t.Error("passHash không phải SHA-256 của mật khẩu")
	}
	if got["hwid"] != HardwareID() {
		t.Error("thiếu hwid")
	}
	for k, v := range got {
		if s, ok := v.(string); ok && s == "matkhau" {
			t.Errorf("mật khẩu thô bị gửi đi ở trường %q", k)
		}
	}

	sess := svc.Current()
	if sess == nil || sess.Username != "u1" || sess.MaxTaxAccounts != 5 || !sess.AllowAI || sess.EndDate != "31/12/2099" {
		t.Errorf("session sai: %+v", sess)
	}
}

func TestLoginRejectsFounderAndEmptyWithoutCalling(t *testing.T) {
	f := &fakeCaller{resp: okResp()}
	svc := NewService(f)

	cases := []struct {
		user, pass string
		want       Result
	}{
		{"Founder", "x", ResultWrongCredentials},
		{"FOUNDER", "x", ResultWrongCredentials},
		{"", "x", ResultBadRequest},
		{"u", "", ResultBadRequest},
	}
	for _, c := range cases {
		resp, err := svc.Login(context.Background(), c.user, c.pass)
		if err != nil || resp.Result != c.want {
			t.Errorf("Login(%q,%q) = %+v, %v; want %s", c.user, c.pass, resp, err, c.want)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("không được gọi máy chủ, nhưng đã gọi %d lần", len(f.calls))
	}
}

func TestLoginFailureClearsSession(t *testing.T) {
	f := &fakeCaller{resp: okResp()}
	svc := NewService(f)
	if _, err := svc.Login(context.Background(), "u1", "p"); err != nil {
		t.Fatal(err)
	}

	f.resp = &Response{Result: ResultPending}
	resp, err := svc.Login(context.Background(), "u2", "p")
	if err != nil || resp.Result != ResultPending {
		t.Fatalf("Login = %+v, %v", resp, err)
	}
	if svc.Current() != nil {
		t.Error("đăng nhập thất bại phải xóa phiên cũ")
	}

	f.err = ErrNetwork // mất mạng cũng không được giữ phiên
	f.resp = nil
	if _, err := svc.Login(context.Background(), "u3", "p"); !errors.Is(err, ErrNetwork) {
		t.Errorf("err = %v, want ErrNetwork", err)
	}
	if svc.Current() != nil {
		t.Error("mất mạng khi đăng nhập không được tạo phiên")
	}
}

func TestCheckWithoutSession(t *testing.T) {
	svc := NewService(&fakeCaller{resp: okResp()})
	if _, err := svc.Check(context.Background(), false); !errors.Is(err, ErrNoSession) {
		t.Errorf("err = %v, want ErrNoSession", err)
	}
}

func TestCheckUsesCacheWithinTTL(t *testing.T) {
	f := &fakeCaller{resp: okResp()}
	svc := NewService(f)
	now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	if _, err := svc.Login(context.Background(), "u1", "p"); err != nil {
		t.Fatal(err)
	}
	f.calls = nil

	now = now.Add(CheckTTL - time.Second)
	resp, err := svc.Check(context.Background(), false)
	if err != nil || resp.Result != ResultOK {
		t.Fatalf("Check = %+v, %v", resp, err)
	}
	if len(f.calls) != 0 {
		t.Errorf("trong TTL không được gọi máy chủ, nhưng đã gọi %v", f.calls)
	}

	// force=true luôn hỏi lại
	if _, err := svc.Check(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0] != "check" {
		t.Errorf("force phải gọi check, calls = %v", f.calls)
	}
}

func TestCheckAfterTTLCallsServerAndRefreshes(t *testing.T) {
	f := &fakeCaller{resp: okResp()}
	svc := NewService(f)
	now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	_, _ = svc.Login(context.Background(), "u1", "p")
	f.calls, f.fields = nil, nil

	f.resp = &Response{Result: ResultOK, FullName: "Nguyen A", MaxTaxAccounts: 9, AllowAI: false, EndDate: "01/01/2100"}
	now = now.Add(CheckTTL + time.Second)
	if _, err := svc.Check(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("hết TTL phải gọi máy chủ 1 lần, calls = %v", f.calls)
	}
	if f.fields[0]["username"] != "u1" || f.fields[0]["hwid"] != HardwareID() {
		t.Errorf("check gửi thiếu thông tin: %v", f.fields[0])
	}
	if _, has := f.fields[0]["passHash"]; has {
		t.Error("check không được gửi mật khẩu")
	}
	sess := svc.Current()
	if sess.MaxTaxAccounts != 9 || sess.AllowAI || sess.EndDate != "01/01/2100" {
		t.Errorf("quyền hạn chưa được làm mới: %+v", sess)
	}
}

func TestCheckNonOKClearsSession(t *testing.T) {
	for _, bad := range []Result{ResultExpired, ResultLocked, ResultWrongDevice} {
		f := &fakeCaller{resp: okResp()}
		svc := NewService(f)
		_, _ = svc.Login(context.Background(), "u1", "p")

		f.resp = &Response{Result: bad}
		resp, err := svc.Check(context.Background(), true)
		if err != nil || resp.Result != bad {
			t.Errorf("%s: Check = %+v, %v", bad, resp, err)
		}
		if svc.Current() != nil {
			t.Errorf("%s: phiên phải bị xóa", bad)
		}
		if _, err := svc.Check(context.Background(), false); !errors.Is(err, ErrNoSession) {
			t.Errorf("%s: sau khi bị xóa, Check phải trả ErrNoSession, got %v", bad, err)
		}
	}
}

func TestCheckNetworkErrorAfterTTLIsError(t *testing.T) {
	f := &fakeCaller{resp: okResp()}
	svc := NewService(f)
	now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	_, _ = svc.Login(context.Background(), "u1", "p")

	f.resp, f.err = nil, ErrNetwork
	now = now.Add(CheckTTL + time.Second)
	if _, err := svc.Check(context.Background(), false); !errors.Is(err, ErrNetwork) {
		t.Errorf("err = %v, want ErrNetwork (mất mạng thì không được dùng)", err)
	}
}

func TestLogout(t *testing.T) {
	svc := NewService(&fakeCaller{resp: okResp()})
	_, _ = svc.Login(context.Background(), "u1", "p")
	svc.Logout()
	if svc.Current() != nil {
		t.Error("Logout phải xóa phiên")
	}
}

func TestCurrentReturnsCopy(t *testing.T) {
	svc := NewService(&fakeCaller{resp: okResp()})
	_, _ = svc.Login(context.Background(), "u1", "p")
	s := svc.Current()
	s.MaxTaxAccounts = 999
	if svc.Current().MaxTaxAccounts == 999 {
		t.Error("Current() phải trả bản sao, không cho sửa phiên từ bên ngoài")
	}
}

func TestDescribe(t *testing.T) {
	cases := map[Result]string{
		ResultWrongCredentials: "không đúng",
		ResultPending:          "chờ Admin",
		ResultLocked:           "bị khóa",
		ResultWrongDevice:      "thiết bị khác",
	}
	for r, sub := range cases {
		if got := Describe(&Response{Result: r}); !strings.Contains(got, sub) {
			t.Errorf("Describe(%s) = %q, thiếu %q", r, got, sub)
		}
	}
	if got := Describe(&Response{Result: ResultExpired, EndDate: "01/01/2020"}); !strings.Contains(got, "01/01/2020") {
		t.Errorf("Describe(EXPIRED) = %q, thiếu ngày hết hạn", got)
	}
	if got := Describe(&Response{Result: ResultError, Message: "Hệ thống đang bận"}); got != "Hệ thống đang bận" {
		t.Errorf("Describe(ERROR) = %q", got)
	}
}
