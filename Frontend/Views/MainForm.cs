using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Data;
using System.Drawing;
using System.Linq;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
using Microsoft.Web.WebView2.Core;
using System.Text.Json;
using System.IO;
using System.Security.Cryptography;
using System.Text;
using Frontend.Models;
using Frontend.Services;
namespace Frontend.Views
{
    public partial class MainForm : Form
    {
        private readonly DatabaseService _databaseService = new DatabaseService();
        private readonly AuthService _authService = new AuthService();

        public MainForm()
        {
            InitializeComponent();
            try
            {
                this.Icon = Icon.ExtractAssociatedIcon(Application.ExecutablePath);
            }
            catch { }
            this.Text = $"TaxSolution - Đăng nhập ({AppVersionHelper.DisplayVersion})";
            this.Size = new Size(900, 600);
            this.StartPosition = FormStartPosition.CenterScreen;
        }
        private async void login_Load(object sender, EventArgs e)
        {
            this.Opacity = 0;

            // Khởi tạo SQLite DB (tự tạo bảng Users và TaxAccounts nếu chưa có)
            await _databaseService.InitializeAsync();

            // 1. Khởi tạo WebView2 (lưu cache vào AppData để tránh lỗi quyền)
            var userDataFolder = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "TaxSolution", "WebView2");
            var env = await CoreWebView2Environment.CreateAsync(null, userDataFolder);
            await webView21.EnsureCoreWebView2Async(env);
            await webView21.CoreWebView2.AddScriptToExecuteOnDocumentCreatedAsync(
                $"window.appVersion = '{AppVersionHelper.DisplayVersion}'; localStorage.setItem('app_version', '{AppVersionHelper.DisplayVersion}');"
            );

            // 2. Thiết lập Mapping thư mục ảo (Giải quyết triệt để lỗi Unsafe/Security)
            string appDirectory = AppDomain.CurrentDomain.BaseDirectory;
            string webResourcesPath = Path.Combine(appDirectory, "WebResources");

            webView21.CoreWebView2.SetVirtualHostNameToFolderMapping(
                "taxscraper.app",
                webResourcesPath,
                CoreWebView2HostResourceAccessKind.Allow
            );

            // 3. Xử lý sau khi nạp trang xong (Ghi nhớ mật khẩu & Phiên bản)
            webView21.NavigationCompleted += (s, args) =>
            {
                this.Opacity = 1;

                // Tự động đồng bộ số phiên bản vào giao diện HTML
                string verScript = $"document.querySelectorAll('.app-version-text').forEach(el => el.innerText = '{AppVersionHelper.DisplayVersion}');";
                webView21.CoreWebView2.ExecuteScriptAsync(verScript);
                _ = FillHwidToWebViewAsync();

                if (Properties.Settings.Default.RememberMe)
                {
                    string u = Properties.Settings.Default.UserName;
                    string encryptedPass = Properties.Settings.Default.PassWord;

                    try
                    {
                        // Giải mã mật khẩu bằng DPAPI (chỉ user Windows hiện tại mới giải được)
                        string p = UnprotectPassword(encryptedPass);

                        string fillScript = $@"
                    if(document.getElementById('user')) document.getElementById('user').value = '{u}';
                    if(document.getElementById('pass')) document.getElementById('pass').value = '{p}';
                    if(document.getElementById('remember')) document.getElementById('remember').checked = true;
                ";
                        webView21.CoreWebView2.ExecuteScriptAsync(fillScript);
                    }
                    catch (Exception ex)
                    {
                        // Nếu giải mã thất bại (ví dụ: dữ liệu cũ dùng AES), xóa settings để user nhập lại
                        Console.WriteLine("Lỗi giải mã (có thể do dữ liệu cũ): " + ex.Message);
                        XoaSettings();
                    }
                }
            };

            // 4. Lắng nghe tin nhắn từ Web gửi về C#
            webView21.CoreWebView2.WebMessageReceived += CoreWebView2_WebMessageReceived;

            // 5. ĐIỀU QUAN TRỌNG: Điều hướng bằng Domain ảo thay vì file:///
            // Hệ thống sẽ tự hiểu gốc là thư mục WebResources
            webView21.CoreWebView2.Navigate("https://taxscraper.app/login.html");

