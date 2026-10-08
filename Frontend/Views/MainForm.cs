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

        public MainForm()
        {
            InitializeComponent();
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
                        bool isValidUser = await _databaseService.ValidateUserAsync(user, hashedInput);

                        if (isFounder || isValidUser)
                        {
                            // A. Lưu vào Session của C# (Để xử lý Backend)
                            UserSession.FullName = user;
                            UserSession.Role = isFounder ? "ADMINISTRATOR" : "STAFF";
                            string initials = string.IsNullOrEmpty(user) ? "U" : user.Substring(0, 1).ToUpper();

                            // B. Lưu vào LocalStorage của trình duyệt (Để giao diện nạp nhanh 0ms)
                            // Sử dụng dấu backtick (`) để bao bọc giá trị chuỗi an toàn
                            string syncScript = $@"
                    localStorage.setItem('user_name', `{UserSession.FullName}`);
                    localStorage.setItem('user_role', `{UserSession.Role}`);
                    localStorage.setItem('user_initials', `{initials}`);
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
                            // Thông báo lỗi nếu thông tin đăng nhập không chính xác
                            await webView21.CoreWebView2.ExecuteScriptAsync("showAlert('Lỗi', 'Tên đăng nhập hoặc mật khẩu không đúng!', 'error')");
                        }
                    }

                    // --- 2. XỬ LÝ ADMIN THÊM TÀI KHOẢN MỚI ---
                    else if (action == "ADMIN_THEM_USER")
                    {
                        string u = root.GetProperty("username").GetString()?.Trim();
                        string p = root.GetProperty("password").GetString()?.Trim();

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

                        // Kiểm tra trùng tên nhân viên trong SQLite
                        if (await _databaseService.UserExistsAsync(u))
                        {
                            await webView21.CoreWebView2.ExecuteScriptAsync("showAlert('Lỗi', 'Tên đăng nhập này đã tồn tại trong hệ thống!', 'warning')");
                            return;
                        }

                        // Hash mật khẩu trước khi lưu trữ để bảo mật
                        string hashedPass = ComputeHash(p);

                        // Thực thi lưu vào database SQLite
                        await _databaseService.AddUserAsync(u, hashedPass, "Staff");

                        string safeName = Newtonsoft.Json.JsonConvert.ToString(u);
                        await webView21.CoreWebView2.ExecuteScriptAsync($"showAlert('Thành công', 'Đã cấp quyền cho tài khoản ' + {safeName}, 'success')");
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
    }
}
