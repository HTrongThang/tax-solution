using Frontend.Models;
using Frontend.Services;
using Microsoft.Web.WebView2.Core;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Data;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Xml;
using static System.Runtime.InteropServices.JavaScript.JSType;
using Formatting = Newtonsoft.Json.Formatting;
namespace Frontend.Views
{
    public partial class UserForm : Form
    {
        private bool _isCrawling = false;
        private readonly TaxService _taxService = new TaxService();
        private CancellationTokenSource _searchCts;
        // Current crawl request id received from frontend (if any)
        private string _currentCrawlRequestId;
        // Thêm dòng này để lưu dữ liệu công ty vào RAM
        private List<CompanyModel> _cachedCompanies = new List<CompanyModel>();
        private readonly string filePath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "tax_accounts.json");
        // Lock để đồng bộ truy cập file
        private readonly object _fileLock = new object();
        // Semaphore để tuần tự hóa các lệnh ExecuteScriptAsync tới WebView2
        private readonly SemaphoreSlim _webviewSemaphore = new SemaphoreSlim(1, 1);
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

            webViewUser.NavigationCompleted += async (s, args) =>
            {
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
                    this.Invoke(new Action(() =>
                    {
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

                        // 4. GHI FILE: Thực hiện ghi xuống ổ cứng để lưu trữ lâu dài (thread-safe)
                        WriteCompaniesToFile(_cachedCompanies);

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
                        // 2. GHI FILE: Cập nhật lại ổ cứng sau khi đã xóa trên RAM
                        WriteCompaniesToFile(_cachedCompanies);

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
                    try
                    {
                        string fromDate = packet.data.fromDate;
                        string toDate = packet.data.toDate;

                        if (string.IsNullOrEmpty(fromDate) || string.IsNullOrEmpty(toDate))
                        {
                            await webViewUser.CoreWebView2.ExecuteScriptAsync("window.showAlert('Lỗi', 'Dữ liệu ngày tháng không hợp lệ', 'error')");
                            break;
                        }

                        // Cancel any previous search
                        _searchCts?.Cancel();
                        _searchCts = new CancellationTokenSource();

                        // Start new search with cancellation support
                        _ = HandleInvoiceSearchStream(fromDate, toDate, _searchCts.Token);
                    }
                    catch (Exception ex)
                    {
                        System.Diagnostics.Debug.WriteLine("Lỗi SEARCH_INVOICE Case: " + ex.Message);
                    }
                    break;
                case "CANCEL_SEARCH":
                    try
                    {
                        _searchCts?.Cancel();
                    }
                    catch { }
                    break;
                case "REQUEST_PAGE":
                    try
                    {
                        int page = packet.data.page;
                        int pageSize = packet.data.pageSize;
                        // Call backend API to get specific page (implement if supported by backend)
                        // For now we forward to Go service via TaxService if implemented
                        _ = Task.Run(async () =>
                        {
                            try
                            {
                                var pageResult = await _taxService.GetInvoicesPageAsync(page, pageSize);
                                if (pageResult != null)
                                {
                                    string js = $"renderInvoices({JsonConvert.SerializeObject(pageResult)});";
                                    await ExecuteScriptSerializedAsync(js);
                                }
                            }
                            catch (Exception ex)
                            {
                                System.Diagnostics.Debug.WriteLine("REQUEST_PAGE error: " + ex.Message);
                            }
                        });
                    }
                    catch { }
                    break;
                case "REQUEST_SELECT_OUTPUT_DIR":
                    try
                    {
                        // Phải chạy trên UI Thread vì FolderBrowserDialog là thành phần giao diện
                        this.Invoke(new Action(async () =>
                        {
                            using (FolderBrowserDialog fbd = new FolderBrowserDialog())
                            {
                                fbd.Description = "Chọn thư mục để lưu tệp XML và hóa đơn cào được";

                                if (fbd.ShowDialog() == DialogResult.OK)
                                {
                                    // Gửi đường dẫn đã chọn về cho Frontend kèm theo requestId để khớp Promise
                                    var response = new
                                    {
                                        action = "OUTPUT_DIR_SELECTED",
                                        output_dir = fbd.SelectedPath,
                                        requestId = packet.requestId // Quan trọng: Trả lại đúng ID này
                                    };
                                    await PostWebMessageAsJson(response);
                                }
                                else
                                {
                                    // Nếu người dùng bấm Cancel, cũng phải báo về để FE tắt trạng thái Loading
                                    var cancelResponse = new
                                    {
                                        action = "CRAWL_ERROR",
                                        message = "Người dùng đã hủy chọn thư mục.",
                                        requestId = packet.requestId
                                    };
                                    await PostWebMessageAsJson(cancelResponse);
                                }
                            }
                        }));
                    }
                    catch (Exception ex)
                    {
                        Debug.WriteLine("Lỗi REQUEST_SELECT_OUTPUT_DIR: " + ex.Message);
                    }
                    break;
                case "START_CRAWL":
                    string outputDir = packet?.output_dir?.ToString();
                    string requestId = packet?.requestId != null ? packet.requestId.ToString() : null;

                    if (string.IsNullOrWhiteSpace(outputDir))
                    {
                        var errorPayload = new { action = "CRAWL_ERROR", message = "Vui lòng chọn thư mục lưu trữ trước khi bắt đầu!", requestId };
                        await PostWebMessageAsJson(errorPayload);
                        break;
                    }

                    // Store requestId for later status messages and send an ACK back to FE immediately
                    _currentCrawlRequestId = requestId;
                    // Send ACK so frontend promise resolves and can update UI
                    var ackPayload = new { action = "START_CRAWL_ACK", requestId = requestId, status = "accepted", message = "Yêu cầu crawl đã được nhận" };
                    await PostWebMessageAsJson(ackPayload);

                    await HandleStartCrawl(outputDir, requestId);
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
                    await ExecuteScriptSerializedAsync($"window.loadInitialData({safeJson})");
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

                // Use serialized execution to avoid concurrent calls
                _ = ExecuteScriptSerializedAsync(jsCommand);
            }
        }

        // Helper: Ensure ExecuteScriptAsync calls are serialized to avoid concurrent execution issues
        private async Task ExecuteScriptSerializedAsync(string script)
        {
            try
            {
                await _webviewSemaphore.WaitAsync();
                if (webViewUser?.CoreWebView2 != null)
                {
                    await webViewUser.CoreWebView2.ExecuteScriptAsync(script);
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("ExecuteScriptSerializedAsync error: " + ex.Message);
            }
            finally
            {
                _webviewSemaphore.Release();
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
        private async Task HandleInvoiceSearchStream(string from, string to, CancellationToken cancellationToken = default)
        {
            try
            {
                await _taxService.SearchInvoicesSSE(from, to, (progressData) =>
                {
                    // Marshal to UI thread and run async work safely
                    this.BeginInvoke((Action)(() =>
                    {
                        _ = Task.Run(async () =>
                        {
                            try
                            {
                                // Gửi progress qua PostWebMessageAsJson (ít overhead hơn ExecuteScript)
                                var wrapper = new { action = "sse_progress", data = progressData };
                                var wrapperJson = JsonConvert.SerializeObject(wrapper);
                                this.BeginInvoke((Action)(() =>
                                {
                                    try { webViewUser.CoreWebView2.PostWebMessageAsJson(wrapperJson); }
                                    catch (Exception ex) { System.Diagnostics.Debug.WriteLine("PostWebMessage error: " + ex.Message); }
                                }));

                                // Fallback: nếu chunk cuối có hóa đơn, render ngay qua ExecuteScript
                                if (progressData.IsDone && progressData.Invoices != null && progressData.Invoices.Count > 0)
                                {
                                    string jsonData = JsonConvert.SerializeObject(progressData.Invoices, Formatting.None);
                                    await ExecuteScriptSerializedAsync($"renderInvoices({jsonData})");
                                }
                            }
                            catch (Exception ex)
                            {
                                System.Diagnostics.Debug.WriteLine("SSE handler error: " + ex.Message);
                            }
                        });
                    }));
                }, cancellationToken);
            }
            catch (Exception ex)
            {
                // Escape message để tránh XSS/syntax error khi nhúng vào JS string
                var safeMsg = JsonConvert.ToString(ex.Message);
                await webViewUser.CoreWebView2.ExecuteScriptAsync($"window.showAlert('Lỗi hệ thống', {safeMsg}, 'error')");
            }
        }


        private async Task HandleStartCrawl(string path, string requestId = null)
        {
            if (_isCrawling) return;
            // Pass requestId to backend if TaxService supports it in future. For now, store locally.
            var (status, message) = await _taxService.StartCrawlAsync(path);

            if (status == "success")
            {
                _isCrawling = true;
                _ = RunStatusPolling();
            }
            else
            {
                MessageBox.Show("Không thể bắt đầu: " + message);
                var payload = new { action = "CRAWL_ERROR", message = message, requestId };
                await PostWebMessageAsJson(payload);
            }
        }
        private async Task RunStatusPolling()
        {
            try
            {
                while (_isCrawling)
                {
                    var statusInfo = await _taxService.GetStatus();
                    if (statusInfo != null)
                    {
                        await PostWebMessageAsJson(new
                        {
                            action = "CRAWL_STATUS_UPDATE",
                            status = statusInfo.current_status,
                            is_running = statusInfo.is_running,
                            total_invoices = statusInfo.total_invoices,
                            requestId = _currentCrawlRequestId
                        });

                        if (!statusInfo.is_running) _isCrawling = false;
                    }
                    if (_isCrawling) await Task.Delay(1000);
                }
            }
            catch (Exception ex) { Debug.WriteLine($"Polling Error: {ex.Message}"); }
            finally
            {
                _isCrawling = false;
                await PostWebMessageAsJson(new { action = "CRAWL_FINISHED", requestId = _currentCrawlRequestId });
                // clear stored request id after finishing
                _currentCrawlRequestId = null;
            }
        }
        private async Task PostWebMessageAsJson(object data)
        {
            if (webViewUser?.CoreWebView2 != null)
            {
                string json = JsonConvert.SerializeObject(data);
                // Marshal về UI Thread nếu cần
                if (this.InvokeRequired)
                {
                    this.Invoke(new Action(() => webViewUser.CoreWebView2.PostWebMessageAsJson(json)));
                }
                else
                {
                    webViewUser.CoreWebView2.PostWebMessageAsJson(json);
                }
            }
            await Task.CompletedTask;
        }

        // Helper: Thread-safe file write for tax_accounts.json
        private void WriteCompaniesToFile(List<CompanyModel> companies)
        {
            try
            {
                lock (_fileLock)
                {
                    string jsonOutput = JsonConvert.SerializeObject(companies, Formatting.Indented);
                    File.WriteAllText(filePath, jsonOutput);
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("WriteCompaniesToFile error: " + ex.Message);
            }
        }

    }
}
