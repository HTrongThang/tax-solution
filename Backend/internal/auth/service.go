package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Caller gửi một action tới máy chủ xác thực (Client thật hoặc bản giả khi test).
type Caller interface {
	Call(ctx context.Context, action string, fields map[string]any) (*Response, error)
}

// ErrNoSession: chưa đăng nhập (hoặc phiên đã bị xóa vì tài khoản không còn hợp lệ).
var ErrNoSession = errors.New("chưa đăng nhập")

// CheckTTL: trong khoảng này, kết quả kiểm tra thành công được dùng lại, không gọi lên Google
// (mỗi lần gọi mất vài giây). Hết thời gian mới hỏi lại; mất mạng lúc đó thì không cho dùng.
const CheckTTL = 2 * time.Minute

// Session là phiên đăng nhập hiện tại của nhân viên (chỉ giữ trong RAM).
type Session struct {
	Username       string
	FullName       string
	MaxTaxAccounts int
	AllowAI        bool
	EndDate        string
	VerifiedAt     time.Time
}

// Service quản lý đăng nhập và phiên của nhân viên.
type Service struct {
	caller Caller
	ttl    time.Duration
	now    func() time.Time

	mu      sync.Mutex
	session *Session
}

// NewService tạo Service dùng caller để nói chuyện với máy chủ xác thực.
func NewService(caller Caller) *Service {
	return &Service{caller: caller, ttl: CheckTTL, now: time.Now}
}

// HashPassword trả về SHA-256 (hex thường) của mật khẩu. Mật khẩu thô không bao giờ rời khỏi máy.
func HashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

// Login xác thực nhân viên. Founder không đăng nhập qua Sheet (được xử lý riêng ở Frontend).
func (s *Service) Login(ctx context.Context, username, password string) (*Response, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return &Response{Result: ResultBadRequest}, nil
	}
	if strings.EqualFold(username, "founder") {
		return &Response{Result: ResultWrongCredentials}, nil
	}

	resp, err := s.caller.Call(ctx, "login", map[string]any{
		"username": username,
		"passHash": HashPassword(password),
		"hwid":     HardwareID(),
	})
	if err != nil {
		s.clear()
		return nil, err
	}
	if resp.Result != ResultOK {
		s.clear()
		return resp, nil
	}

	s.mu.Lock()
	s.session = &Session{
		Username:       username,
		FullName:       resp.FullName,
		MaxTaxAccounts: resp.MaxTaxAccounts,
		AllowAI:        resp.AllowAI,
		EndDate:        resp.EndDate,
		VerifiedAt:     s.now(),
	}
	s.mu.Unlock()
	return resp, nil
}

// Check kiểm tra lại tài khoản đang đăng nhập (hết hạn, bị khóa, đổi máy...).
// Không bắt buộc (force=false) thì dùng lại kết quả trong CheckTTL.
// Lỗi kết nối trả về ErrNetwork: caller phải coi là không được dùng.
func (s *Service) Check(ctx context.Context, force bool) (*Response, error) {
	s.mu.Lock()
	cur := s.session
	if cur == nil {
		s.mu.Unlock()
		return nil, ErrNoSession
	}
	if !force && s.now().Sub(cur.VerifiedAt) < s.ttl {
		resp := responseFrom(cur)
		s.mu.Unlock()
		return resp, nil
	}
	username := cur.Username
	s.mu.Unlock()

	resp, err := s.caller.Call(ctx, "check", map[string]any{
		"username": username,
		"hwid":     HardwareID(),
	})
	if err != nil {
		return nil, err
	}
	if resp.Result != ResultOK {
		s.clear()
		return resp, nil
	}

	s.mu.Lock()
	// Phòng khi trong lúc chờ có người đăng xuất hoặc đăng nhập tài khoản khác
	if s.session != nil && s.session.Username == username {
		s.session.FullName = resp.FullName
		s.session.MaxTaxAccounts = resp.MaxTaxAccounts
		s.session.AllowAI = resp.AllowAI
		s.session.EndDate = resp.EndDate
		s.session.VerifiedAt = s.now()
	}
	s.mu.Unlock()
	return resp, nil
}

// Current trả về bản sao phiên hiện tại, hoặc nil nếu chưa đăng nhập.
func (s *Service) Current() *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return nil
	}
	cp := *s.session
	return &cp
}

// Logout xóa phiên.
func (s *Service) Logout() { s.clear() }

// RestoreSession nạp lại phiên từ bộ nhớ cục bộ máy khách (đăng nhập siêu tốc offline).
func (s *Service) RestoreSession(username, fullName string, maxTaxAccounts int, allowAI bool, endDate string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = &Session{
		Username:       username,
		FullName:       fullName,
		MaxTaxAccounts: maxTaxAccounts,
		AllowAI:        allowAI,
		EndDate:        endDate,
		VerifiedAt:     s.now(),
	}
}

func (s *Service) clear() {
	s.mu.Lock()
	s.session = nil
	s.mu.Unlock()
}

func responseFrom(sess *Session) *Response {
	return &Response{
		Result:         ResultOK,
		FullName:       sess.FullName,
		MaxTaxAccounts: sess.MaxTaxAccounts,
		AllowAI:        sess.AllowAI,
		EndDate:        sess.EndDate,
	}
}

// Describe trả về thông báo tiếng Việt cho người dùng.
func Describe(r *Response) string {
	switch r.Result {
	case ResultOK:
		return "Đăng nhập thành công."
	case ResultWrongCredentials:
		return "Tên đăng nhập hoặc mật khẩu không đúng!"
	case ResultPending:
		return "Tài khoản đang chờ Admin phê duyệt."
	case ResultLocked:
		return "Tài khoản đã bị khóa. Vui lòng liên hệ Admin."
	case ResultWrongDevice:
		return "Tài khoản này đã được cấp phép cho thiết bị khác."
	case ResultNotStarted:
		return fmt.Sprintf("Tài khoản chưa đến ngày sử dụng (%s).", r.StartDate)
	case ResultExpired:
		return fmt.Sprintf("Tài khoản đã hết hạn sử dụng (%s).", r.EndDate)
	case ResultExists:
		return "Tên đăng nhập này đã tồn tại."
	case ResultBadRequest:
		return "Thông tin gửi lên không hợp lệ."
	case ResultUnauthorized:
		return "Cấu hình máy chủ xác thực không đúng (sai khóa API)."
	default:
		if r.Message != "" {
			return r.Message
		}
		return "Có lỗi xảy ra, vui lòng thử lại sau."
	}
}

