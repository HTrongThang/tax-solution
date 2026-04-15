using Frontend.Models;
using Microsoft.Web.WebView2.Core;
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Data;
using System.Drawing;
using System.Linq;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Xml;
using Newtonsoft.Json;
using Frontend.Services;
using System.IO;
using Formatting = Newtonsoft.Json.Formatting;
namespace Frontend.Views
{
    public partial class UserForm : Form
    {
        private readonly TaxService _taxService = new TaxService();
        // Thêm dòng này để lưu dữ liệu công ty vào RAM
        private List<CompanyModel> _cachedCompanies = new List<CompanyModel>();
        private readonly string filePath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "tax_accounts.json");
        public UserForm()
        {
            InitializeComponent();
            this.Load += UserForm_Load;
            this.FormClosed += UserForm_FormClosed;
            LoadCompaniesToRAM();
        }
        private async void UserForm_Load(object sender, EventArgs e)
        {
            this.Opacity = 0;

            // 1. Căn chỉnh kích thước
            Rectangle screenRect = Screen.PrimaryScreen.WorkingArea;
            this.Size = new Size((int)(screenRect.Width * 0.85), (int)(screenRect.Height * 0.85));
            this.StartPosition = FormStartPosition.Manual; // Chỉnh lại để CenterScreen ăn theo Location
            this.Location = new Point((screenRect.Width - this.Width) / 2, (screenRect.Height - this.Height) / 2);

            // 2. Khởi tạo WebView2
            await webViewUser.EnsureCoreWebView2Async();

            // 3. Cấu hình Mapping (Đảm bảo folder WebResources tồn tại)
            string webResourcesPath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "WebResources");

            webViewUser.CoreWebView2.SetVirtualHostNameToFolderMapping(
                "taxscraper.app",
                webResourcesPath,
                CoreWebView2HostResourceAccessKind.Allow
            );

            // 4. ĐĂNG KÝ SỰ KIỆN TRƯỚC KHI NAVIGATE
            webViewUser.CoreWebView2.WebMessageReceived += CoreWebView2_WebMessageReceived;

            webViewUser.NavigationCompleted += async (s, args) => {
                if (args.IsSuccess)
                {
                    await SyncHeaderWithSession();
                    try
                    {
                        string response = await _taxService.Init();

                        if (!string.IsNullOrEmpty(response))
                        {
                            // Parse JSON trả về từ Go
                            dynamic result = JsonConvert.DeserializeObject(response);
                            string status = result?.status;
                            string message = result?.message;

                            if (status == "success")
                            {
                                // Nếu thành công, log ra để debug
                                System.Diagnostics.Debug.WriteLine("Khởi tạo thành công: " + message);
                            }
                            else
                            {
                                // Nếu Go trả về lỗi (status: "error")
                                await webViewUser.CoreWebView2.ExecuteScriptAsync(
                                    $"window.showAlert('Lỗi hệ thống', '{message}', 'error')"
                                );
                            }
                        }
                    }
                    catch (Exception ex)
                    {
                        // Lỗi này xảy ra khi Backend chưa bật hoặc sập (Connection Refused)
                        System.Diagnostics.Debug.WriteLine("Lỗi kết nối Backend: " + ex.Message);

                        await webViewUser.CoreWebView2.ExecuteScriptAsync(
                            $"window.showAlert('Lỗi kết nối', 'Không thể kết nối tới Service (localhost:8080). Vui lòng kiểm tra Backend!', 'error')"
                        );
                    }
                    this.Opacity = 1; // Chỉ hiện khi đã nạp xong và đồng bộ header
                }
                else
                {
                    MessageBox.Show($"Lỗi nạp trang: {args.WebErrorStatus}", "Lỗi hệ thống");
                    this.Opacity = 1; // Vẫn hiện để người dùng thấy lỗi mà xử lý
                }
            };

