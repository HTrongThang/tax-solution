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
namespace Frontend.Views
{
    public partial class MainForm : Form
    {
        public MainForm()
        {
            InitializeComponent();
            this.Size = new Size(900, 600);
            this.StartPosition = FormStartPosition.CenterScreen;
        }
        private async void login_Load(object sender, EventArgs e)
        {
            this.Opacity = 0;

            // 1. Khởi tạo WebView2
            await webView21.EnsureCoreWebView2Async();

            // 2. Thiết lập Mapping thư mục ảo (Giải quyết triệt để lỗi Unsafe/Security)
            string appDirectory = AppDomain.CurrentDomain.BaseDirectory;
            string webResourcesPath = Path.Combine(appDirectory, "WebResources");

            webView21.CoreWebView2.SetVirtualHostNameToFolderMapping(
                "taxscraper.app",
                webResourcesPath,
                CoreWebView2HostResourceAccessKind.Allow
            );

            // 3. Xử lý sau khi nạp trang xong (Ghi nhớ mật khẩu)
            webView21.NavigationCompleted += (s, args) =>
            {
                this.Opacity = 1;

                if (Properties.Settings.Default.RememberMe)
                {
                    string u = Properties.Settings.Default.UserName;
                    string encryptedPass = Properties.Settings.Default.PassWord;

                    try
                    {
                        // Giải mã mật khẩu (Sử dụng hàm AES của Thắng)
                        string p = GiaiMaAES(encryptedPass);

                        string fillScript = $@"
                    if(document.getElementById('user')) document.getElementById('user').value = '{u}';
                    if(document.getElementById('pass')) document.getElementById('pass').value = '{p}';
                    if(document.getElementById('remember')) document.getElementById('remember').checked = true;
                ";
                        webView21.CoreWebView2.ExecuteScriptAsync(fillScript);
                    }
                    catch (Exception ex)
                    {
                        Console.WriteLine("Lỗi giải mã: " + ex.Message);
                    }
                }
            };

            // 4. Lắng nghe tin nhắn từ Web gửi về C#
            webView21.CoreWebView2.WebMessageReceived += CoreWebView2_WebMessageReceived;

            // 5. ĐIỀU QUAN TRỌNG: Điều hướng bằng Domain ảo thay vì file:///
            // Hệ thống sẽ tự hiểu gốc là thư mục WebResources
            webView21.CoreWebView2.Navigate("https://taxscraper.app/login.html");
        }
        private async void CoreWebView2_WebMessageReceived(object sender, CoreWebView2WebMessageReceivedEventArgs e)
        {
            string NavigationCompletedjsonRaw = e.TryGetWebMessageAsString();
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
                        string founderHash = "0e398941913d4b5479819dd6fc8e35c168cd00c4b5a11f602d5b09f02cd61448";

                        bool isFounder = (user == "Founder" && hashedInput == founderHash);
                        bool isValidUser = ValidateUserFromJson(user, hashedInput);

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
                                LuuVaoSettings(user, MaHoaAES(pass));
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
                        string u = root.GetProperty("username").GetString();
                        string p = root.GetProperty("password").GetString();

                        // Hash mật khẩu trước khi lưu trữ để bảo mật
                        string hashedPass = ComputeHash(p);

                        // Thực thi lưu vào file JSON hệ thống
                        LuuUserVaoFileJson(u, hashedPass);

                        await webView21.CoreWebView2.ExecuteScriptAsync("showAlert('Thành công', 'Đã thêm tài khoản mới vào hệ thống!', 'success')");
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
        private void LuuUserVaoFileJson(string u, string p)
        {
            try
            {
                string path = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "users.json");
                List<dynamic> users = new List<dynamic>();

                if (File.Exists(path))
                {
                    string oldData = File.ReadAllText(path);
                    users = JsonSerializer.Deserialize<List<dynamic>>(oldData) ?? new List<dynamic>();
                }

                // Thêm user mới
                users.Add(new
                {
                    username = u,
                    password = p,
                    role = "Staff",
                    created_at = DateTime.Now.ToString("dd/MM/yyyy HH:mm")
                });

                string jsonString = JsonSerializer.Serialize(users, new JsonSerializerOptions { WriteIndented = true });
                File.WriteAllText(path, jsonString);

                // Bắn thông báo về giao diện Web của Thắng
                webView21.CoreWebView2.ExecuteScriptAsync($"showAlert('Thành công', 'Đã cấp quyền cho {u}', 'success')");
            }
            catch (Exception ex)
            {
                MessageBox.Show("Lỗi lưu user: " + ex.Message);
            }
        }
        private void NavigateToPage(string folder, string fileName)
        {
            // Xác định đường dẫn thư mục gốc của App
            string appDirectory = AppDomain.CurrentDomain.BaseDirectory;

            // Kết hợp đường dẫn đến file HTML trong folder WebResources
            // Ví dụ: WebResources/Admin/manage_users.html
            string filePath = Path.Combine(appDirectory, "WebResources", folder, fileName);

            if (File.Exists(filePath))
            {
                // Nếu file tồn tại, bảo WebView2 mở nó lên
                webView21.CoreWebView2.Navigate(new Uri(filePath).AbsoluteUri);
            }
            else
            {
                // Nếu không thấy file, báo lỗi để Thắng kiểm tra lại folder
                MessageBox.Show($"Lỗi: Không tìm thấy file tại đường dẫn: {filePath}", "Lỗi file");
            }
        }
        private bool ValidateUserFromJson(string user, string pass)
        {
            string filePath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "users.json");