            // 6. Kiểm tra phiên bản mới ngầm trên mạng (không gây đơ UI)
            _ = Task.Run(async () =>
            {
                var updateResult = await UpdateService.CheckForUpdateAsync();
                if (updateResult.Success && updateResult.HasUpdate)
                {
                    this.Invoke(new Action(() =>
                    {
                        var dlg = new UpdateDialogForm(updateResult);
                        dlg.ShowDialog(this);
                    }));
                }
            });
        }
        private async void CoreWebView2_WebMessageReceived(object sender, CoreWebView2WebMessageReceivedEventArgs e)
        {
            string jsonRaw = e.TryGetWebMessageAsString();
            try
            {
                using (JsonDocument doc = JsonDocument.Parse(jsonRaw))
                {
                    JsonElement root = doc.RootElement;
                    string action = root.GetProperty("action").GetString();

                    // --- 0. LẤY MÃ MÁY HIỆN TẠI (HWID) ---
                    if (action == "GET_HWID")
                    {
                        _ = FillHwidToWebViewAsync();
                        return;
                    }
                    // --- 0b. KHÁCH GỬI YÊU CẦU CẤP TÀI KHOẢN (REGISTER_REQUEST) ---
                    if (action == "REGISTER_REQUEST")
                    {
                        string u = root.TryGetProperty("username", out var uEl) ? uEl.GetString()?.Trim() ?? "" : "";
                        string p = root.TryGetProperty("password", out var pEl) ? pEl.GetString()?.Trim() ?? "" : "";
                        string fullName = root.TryGetProperty("fullName", out var fnEl) ? fnEl.GetString()?.Trim() ?? "" : "";

                        if (string.IsNullOrEmpty(u) || string.IsNullOrEmpty(p))
                        {
                            await webView21.CoreWebView2.ExecuteScriptAsync(
                                "showAlert('Thiếu thông tin', 'Vui lòng nhập đầy đủ Tên đăng nhập và Mật khẩu!', 'warning')");
                            return;
                        }

                        var regResult = await _authService.RegisterAsync(u, p, fullName);
                        if (regResult.Success)
                        {
                            string msg = string.IsNullOrEmpty(regResult.Message)
                                ? "Đã gửi yêu cầu cấp tài khoản! Vui lòng liên hệ Admin (Zalo: 0356.587.127) để được kích hoạt."
                                : regResult.Message;
                            await webView21.CoreWebView2.ExecuteScriptAsync(
                                $"if(window.dongFormDangKy) dongFormDangKy(); showAlert('Đã gửi yêu cầu', {Newtonsoft.Json.JsonConvert.ToString(msg)}, 'success')");
                        }
                        else
                        {
                            string msg = string.IsNullOrEmpty(regResult.Message)
                                ? "Không gửi được yêu cầu. Vui lòng thử lại."
                                : regResult.Message;
                            await webView21.CoreWebView2.ExecuteScriptAsync(
                                $"showAlert('Không thể đăng ký', {Newtonsoft.Json.JsonConvert.ToString(msg)}, 'warning')");
                        }
                        return;
                    }
                    // --- 1. XỬ LÝ ĐĂNG NHẬP (LOGIN) ---
                    if (action == "Login")
                    {
                        string user = root.GetProperty("username").GetString();
                        string pass = root.GetProperty("password").GetString();
                        bool remember = root.GetProperty("remember").GetBoolean();

                        // Kiểm tra mã băm mật khẩu
                        string hashedInput = ComputeHash(pass);
                        string founderHash = "6fae104d9d8828ab82bf0842c2ba35465ebe7b6c4501beacde8735325b330035"; 

                        bool isFounder = (string.Equals(user, "Founder", StringComparison.OrdinalIgnoreCase) && hashedInput == founderHash);
                        AuthResult authResult = null;
                        bool isValidUser = false;

                        if (!isFounder)
                        {
                            // A. Kiểm tra nhanh trong SQLite cục bộ máy này (Chỉ duy nhất tài khoản này, mở 0ms)
                            var localAcc = await _databaseService.GetLocalAccountAsync();
                            string todayStr = DateTime.Now.ToString("yyyy-MM-dd");

                            bool isSameUser = localAcc != null && string.Equals(localAcc.Username, user, StringComparison.OrdinalIgnoreCase);
                            bool isPassMatch = isSameUser && localAcc.PasswordHash == hashedInput;
                            bool isActive = isSameUser && localAcc.Status == "ACTIVE";
                            bool isEndValid = isSameUser && IsEndDateValid(localAcc.EndDate);

                            if (isSameUser && isPassMatch && isActive && isEndValid)
                            {
                                // 1. Cho vào ngay lập tức từ SQLite (0ms)!
                                isValidUser = true;
                                authResult = new AuthResult
                                {
                                    Success = true,
                                    Result = "OK",
                                    Username = localAcc.Username,
                                    FullName = string.IsNullOrEmpty(localAcc.FullName) ? localAcc.Username : localAcc.FullName,
                                    MaxTaxAccounts = localAcc.MaxTaxAccounts,
                                    AllowAI = localAcc.AllowAI,
                                    EndDate = localAcc.EndDate
                                };

                                // Khôi phục phiên ngầm cho Backend Go (1ms)
                                _ = _authService.RestoreSessionAsync(localAcc.Username, localAcc.FullName, localAcc.MaxTaxAccounts, localAcc.AllowAI, localAcc.EndDate);
                            }
                            else
                            {
                                // 2. Chưa có trong SQLite hoặc mật khẩu mới / đổi tài khoản -> Bắt buộc gọi Google Sheet xác thực
                                authResult = await _authService.LoginAsync(user, pass);
                                isValidUser = authResult != null && authResult.Success;

                                if (isValidUser)
                                {
                                    // Lưu thông tin của duy nhất tài khoản này vào SQLite
                                    await _databaseService.SaveLocalAccountAsync(new LocalAccountModel
                                    {
                                        Username = user,
                                        FullName = authResult.FullName,
                                        Role = "Staff",
                                        MaxTaxAccounts = authResult.MaxTaxAccounts,
                                        AllowAI = authResult.AllowAI,
                                        EndDate = authResult.EndDate,
                                        Status = "ACTIVE",
                                        PasswordHash = hashedInput,
                                        LastCheckedDate = todayStr
                                    });
                                }
                            }
                        }

                        if (isFounder || isValidUser)
                        {
                            // A. Lưu vào Session của C# (Để xử lý Backend)
                            UserSession.Username = user;
                            UserSession.FullName = authResult != null && !string.IsNullOrEmpty(authResult.FullName) ? authResult.FullName : user;
                            UserSession.Role = isFounder ? "ADMINISTRATOR" : "STAFF";
                            UserSession.Status = "ACTIVE";
                            UserSession.AdminToken = isFounder ? hashedInput : "";
                            UserSession.MaxTaxAccounts = isFounder ? int.MaxValue : authResult.MaxTaxAccounts;
                            UserSession.AllowAI = isFounder || authResult.AllowAI;
                            UserSession.EndDate = isFounder ? "" : authResult.EndDate;
                            string initials = string.IsNullOrEmpty(user) ? "U" : user.Substring(0, 1).ToUpper();

                            // B. Lưu vào LocalStorage của trình duyệt (Để giao diện nạp nhanh 0ms)
                            // Dùng JsonConvert.ToString để escape an toàn (tránh chèn mã JS qua tên đăng nhập)
                            string syncScript = $@"
                    localStorage.setItem('user_name', {Newtonsoft.Json.JsonConvert.ToString(UserSession.FullName)});
                    localStorage.setItem('user_role', {Newtonsoft.Json.JsonConvert.ToString(UserSession.Role)});
                    localStorage.setItem('user_initials', {Newtonsoft.Json.JsonConvert.ToString(initials)});
                    console.log('TaxSolution: User data synced to LocalStorage.');
                ";

                            // Đợi trình duyệt lưu xong dữ liệu mới tiếp tục
                            await webView21.CoreWebView2.ExecuteScriptAsync(syncScript);

                            // C. Xử lý ghi nhớ mật khẩu (Lưu xuống file settings)
                            if (remember)
                            {
                                LuuVaoSettings(user, ProtectPassword(pass));
                            }
                            else
                            {
                                XoaSettings();
                            }

                            // D. Điều hướng giao diện
                            this.Invoke(new Action(() => {
                                if (isFounder)
                                {
                                    // Chuyển hướng ngay trên WebView hiện tại sang trang quản trị
                                    // Trang Admin có nhiều ô hơn trang Login: phóng to (không vượt quá màn hình) và căn giữa
                                    var wa = Screen.FromControl(this).WorkingArea;
                                    this.Size = new Size(Math.Min(1000, wa.Width - 40), Math.Min(820, wa.Height - 40));
                                    this.Location = new Point(wa.Left + (wa.Width - this.Width) / 2, wa.Top + (wa.Height - this.Height) / 2);
                                    webView21.CoreWebView2.Navigate("https://taxscraper.app/Admin/manage_users.html");
                                }
                                else
                                {
                                    // Khởi tạo và hiển thị Form làm việc của nhân viên
                                    UserForm fUser = new UserForm();

                                    // Đóng toàn bộ ứng dụng khi người dùng tắt Form chính
                                    fUser.FormClosed += (s, args) => { this.Close(); };

                                    fUser.Show();
                                    this.Hide(); // Ẩn Form Login
                                }
                            }));
                        }
                        else
                        {
                            // Thông báo đúng nguyên nhân: sai mật khẩu, chờ duyệt, bị khóa, hết hạn, sai máy, mất mạng...
                            string loginMsg = authResult != null && !string.IsNullOrEmpty(authResult.Message)
                                ? authResult.Message
                                : "Tên đăng nhập hoặc mật khẩu không đúng!";
                            string alertType = authResult == null || authResult.Result == "WRONG_CREDENTIALS" ? "error" : "warning";
                            await webView21.CoreWebView2.ExecuteScriptAsync(
                                $"showAlert('Không thể đăng nhập', {Newtonsoft.Json.JsonConvert.ToString(loginMsg)}, '{alertType}')");
                        }
                    }

                    // --- 2. XỬ LÝ ADMIN THÊM TÀI KHOẢN MỚI ---
                    else if (action == "ADMIN_THEM_USER")
                    {
                        string u = root.GetProperty("username").GetString()?.Trim();
                        string p = root.GetProperty("password").GetString()?.Trim();

                        // Chỉ Founder đã đăng nhập mới được cấp tài khoản
                        if (UserSession.Role != "ADMINISTRATOR" || string.IsNullOrEmpty(UserSession.AdminToken))
                        {
                            await webView21.CoreWebView2.ExecuteScriptAsync("showAlert('Lỗi', 'Bạn không có quyền thực hiện thao tác này!', 'error')");
                            return;
                        }

                        if (string.IsNullOrEmpty(u) || string.IsNullOrEmpty(p))
                        {
                            await webView21.CoreWebView2.ExecuteScriptAsync("showAlert('Lỗi', 'Thông tin tài khoản không được để trống!', 'warning')");
                            return;
                        }

                        // Không cho phép đặt trùng tên với Quản trị viên tối cao (Founder)
                        if (string.Equals(u, "Founder", StringComparison.OrdinalIgnoreCase))
                        {
                            await webView21.CoreWebView2.ExecuteScriptAsync("showAlert('Lỗi', 'Tên đăng nhập này không được sử dụng!', 'error')");
                            return;
                        }

                        // Thông tin mở rộng của tài khoản
                        string fullName = root.TryGetProperty("fullName", out var fnEl) ? fnEl.GetString()?.Trim() ?? "" : "";
                        string hwid = root.TryGetProperty("hwid", out var hwEl) ? hwEl.GetString()?.Trim() ?? "" : "";
                        string startDate = root.TryGetProperty("startDate", out var sdEl) ? sdEl.GetString()?.Trim() ?? "" : "";
                        string endDate = root.TryGetProperty("endDate", out var edEl) ? edEl.GetString()?.Trim() ?? "" : "";
                        bool allowAi = root.TryGetProperty("allowAI", out var aiEl) && aiEl.ValueKind == JsonValueKind.True;
                        int maxTax = root.TryGetProperty("maxTaxAccounts", out var mtEl) && mtEl.TryGetInt32(out var mt) && mt >= 0 ? mt : 5;

                        // Ghi lên Google Sheet qua Backend (trạng thái ACTIVE)
                        var createResult = await _authService.AdminCreateAsync(
                            UserSession.AdminToken, u, p, fullName, hwid, maxTax, allowAi, startDate, endDate);

                        if (createResult.Success)
                        {
                            string safeName = Newtonsoft.Json.JsonConvert.ToString(u);
                            await webView21.CoreWebView2.ExecuteScriptAsync(
                                $"if(window.setSubmitLoading) setSubmitLoading(false); if(window.lamMoiForm) lamMoiForm(); showAlert('Thành công', 'Đã cấp quyền thành công cho tài khoản ' + {safeName} + '! Đang quay lại trang đăng nhập...', 'success'); setTimeout(function() {{ if(window.quayLaiDangNhap) quayLaiDangNhap(); }}, 1500);");
                        }
                        else
                        {
                            string msg = string.IsNullOrEmpty(createResult.Message) ? "Không cấp được tài khoản. Vui lòng thử lại." : createResult.Message;
                            await webView21.CoreWebView2.ExecuteScriptAsync(
                                $"if(window.setSubmitLoading) setSubmitLoading(false); showAlert('Không cấp được tài khoản', {Newtonsoft.Json.JsonConvert.ToString(msg)}, 'warning')");
                        }
                    }

                    // --- 3. QUAY LẠI TRANG ĐĂNG NHẬP ---
                    else if (action == "NAVIGATE_TO_LOGIN")
                    {
                        this.Invoke(new Action(() =>
                        {
                            this.Size = new Size(900, 600);
                            this.CenterToScreen();
                            webView21.CoreWebView2.Navigate("https://taxscraper.app/login.html");
                        }));
                    }
                }
            }
            catch (Exception ex)
            {
                // Ghi nhận lỗi và hiển thị cảnh báo để dễ dàng bảo trì
                Console.WriteLine("Lỗi tại WebMessageReceived: " + ex.Message);
                MessageBox.Show("Hệ thống gặp sự cố khi xử lý dữ liệu: " + ex.Message, "Thông báo lỗi", MessageBoxButtons.OK, MessageBoxIcon.Error);
            }
        }
        // Hàm này để lưu thông tin khi người dùng tích vào checkbox
        private void LuuVaoSettings(string u, string p)
        {
            // Sửa lại cho khớp với hình Thắng chụp: UserName, PassWord, RememberMe
            Properties.Settings.Default.UserName = u;
            Properties.Settings.Default.PassWord = p;
            Properties.Settings.Default.RememberMe = true;
            Properties.Settings.Default.Save();
        }