            // 5. ĐIỀU HƯỚNG (Kiểm tra kỹ chữ User viết hoa/thường)
            // Nếu trong folder là "User" thì giữ nguyên, nếu là "user" thì phải sửa lại
            webViewUser.CoreWebView2.Navigate("https://taxscraper.app/User/dashboard.html");
        }
        private async Task SyncHeaderWithSession()
        {
            // 1. Lấy thông tin từ Session
            string name = UserSession.FullName ?? "Người dùng";
            string role = UserSession.Role ?? "Nhân viên";
            string initials = UserSession.GetInitials(); // Ví dụ: "TT"

            // 2. JavaScript để cập nhật đúng các ID trong file dashboard.html của Thắng
            string jsCode = $@"
                (function() {{
                    const nameEl = document.getElementById('header_user_name');
                    const roleEl = document.getElementById('header_user_role');
                    const avatarEl = document.getElementById('header_user_avatar');

                    if(nameEl) nameEl.innerText = '{name}';
                    if(roleEl) roleEl.innerText = '{role}';
                    if(avatarEl) avatarEl.innerText = '{initials}';
                }})();";

            await webViewUser.CoreWebView2.ExecuteScriptAsync(jsCode);
        }
        private async void CoreWebView2_WebMessageReceived(object sender, CoreWebView2WebMessageReceivedEventArgs e)
        {
            // 1. Lấy dữ liệu dưới dạng JSON
            string jsonRaw = e.WebMessageAsJson;

            // 2. Parse thành đối tượng dynamic để truy cập nhanh các thuộc tính
            dynamic packet = JsonConvert.DeserializeObject(jsonRaw);
            if (packet == null) return;

            string action = packet.action;

            switch (action)
            {
                case "LOGOUT":
                    this.Invoke(new Action(() => {
                        this.Close();
                    }));
                    break;
                case "ADD_AND_SAVE_MST":
                    try
                    {
                        // 1. Kiểm tra dữ liệu đầu vào từ JS
                        if (packet.data == null || packet.data.mst == null)
                        {
                            await webViewUser.CoreWebView2.ExecuteScriptAsync("window.showAlert('Thông báo', 'Dữ liệu không hợp lệ!', 'warning')");
                            break;
                        }

                        // 2. Chuyển đổi dữ liệu từ dynamic sang Model chuẩn
                        // Giả sử Model của Thắng có các trường: Mst, Password, Company
                        var newCompany = new CompanyModel
                        {
                            Mst = packet.data.mst,
                            Password = packet.data.password,
                            CompanyName = packet.data.company
                        };

                        // 3. CẬP NHẬT RAM: Thêm vào List cache ngay lập tức để sử dụng tức thì
                        if (_cachedCompanies == null) _cachedCompanies = new List<CompanyModel>();

                        // Kiểm tra trùng MST để tránh nạp trùng vào RAM
                        var existing = _cachedCompanies.FirstOrDefault(c => c.Mst == newCompany.Mst);
                        if (existing != null)
                        {
                            _cachedCompanies.Remove(existing); // Nếu trùng thì xóa cũ thay mới
                        }
                        _cachedCompanies.Add(newCompany);

                        // 4. GHI FILE: Thực hiện ghi xuống ổ cứng để lưu trữ lâu dài
                        // Thắng nên dùng Formatting.Indented để file JSON dễ đọc
                        string jsonOutput = JsonConvert.SerializeObject(_cachedCompanies, Newtonsoft.Json.Formatting.Indented);
                        File.WriteAllText(filePath, jsonOutput);

                        // 5. PHẢN HỒI GIAO DIỆN: Gom lệnh để chạy cực nhanh
                        // Vừa hiện alert đẹp, vừa đóng modal thêm mới (nếu có), vừa render lại dropdown
                        string jsSuccess = $@"
            (function() {{
                window.showAlert('Thành công', 'Đã lưu công ty {newCompany.Mst} thành công!', 'success');
                if(window.renderCompanyDropdown) window.renderCompanyDropdown(); // Cập nhật lại list trên web
            }})();";

                        await webViewUser.CoreWebView2.ExecuteScriptAsync(jsSuccess);
                    }
                    catch (Exception ex)
                    {
                        System.Diagnostics.Debug.WriteLine("Lỗi ADD_AND_SAVE_MST: " + ex.Message);

                        string errorMsg = ex.Message.Replace("'", "\\'").Replace("\n", " ");
                        if (webViewUser?.CoreWebView2 != null)
                        {
                            await webViewUser.CoreWebView2.ExecuteScriptAsync($"window.showAlert('Lỗi hệ thống', '{errorMsg}', 'error')");
                        }
                        else
                        {
                            MessageBox.Show($"Lỗi nghiêm trọng: {ex.Message}", "Hệ thống");
                        }
                    }
                    break;
                case "DELETE_AND_SAVE_MST":
                    try
                    {
                        string mstToDelete = packet.mst;
                        if (string.IsNullOrEmpty(mstToDelete)) break;

                        // 1. TỐI ƯU: Xóa trực tiếp trên RAM (List đã cache)
                        // Việc xóa trên List này cực nhanh (gần như 0ms)
                        if (_cachedCompanies != null)
                        {
                            var itemToRemove = _cachedCompanies.FirstOrDefault(x => x.Mst == mstToDelete);
                            if (itemToRemove != null)
                            {
                                _cachedCompanies.Remove(itemToRemove);
                            }
                        }

                        // 2. GHI FILE: Cập nhật lại ổ cứng sau khi đã xóa trên RAM
                        string jsonOutput = JsonConvert.SerializeObject(_cachedCompanies, Newtonsoft.Json.Formatting.Indented);
                        File.WriteAllText(filePath, jsonOutput);

                        // 3. PHẢN HỒI GIAO DIỆN: Gom lệnh để Web cập nhật ngay lập tức
                        string jsDeleteSuccess = $@"
            (function() {{
                // 1. Thông báo
                window.showAlert('Thành công', 'Đã xóa MST {mstToDelete}!', 'success');
                
                // 2. TỐI ƯU: Xóa phần tử đó trên giao diện Web mà không cần load lại trang
                // Giả sử Thắng có hàm render lại list hoặc xóa element theo ID
                if(window.renderCompanyDropdown) window.renderCompanyDropdown();
                
                console.log('Hệ thống: Đã đồng bộ xóa giữa RAM, File và UI');
            }})();";

                        await webViewUser.CoreWebView2.ExecuteScriptAsync(jsDeleteSuccess);
                    }
                    catch (Exception ex)
                    {
                        System.Diagnostics.Debug.WriteLine("Lỗi DELETE_AND_SAVE_MST: " + ex.Message);

                        // Bắn lỗi ra giao diện Web nếu có sự cố ghi file
                        string safeError = ex.Message.Replace("'", "\\'");
                        await webViewUser.CoreWebView2.ExecuteScriptAsync($"window.showAlert('Lỗi', '{safeError}', 'error')");
                    }
                    break;
                case "GET_SETTINGS_DATA":
                    await SendJsonDataToWeb(filePath);
                    break;
                case "REFRESH_CAPTCHA":
                    await LoadCaptchaToUI();
                    break;
                case "REQUEST_OPEN_CAPTCHA":
                    System.Diagnostics.Debug.WriteLine(">>> Hệ thống: Đã nhận lệnh REQUEST_OPEN_CAPTCHA từ giao diện!");
                    await LoadCaptchaToUI();
                    break;
                case "SUBMIT_CAPTCHA":
                    try
                    {
                        string selectedMst = packet.data.taxCode;
                        string captcha = packet.data.captcha;

                        var currentCompany = _cachedCompanies?.FirstOrDefault(c => c.Mst == selectedMst);

                        if (currentCompany == null)
                        {
                            _ = webViewUser.CoreWebView2.ExecuteScriptAsync($"window.showAlert('Lỗi', 'Không tìm thấy MST {selectedMst} !', 'error')");
                            break;
                        }

                        // Gọi API
                        var (loginStatus, loginMessage) = await _taxService.LoginTax(selectedMst, currentCompany.Password, captcha);

                        if (loginStatus == "success")
                        {
                            // 1. Cập nhật UI ngay lập tức (Không đợi)
                            string jsUpdate = "(function() { if(window.closeCaptcha) window.closeCaptcha(); if(window.updateLoginStatus) window.updateLoginStatus(true); })();";
                            _ = webViewUser.CoreWebView2.ExecuteScriptAsync(jsUpdate);

                            // 2. Delay một chút xíu (100ms) để UI kịp vẽ xong chữ 'Đăng xuất' rồi mới hiện Box
                            await Task.Delay(100);
                            MessageBox.Show("Đăng nhập trang thuế thành công!", "Thành công", MessageBoxButtons.OK, MessageBoxIcon.Information);
                        }
                        else
                        {
                            string safeMsg = (loginMessage ?? "").Replace("'", "\\'").Replace("\n", " ");

                            // Luôn đảm bảo nút được mở khóa nếu thất bại
                            string jsReset = $"(function() {{ if(window.updateLoginStatus) window.updateLoginStatus(false); }})();";

                            if (loginStatus == "error_captcha")
                            {
                                _ = webViewUser.CoreWebView2.ExecuteScriptAsync(jsReset);
                                MessageBox.Show($"Mã Captcha không chính xác: {safeMsg}", "Sai Captcha", MessageBoxButtons.OK, MessageBoxIcon.Warning);

                                // Nạp captcha mới ngầm
                                _ = LoadCaptchaToUI();
                            }
                            else
                            {
                                _ = webViewUser.CoreWebView2.ExecuteScriptAsync(jsReset);
                                MessageBox.Show($"Lỗi: {safeMsg}", "Lỗi đăng nhập", MessageBoxButtons.OK, MessageBoxIcon.Error);
                            }
                        }
                    }
                    catch (Exception ex)
                    {
                        // Giải phóng nút khi có crash code
                        _ = webViewUser.CoreWebView2.ExecuteScriptAsync("if(window.updateLoginStatus) window.updateLoginStatus(false);");
                        System.Diagnostics.Debug.WriteLine("CRITICAL ERROR: " + ex.Message);
                        MessageBox.Show("Lỗi hệ thống: " + ex.Message, "Hệ thống", MessageBoxButtons.OK, MessageBoxIcon.Stop);
                    }
                    break;
                // Trong phần xử lý Message từ WebView2
                case "REQUEST_LOGOUT":

                    var (status, message) = await _taxService.LogoutTaxAsync();

                    if (status == "success")
                    {
                        // 1. Thông báo thành công cho giao diện Web
                        await webViewUser.CoreWebView2.ExecuteScriptAsync(
                            $"window.showAlert('Thành công', '{message}', 'success')"
                        );

                        // 2. Tự động reload lại trang để xóa sạch dữ liệu cũ trên giao diện
                        await webViewUser.CoreWebView2.ExecuteScriptAsync("window.location.reload()");
                    }
                    else
                    {
                        // Thông báo lỗi nếu Logout thất bại
                        await webViewUser.CoreWebView2.ExecuteScriptAsync(
                            $"window.showAlert('Lỗi Logout', '{message}', 'error')"
                        );
                    }
                    break;
                case "SEARCH_INVOICE":
                    List<dynamic> invoices = null;
                    string from = packet.data.fromDate;
                    string to = packet.data.toDate;

                    try
                    {
                        var result = await _taxService.SearchInvoices(from, to);

                         status = result.Status;
                         message = result.Message;
                        invoices = result.Invoices; 

                        if (status == "success")
                        {
                            if (invoices != null && invoices.Count > 0)
                            {
                                MessageBox.Show($"Tìm thấy {invoices.Count} hóa đơn.", "Thành công");
                            }
                            else
                            {
                                MessageBox.Show("Không có hóa đơn nào.", "Thông báo");
                            }
                        }
                        else
                        {
                            MessageBox.Show(message, "Lỗi");
                        }
                    }
                    catch (Exception ex)
                    {
                        MessageBox.Show("Lỗi thực thi: " + ex.Message);
                    }
                    break;
            }
        }
        private async Task SendJsonDataToWeb(string filePath)
        {
            try
            {
                if (File.Exists(filePath))
                {
                    string jsonData = File.ReadAllText(filePath);
                    string safeJson = JsonConvert.ToString(jsonData);
                    await webViewUser.CoreWebView2.ExecuteScriptAsync($"window.loadInitialData({safeJson})");
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("Lỗi bắn dữ liệu: " + ex.Message);
            }
        }
        private void UserForm_FormClosed(object sender, FormClosedEventArgs e)
        {
            // Lệnh này sẽ quét sạch tất cả các Form đang ẩn và đóng toàn bộ App
            Application.Exit();
        }

        private async Task LoadCaptchaToUI()
        {
            string captchaData = await _taxService.GetCaptchaBase64();

            if (!string.IsNullOrEmpty(captchaData))
            {
                if (!captchaData.StartsWith("data:image"))
                    captchaData = "data:image/png;base64," + captchaData;

                // Lưu ý: Tất cả dấu { và } của JavaScript phải viết thành {{ và }}
                string jsCommand = $@"
        (function() {{
            if(window.openCaptcha) {{
                window.openCaptcha('{captchaData}');
                
                const input = document.getElementById('input-captcha');
                if(input) {{
                    input.value = ''; 
                    setTimeout(function() {{ 
                        input.focus(); 
                    }}, 150);
                }}
            }}
        }})();";

                _ = webViewUser.CoreWebView2.ExecuteScriptAsync(jsCommand);
            }
        }
        private void LoadCompaniesToRAM()
        {
            try
            {
                if (File.Exists(filePath))
                {
                    string jsonContent = File.ReadAllText(filePath);
                    _cachedCompanies = JsonConvert.DeserializeObject<List<CompanyModel>>(jsonContent) ?? new List<CompanyModel>();
                    System.Diagnostics.Debug.WriteLine($">>> RAM: Đã nạp {_cachedCompanies.Count} công ty.");
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("Lỗi nạp cache: " + ex.Message);
            }
        }
    }
}