            // Nếu file chưa tồn tại (chưa có ai được tạo) thì trả về false luôn
            if (!File.Exists(filePath)) return false;

            string json = File.ReadAllText(filePath);
            using (JsonDocument doc = JsonDocument.Parse(json))
            {
                // Duyệt qua danh sách user trong file JSON
                foreach (var item in doc.RootElement.EnumerateArray())
                {
                    if (item.GetProperty("username").GetString() == user &&
                        item.GetProperty("password").GetString() == pass)
                    {
                        return true; // Khớp tài khoản!
                    }
                }
            }
            return false; // Không tìm thấy hoặc sai pass
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
        public static string MaHoaAES(string plainText)
        {
            string key = "1234567890123456"; // Phải đủ 16 ký tự (128 bit)
            string iv = "abcdefghijklmnop";  // Phải đủ 16 ký tự

            using (Aes aesAlg = Aes.Create())
            {
                aesAlg.Key = Encoding.UTF8.GetBytes(key);
                aesAlg.IV = Encoding.UTF8.GetBytes(iv);

                ICryptoTransform encryptor = aesAlg.CreateEncryptor(aesAlg.Key, aesAlg.IV);

                using (MemoryStream msEncrypt = new MemoryStream())
                {
                    using (CryptoStream csEncrypt = new CryptoStream(msEncrypt, encryptor, CryptoStreamMode.Write))
                    {
                        using (StreamWriter swEncrypt = new StreamWriter(csEncrypt))
                        {
                            swEncrypt.Write(plainText);
                        }
                        return Convert.ToBase64String(msEncrypt.ToArray());
                    }
                }
            }
        }
        public static string GiaiMaAES(string cipherText)
        {
            string key = "1234567890123456"; // Trùng với hàm mã hóa
            string iv = "abcdefghijklmnop";  // Trùng với hàm mã hóa

            using (Aes aesAlg = Aes.Create())
            {
                aesAlg.Key = Encoding.UTF8.GetBytes(key);
                aesAlg.IV = Encoding.UTF8.GetBytes(iv);

                ICryptoTransform decryptor = aesAlg.CreateDecryptor(aesAlg.Key, aesAlg.IV);

                using (MemoryStream msDecrypt = new MemoryStream(Convert.FromBase64String(cipherText)))
                {
                    using (CryptoStream csDecrypt = new CryptoStream(msDecrypt, decryptor, CryptoStreamMode.Read))
                    {
                        using (StreamReader srDecrypt = new StreamReader(csDecrypt))
                        {
                            return srDecrypt.ReadToEnd();
                        }
                    }
                }
            }
        }
    }
}
