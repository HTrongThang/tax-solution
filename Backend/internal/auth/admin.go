package auth

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// AdminCreateRequest là yêu cầu Founder cấp một tài khoản nhân viên (ghi lên Google Sheet).
type AdminCreateRequest struct {
	AdminToken     string // SHA-256 mật khẩu Founder; script đối chiếu với FOUNDER_HASH
	Username       string
	Password       string
	FullName       string
	HWID           string // Mã máy; để trống = gắn ở lần đăng nhập đầu tiên
	MaxTaxAccounts int
	AllowAI        bool
	StartDate      string // dd/MM/yyyy, có thể trống
	EndDate        string // dd/MM/yyyy, có thể trống
}

// RegisterRequest là yêu cầu khách tự đăng ký tài khoản (ghi lên Google Sheet với trạng thái PENDING).
type RegisterRequest struct {
	Username string
	Password string
	FullName string
}

// ValidationError báo dữ liệu đầu vào sai (hiển thị được cho người dùng).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// IsValidationError cho biết err có phải lỗi dữ liệu đầu vào không.
func IsValidationError(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}

const (
	dateLayout     = "02/01/2006"
	maxUsernameLen = 50
	maxFullNameLen = 100
	maxTaxLimit    = 100000
)

var hwidPattern = regexp.MustCompile(`^TS-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}$`)

// AdminCreate kiểm tra dữ liệu rồi nhờ script ghi tài khoản mới (trạng thái ACTIVE).
// Việc xác thực Founder do script làm (so adminToken với FOUNDER_HASH).
func (s *Service) AdminCreate(ctx context.Context, req AdminCreateRequest) (*Response, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.FullName = strings.TrimSpace(req.FullName)
	req.HWID = strings.ToUpper(strings.TrimSpace(req.HWID))
	req.StartDate = strings.TrimSpace(req.StartDate)
	req.EndDate = strings.TrimSpace(req.EndDate)

	if strings.TrimSpace(req.AdminToken) == "" {
		return nil, &ValidationError{"Thiếu xác thực Founder."}
	}
	if req.Username == "" || req.Password == "" {
		return nil, &ValidationError{"Tên đăng nhập và mật khẩu không được để trống."}
	}
	if len([]rune(req.Username)) > maxUsernameLen {
		return nil, &ValidationError{"Tên đăng nhập quá dài (tối đa 50 ký tự)."}
	}
	for _, r := range req.Username {
		if unicode.IsControl(r) {
			return nil, &ValidationError{"Tên đăng nhập chứa ký tự không hợp lệ."}
		}
	}
	if strings.EqualFold(req.Username, "founder") {
		return &Response{Result: ResultExists}, nil
	}
	if len([]rune(req.FullName)) > maxFullNameLen {
		req.FullName = string([]rune(req.FullName)[:maxFullNameLen])
	}
	if req.HWID != "" && !hwidPattern.MatchString(req.HWID) {
		return nil, &ValidationError{"Mã máy không đúng định dạng (TS-XXXX-XXXX-XXXX)."}
	}
	if req.MaxTaxAccounts < 0 || req.MaxTaxAccounts > maxTaxLimit {
		return nil, &ValidationError{"Số MST tối đa phải từ 0 trở lên."}
	}

	var start, end time.Time
	var err error
	if req.StartDate != "" {
		if start, err = time.Parse(dateLayout, req.StartDate); err != nil {
			return nil, &ValidationError{"Ngày bắt đầu không đúng định dạng dd/MM/yyyy."}
		}
	}
	if req.EndDate != "" {
		if end, err = time.Parse(dateLayout, req.EndDate); err != nil {
			return nil, &ValidationError{"Ngày kết thúc không đúng định dạng dd/MM/yyyy."}
		}
	}
	if !start.IsZero() && !end.IsZero() && start.After(end) {
		return nil, &ValidationError{"Ngày kết thúc phải sau ngày bắt đầu."}
	}

	return s.caller.Call(ctx, "adminCreate", map[string]any{
		"adminToken":     req.AdminToken,
		"username":       req.Username,
		"passHash":       HashPassword(req.Password),
		"fullName":       req.FullName,
		"hwid":           req.HWID,
		"maxTaxAccounts": req.MaxTaxAccounts,
		"allowAI":        req.AllowAI,
		"startDate":      req.StartDate,
		"endDate":        req.EndDate,
	})
}

// Register gửi yêu cầu cấp tài khoản mới lên Google Sheet (trạng thái PENDING, gắn sẵn mã máy hiện tại).
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*Response, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.FullName = strings.TrimSpace(req.FullName)

	if req.Username == "" || req.Password == "" {
		return nil, &ValidationError{"Tên đăng nhập và mật khẩu không được để trống."}
	}
	if len([]rune(req.Username)) > maxUsernameLen {
		return nil, &ValidationError{"Tên đăng nhập quá dài (tối đa 50 ký tự)."}
	}
	for _, r := range req.Username {
		if unicode.IsControl(r) {
			return nil, &ValidationError{"Tên đăng nhập chứa ký tự không hợp lệ."}
		}
	}
	if strings.EqualFold(req.Username, "founder") {
		return &Response{Result: ResultExists}, nil
	}
	if len([]rune(req.FullName)) > maxFullNameLen {
		req.FullName = string([]rune(req.FullName)[:maxFullNameLen])
	}

	return s.caller.Call(ctx, "register", map[string]any{
		"username": req.Username,
		"passHash": HashPassword(req.Password),
		"fullName": req.FullName,
		"hwid":     HardwareID(),
	})
}

