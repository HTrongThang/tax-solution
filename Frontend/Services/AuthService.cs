using System.Net.Http;
using System.Text;
using System.Text.Json;

namespace Frontend.Services
{
    /// <summary>Kết quả đăng nhập/kiểm tra do Backend trả về.</summary>
    public class AuthResult
    {
        /// <summary>true khi Backend xác nhận tài khoản hợp lệ (result = OK).</summary>
        public bool Success { get; set; }

        /// <summary>Mã kết quả: OK, WRONG_CREDENTIALS, PENDING, LOCKED, WRONG_DEVICE, NOT_STARTED, EXPIRED,
        /// NETWORK_ERROR, NOT_CONFIGURED, NO_SESSION, BACKEND_UNREACHABLE, ERROR...</summary>
        public string Result { get; set; } = "ERROR";

        /// <summary>Thông báo tiếng Việt để hiển thị cho người dùng.</summary>
        public string Message { get; set; } = "";

        public string Username { get; set; } = "";
        public string FullName { get; set; } = "";
        public int MaxTaxAccounts { get; set; }
        public bool AllowAI { get; set; }
        public string EndDate { get; set; } = "";
        public string Hwid { get; set; } = "";
    }

    /// <summary>
    /// Gọi API xác thực của Backend (/api/auth/*). Backend lo việc nói chuyện với Google Sheet.
    /// Mất mạng hoặc Backend không chạy => không thành công => không cho dùng phần mềm.
    /// </summary>
    public class AuthService
    {
        private static readonly HttpClient _http = new HttpClient
        {
            BaseAddress = new Uri("http://127.0.0.1:18386"),
            Timeout = TimeSpan.FromSeconds(25) // Backend tự giới hạn 15s khi gọi Google
        };

        /// <summary>Đăng nhập nhân viên. Mật khẩu được Backend băm trước khi gửi đi.</summary>
        public Task<AuthResult> LoginAsync(string username, string password) =>
            PostAsync("/api/auth/login", new { username, password });

        /// <summary>Khách gửi yêu cầu cấp tài khoản mới (lưu trạng thái PENDING + mã máy hiện tại lên Google Sheet).</summary>
        public Task<AuthResult> RegisterAsync(string username, string password, string fullName) =>
            PostAsync("/api/auth/register", new { username, password, fullName });

        /// <summary>Kiểm tra lại tài khoản đang đăng nhập (hết hạn, bị khóa, đổi máy...).</summary>
        public Task<AuthResult> CheckAsync(bool force = false) =>
            PostAsync("/api/auth/check", new { force });

        public Task<AuthResult> LogoutAsync() =>
            PostAsync("/api/auth/logout", new { });

        /// <summary>Lấy mã máy (Hardware ID) của thiết bị hiện tại từ Backend.</summary>
        public async Task<string> GetHwidAsync()
        {
            try
            {
                using var res = await _http.GetAsync("/api/auth/hwid");
                string body = await res.Content.ReadAsStringAsync();
                var parsed = Parse(body);
                return parsed.Hwid;
            }
            catch (Exception ex) when (ex is HttpRequestException || ex is TaskCanceledException)
            {
                return "";
            }
        }

        /// <summary>
        /// Founder cấp tài khoản nhân viên (ghi lên Google Sheet). adminToken là SHA-256 mật khẩu Founder.
        /// Mật khẩu được Backend băm trước khi gửi lên Google.
        /// </summary>
        public Task<AuthResult> AdminCreateAsync(string adminToken, string username, string password, string fullName,
            string hwid, int maxTaxAccounts, bool allowAI, string startDate, string endDate) =>
            PostAsync("/api/auth/admin/create", new
            {
                adminToken, username, password, fullName, hwid, maxTaxAccounts, allowAI, startDate, endDate
            });

        private static async Task<AuthResult> PostAsync(string path, object payload)
        {
            try
            {
                using var content = new StringContent(JsonSerializer.Serialize(payload), Encoding.UTF8, "application/json");
                using var res = await _http.PostAsync(path, content);
                string body = await res.Content.ReadAsStringAsync();
                return Parse(body);
            }
            catch (Exception ex) when (ex is HttpRequestException || ex is TaskCanceledException)
            {
                return new AuthResult
                {
                    Result = "BACKEND_UNREACHABLE",
                    Message = "Không kết nối được dịch vụ nền của phần mềm. Vui lòng khởi động lại ứng dụng."
                };
            }
        }

        internal static AuthResult Parse(string body)
        {
            try
            {
                using var doc = JsonDocument.Parse(body);
                var root = doc.RootElement;

                string status = root.TryGetProperty("status", out var s) ? s.GetString() ?? "" : "";
                // Một số API (vd. logout) chỉ trả status=success, không có result
                string result = root.TryGetProperty("result", out var r)
                    ? r.GetString() ?? "ERROR"
                    : (status == "success" ? "OK" : "ERROR");
                var ar = new AuthResult
                {
                    Success = status == "success" && result == "OK",
                    Result = result,
                    Message = root.TryGetProperty("message", out var m) ? m.GetString() ?? "" : ""
                };

                if (root.TryGetProperty("data", out var d) && d.ValueKind == JsonValueKind.Object)
                {
                    ar.Username = Str(d, "username");
                    ar.FullName = Str(d, "fullName");
                    ar.EndDate = Str(d, "endDate");
                    ar.Hwid = Str(d, "hwid");
                    ar.AllowAI = d.TryGetProperty("allowAI", out var ai) && ai.ValueKind == JsonValueKind.True;
                    ar.MaxTaxAccounts = d.TryGetProperty("maxTaxAccounts", out var mt) && mt.TryGetInt32(out var n) ? n : 0;
                }
                return ar;
            }
            catch (JsonException)
            {
                return new AuthResult { Result = "ERROR", Message = "Phản hồi từ dịch vụ nền không hợp lệ." };
            }
        }

        private static string Str(JsonElement e, string name) =>
            e.TryGetProperty(name, out var v) && v.ValueKind == JsonValueKind.String ? v.GetString() ?? "" : "";
    }
}