        private void XoaSettings()
        {
            Properties.Settings.Default.RememberMe = false;
            Properties.Settings.Default.UserName = "";
            Properties.Settings.Default.PassWord = "";
            Properties.Settings.Default.Save();
        }

        /// <summary>
        /// Kiểm tra ngày hết hạn có còn hiệu lực không (tính theo ngày hiện tại của máy).
        /// </summary>
        public static bool IsEndDateValid(string endDate)
        {
            if (string.IsNullOrWhiteSpace(endDate)) return true;
            if (DateTime.TryParseExact(endDate.Trim(), new[] { "dd/MM/yyyy", "d/M/yyyy", "yyyy-MM-dd" },
                System.Globalization.CultureInfo.InvariantCulture, System.Globalization.DateTimeStyles.None, out var dt))
            {
                return DateTime.Today <= dt.Date;
            }
            return true;
        }

        public string ComputeHash(string rawData)
        {
            using (SHA256 sha256Hash = SHA256.Create())
            {
                byte[] bytes = sha256Hash.ComputeHash(Encoding.UTF8.GetBytes(rawData));
                StringBuilder builder = new StringBuilder();
                for (int i = 0; i < bytes.Length; i++)
                {
                    builder.Append(bytes[i].ToString("x2"));
                }
                return builder.ToString();
            }
        }
        /// <summary>
        /// Mã hóa mật khẩu bằng Windows DPAPI thông qua SecurityHelper.
        /// </summary>
        public static string ProtectPassword(string plainText) => SecurityHelper.ProtectPassword(plainText);

        /// <summary>
        /// Giải mã mật khẩu bằng DPAPI thông qua SecurityHelper.
        /// </summary>
        public static string UnprotectPassword(string cipherText) => SecurityHelper.UnprotectPassword(cipherText);

        private async Task FillHwidToWebViewAsync()
        {
            try
            {
                string hwid = await _authService.GetHwidAsync();
                if (string.IsNullOrEmpty(hwid) || webView21?.CoreWebView2 == null) return;
                string safeHwid = Newtonsoft.Json.JsonConvert.ToString(hwid);
                string script = $"window.hardwareId = {safeHwid}; localStorage.setItem('hardware_id', {safeHwid}); var hwInput = document.getElementById('new_hwid'); if (hwInput && !hwInput.value) hwInput.value = {safeHwid}; var regHwInput = document.getElementById('reg_hwid'); if (regHwInput) regHwInput.value = {safeHwid};";
                await webView21.CoreWebView2.ExecuteScriptAsync(script);
            }
            catch { }
        }
    }
}
