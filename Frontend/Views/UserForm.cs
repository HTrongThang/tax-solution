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
        private string _currentCrawlOutputDir;
        private bool _crawlFolderIsDedicated; // true khi backend trả về thư mục riêng của lần cào này (an toàn để xóa XML sau khi bóc tách)
        private string _currentCrawlMst;
        private string _currentCrawlInvoiceType;
        // Thêm dòng này để lưu dữ liệu công ty vào RAM
        private List<CompanyModel> _cachedCompanies = new List<CompanyModel>();
        // Lưu trữ danh sách hóa đơn hiện tại trong RAM C# (chỉ giữ 1 công ty đang làm việc)
        private List<dynamic> _currentInvoicesRAM = new List<dynamic>();
        // Quản lý cơ sở dữ liệu SQLite cục bộ
        private readonly DatabaseService _databaseService = new DatabaseService();
        // Semaphore để tuần tự hóa các lệnh ExecuteScriptAsync tới WebView2
        private readonly SemaphoreSlim _webviewSemaphore = new SemaphoreSlim(1, 1);
        private bool _isForceClosing = false;
        public UserForm()
        {
            InitializeComponent();
            this.Text = $"TaxSolution - Quản lý Hóa đơn & Khai thác Thuế ({AppVersionHelper.DisplayVersion})";
            this.Load += UserForm_Load;
            this.FormClosing += UserForm_FormClosing;
            this.FormClosed += UserForm_FormClosed;
        }
        private async void UserForm_Load(object sender, EventArgs e)
        {
            this.Opacity = 0;

            // Khởi tạo SQLite DB và nạp dữ liệu công ty vào RAM (async để không block UI)
            await _databaseService.InitializeAsync();
            await LoadCompaniesToRAM();

            // 1. Căn chỉnh kích thước: Full màn hình
            this.WindowState = FormWindowState.Maximized;

            // 2. Khởi tạo WebView2 (lưu cache vào AppData để tránh lỗi quyền)
            var userDataFolder = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "TaxSolution", "WebView2");
            var env = await CoreWebView2Environment.CreateAsync(null, userDataFolder);
            await webViewUser.EnsureCoreWebView2Async(env);
            await webViewUser.CoreWebView2.AddScriptToExecuteOnDocumentCreatedAsync(
                $"window.appVersion = '{AppVersionHelper.DisplayVersion}'; localStorage.setItem('app_version', '{AppVersionHelper.DisplayVersion}');"
            );

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

                    // Đồng bộ số phiên bản vào giao diện HTML (sidebar, footer,...)
                    try
                    {
                        await webViewUser.CoreWebView2.ExecuteScriptAsync(
                            $"document.querySelectorAll('.app-version-text').forEach(el => el.innerText = '{AppVersionHelper.DisplayVersion}');"
                        );
                    }
                    catch { }

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
                            $"window.showAlert('Lỗi kết nối', 'Không thể kết nối tới Service. Vui lòng kiểm tra Service!', 'error')"
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

            // 2. Escape giá trị để tránh XSS/syntax error (JsonConvert.ToString tự thêm ngoặc kép + escape)
            string safeName = JsonConvert.ToString(name);
            string safeRole = JsonConvert.ToString(role);
            string safeInitials = JsonConvert.ToString(initials);

            // 3. JavaScript để cập nhật đúng các ID trong file dashboard.html
            string jsCode = $@"
                (function() {{
                    const nameEl = document.getElementById('header_user_name');
                    const roleEl = document.getElementById('header_user_role');
                    const avatarEl = document.getElementById('header_user_avatar');

                    if(nameEl) nameEl.innerText = {safeName};
                    if(roleEl) roleEl.innerText = {safeRole};
                    if(avatarEl) avatarEl.innerText = {safeInitials};

                    localStorage.setItem('app_version', '{AppVersionHelper.DisplayVersion}');
                    document.querySelectorAll('.app-version-text').forEach(el => el.innerText = '{AppVersionHelper.DisplayVersion}');
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
                case "FORCE_EXIT":
                    _isForceClosing = true;
                    this.Invoke(new Action(() =>
                    {
                        this.Close();
                    }));
                    break;
                case "CHECK_APP_UPDATE":
                    _ = Task.Run(async () =>
                    {
                        var updateResult = await UpdateService.CheckForUpdateAsync();
                        this.Invoke(new Action(() =>
                        {
                            if (updateResult.Success && updateResult.HasUpdate)
                            {
                                var dlg = new UpdateDialogForm(updateResult);
                                dlg.ShowDialog(this);
                            }
                            else if (updateResult.Success && !updateResult.HasUpdate)
                            {
                                MessageBox.Show(
                                    $"Bạn đang sử dụng phiên bản mới nhất ({AppVersionHelper.DisplayVersion}).\nKhông có bản cập nhật mới nào.",
                                    "Kiểm tra cập nhật",
                                    MessageBoxButtons.OK,
                                    MessageBoxIcon.Information
                                );
                            }
                            else
                            {
                                MessageBox.Show(
                                    $"Không thể kết nối đến máy chủ kiểm tra cập nhật.\nChi tiết: {updateResult.ErrorMessage}",
                                    "Lỗi kiểm tra cập nhật",
                                    MessageBoxButtons.OK,
                                    MessageBoxIcon.Warning
                                );
                            }
                        }));
                    });
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
                            Password = packet.data.pass,
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

                        // 4. LƯU SQLITE: Thực hiện ghi xuống database SQLite (đã mã hóa DPAPI an toàn)
                        await _databaseService.UpsertAccountAsync(newCompany);

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

                        // 2. XÓA TRONG SQLITE: Cập nhật lại database sau khi đã xóa trên RAM
                        await _databaseService.DeleteAccountAsync(mstToDelete);

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
                    await SendSettingsDataToWeb();
                    break;
                case "REFRESH_CAPTCHA":
                    await LoadCaptchaToUI();
                    break;
                case "REQUEST_OPEN_CAPTCHA":
                    System.Diagnostics.Debug.WriteLine(">>> Hệ thống: Đã nhận lệnh REQUEST_OPEN_CAPTCHA từ giao diện!");

                    // Mở dialog đăng nhập trên tab hiện tại (không mở tab mới)
                    var (loginDialogStatus, loginDialogMsg) = await _taxService.ShowLoginDialogAsync();
                    if (loginDialogStatus != "success")
                    {
                        await webViewUser.CoreWebView2.ExecuteScriptAsync(
                            $"window.showAlert('Lỗi', 'Không thể mở form đăng nhập: {loginDialogMsg.Replace("'", "\\'")}', 'error')"
                        );
                        break;
                    }

                    // Chờ dialog và captcha image trên trang thuế render xong
                    await Task.Delay(1500);
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
                            // Cập nhật UI ngay lập tức và hiển thị thông báo thành công đẹp mắt
                            string safeMst = JsonConvert.ToString(selectedMst);
                            string safeSuccessMsg = JsonConvert.ToString("Đăng nhập trang thuế thành công!");
                            string jsUpdate = $"(function() {{ if(window.closeCaptcha) window.closeCaptcha(); if(window.updateLoginStatus) window.updateLoginStatus(true, {safeMst}); if(window.showAlert) window.showAlert('Thành công', {safeSuccessMsg}, 'success'); }})();";
                            _ = webViewUser.CoreWebView2.ExecuteScriptAsync(jsUpdate);
                        }
                        else
                        {
                            string rawMsg = loginMessage ?? "";
                            string lowerMsg = rawMsg.ToLower();

                            // Phân biệt chính xác giữa lỗi tài khoản/mật khẩu và lỗi captcha
                            bool isCredentialsError = loginStatus == "error_credentials"
                                || rawMsg.Contains("SAI_THONG_TIN")
                                || lowerMsg.Contains("tên đăng nhập")
                                || lowerMsg.Contains("mật khẩu")
                                || lowerMsg.Contains("tài khoản");

                            bool isCaptchaError = !isCredentialsError && (
                                loginStatus == "error_captcha"
                                || rawMsg.Contains("SAI_CAPTCHA")
                                || lowerMsg.Contains("captcha")
                                || lowerMsg.Contains("mã xác nhận")
                                || lowerMsg.Contains("mã xác thực")
                                || lowerMsg.Contains("mã bảo vệ")
                                || lowerMsg.Contains("mã kiểm tra"));

                            if (isCredentialsError)
                            {
                                // Sai tài khoản / mật khẩu: Đóng popup captcha vì nhập lại captcha cũng vô ích
                                string displayMsg = rawMsg
                                    .Replace("SAI_THONG_TIN:", "")
                                    .Replace("LOI_DANG_NHAP:", "")
                                    .Replace("SAI_CAPTCHA:", "")
                                    .Trim();
                                if (string.IsNullOrEmpty(displayMsg))
                                    displayMsg = "Tên đăng nhập hoặc mật khẩu không đúng.";
                                
                                if (!displayMsg.ToLower().Contains("kiểm tra lại"))
                                    displayMsg += " Vui lòng kiểm tra lại mật khẩu doanh nghiệp trong phần mềm!";

                                string safeMsg = JsonConvert.ToString(displayMsg);
                                string jsErr = $"(function() {{ if(window.closeCaptcha) window.closeCaptcha(); if(window.updateLoginStatus) window.updateLoginStatus(false); if(window.showAlert) window.showAlert('Sai thông tin đăng nhập', {safeMsg}, 'error'); }})();";
                                _ = webViewUser.CoreWebView2.ExecuteScriptAsync(jsErr);
                            }
                            else if (isCaptchaError)
                            {
                                // Sai Captcha: Giữ mở popup, làm trống ô nhập và nạp lại Captcha mới
                                string displayMsg = rawMsg
                                    .Replace("SAI_CAPTCHA:", "")
                                    .Replace("LOI_DANG_NHAP:", "")
                                    .Trim();
                                if (string.IsNullOrEmpty(displayMsg)) displayMsg = "Mã Captcha không chính xác, vui lòng nhập lại!";
                                string safeCaptchaMsg = JsonConvert.ToString(displayMsg);

                                string jsReset = $"(function() {{ const inp = document.getElementById('input-captcha'); if(inp) {{ inp.disabled = false; inp.value = ''; inp.focus(); }} if(window.showAlert) window.showAlert('Sai Captcha', {safeCaptchaMsg}, 'warning'); }})();";
                                _ = webViewUser.CoreWebView2.ExecuteScriptAsync(jsReset);

                                await Task.Delay(400);
                                await LoadCaptchaToUI();
                            }
                            else
                            {
                                // Lỗi khác (Hệ thống, mất kết nối, quá thời gian chờ...)
                                string displayMsg = rawMsg
                                    .Replace("LOI_DANG_NHAP:", "")
                                    .Replace("SAI_CAPTCHA:", "")
                                    .Trim();
                                if (string.IsNullOrEmpty(displayMsg)) displayMsg = "Đăng nhập thất bại. Vui lòng thử lại!";
                                string safeErrMsg = JsonConvert.ToString(displayMsg);

                                string jsErr = $"(function() {{ if(window.closeCaptcha) window.closeCaptcha(); if(window.updateLoginStatus) window.updateLoginStatus(false); if(window.showAlert) window.showAlert('Lỗi đăng nhập', {safeErrMsg}, 'error'); }})();";
                                _ = webViewUser.CoreWebView2.ExecuteScriptAsync(jsErr);
                            }
                        }
                    }
                    catch (Exception ex)
                    {
                        // Giải phóng nút khi có crash code
                        _ = webViewUser.CoreWebView2.ExecuteScriptAsync("if(window.updateLoginStatus) window.updateLoginStatus(false);");
                        System.Diagnostics.Debug.WriteLine("CRITICAL ERROR: " + ex.Message);
                        string safeCritMsg = JsonConvert.ToString("Lỗi hệ thống: " + ex.Message);
                        _ = webViewUser.CoreWebView2.ExecuteScriptAsync($"if(window.showAlert) window.showAlert('Lỗi hệ thống', {safeCritMsg}, 'error');");
                    }
                    break;
                case "REQUEST_LOGOUT":
                    InvoiceMemoryService.Instance.Clear("Đăng xuất tài khoản");
                    var (status, message) = await _taxService.LogoutTaxAsync();

                    if (status == "success")
                    {
                        // Cập nhật UI: nút "Đăng xuất" → "Đăng nhập"
                        await webViewUser.CoreWebView2.ExecuteScriptAsync(
                            "if(window.updateLoginStatus) window.updateLoginStatus(false);"
                        );

                        string safeMsg = JsonConvert.ToString(message);
                        await webViewUser.CoreWebView2.ExecuteScriptAsync(
                            $"window.showAlert('Thành công', {safeMsg}, 'success')"
                        );
                    }
                    else
                    {
                        string safeLogoutMsg = JsonConvert.ToString(message);
                        await webViewUser.CoreWebView2.ExecuteScriptAsync(
                            $"window.showAlert('Lỗi Logout', {safeLogoutMsg}, 'error')"
                        );
                    }
                    break;
                case "SEARCH_INVOICE":
                    try
                    {
                        string fromDate = packet.data.fromDate;
                        string toDate = packet.data.toDate;
                        string invoiceType = packet.data.invoiceType != null ? (string)packet.data.invoiceType : "ban_ra";
                        string invoiceCategory = packet.data.invoiceCategory != null ? (string)packet.data.invoiceCategory : "dien_tu";
                        string checkResult = packet.data.checkResult != null ? (string)packet.data.checkResult : "";

                        if (string.IsNullOrEmpty(fromDate) || string.IsNullOrEmpty(toDate))
                        {
                            await webViewUser.CoreWebView2.ExecuteScriptAsync("window.showAlert('Lỗi', 'Dữ liệu ngày tháng không hợp lệ', 'error')");
                            break;
                        }

                        // Cancel any previous search
                        _searchCts?.Cancel();
                        _searchCts = new CancellationTokenSource();

                        // Start new search with cancellation support
                        _ = HandleInvoiceSearchStream(fromDate, toDate, invoiceType, invoiceCategory, checkResult, _searchCts.Token);
                    }
                    catch (Exception ex)
                    {
                        System.Diagnostics.Debug.WriteLine("Lỗi SEARCH_INVOICE Case: " + ex.Message);
                    }
                    break;
                case "CANCEL_SEARCH":
                case "CANCEL_CRAWL":
                    try
                    {
                        _searchCts?.Cancel();
                        _isCrawling = false;
                        _ = _taxService.StopTaxAsync();
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
                            // Tự động lưu vào thư mục Downloads của máy tính, không mở hộp thoại
                            string defaultDownloadsDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), "Downloads");
                            if (!Directory.Exists(defaultDownloadsDir))
                            {
                                defaultDownloadsDir = Environment.GetFolderPath(Environment.SpecialFolder.MyDocuments);
                            }
                            var response = new
                            {
                                action = "OUTPUT_DIR_SELECTED",
                                output_dir = defaultDownloadsDir,
                                requestId = packet.requestId
                            };
                            await PostWebMessageAsJson(response);
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
                    string crawlMst = packet?.mst != null ? packet.mst.ToString() : "";
                    string crawlInvoiceType = packet?.invoice_type != null ? packet.invoice_type.ToString() : "ban_ra";

                    if (string.IsNullOrWhiteSpace(outputDir) || outputDir == "DEFAULT_DOWNLOADS")
                    {
                        outputDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), "Downloads");
                        if (!Directory.Exists(outputDir))
                        {
                            outputDir = Environment.GetFolderPath(Environment.SpecialFolder.MyDocuments);
                        }
                    }

                    // Store requestId for later status messages and send an ACK back to FE immediately
                    _currentCrawlRequestId = requestId;
                    // Send ACK so frontend promise resolves and can update UI
                    var ackPayload = new { action = "START_CRAWL_ACK", requestId = requestId, status = "accepted", message = "Yêu cầu crawl đã được nhận" };
                    await PostWebMessageAsJson(ackPayload);

                    await HandleStartCrawl(outputDir, requestId, crawlMst, crawlInvoiceType);
                    break;

                case "STOP_CRAWL":
                case "STOP_DOWNLOAD":
                case "STOP_SEARCH":
                    try
                    {
                        await _taxService.StopTaxAsync();
                        _isCrawling = false;
                        await PostWebMessageAsJson(new
                        {
                            action = "CRAWL_FINISHED",
                            status = "stopped",
                            is_stopped = true,
                            message = "Tiến trình đã được dừng.",
                            requestId = _currentCrawlRequestId
                        });
                        _currentCrawlRequestId = null;
                    }
                    catch (Exception ex)
                    {
                        Debug.WriteLine("Lỗi STOP_CRAWL: " + ex.Message);
                    }
                    break;

                case "EXPORT_EXCEL":
                    try
                    {
                        string excelRequestId = packet?.requestId != null ? packet.requestId.ToString() : null;
                        string reportType = packet?.data?.reportType != null ? packet.data.reportType.ToString() : "bao_cao_tong_hop";
                        string invoicesJsonRaw = packet?.data?.invoices != null ? packet.data.invoices.ToString() : "[]";
                        bool excelIsMuaVao = packet?.data?.isMuaVao != null && (bool)packet.data.isMuaVao;
                        string excelFromDate = packet?.data?.fromDate != null ? packet.data.fromDate.ToString() : "";
                        string excelToDate = packet?.data?.toDate != null ? packet.data.toDate.ToString() : "";
                        string excelMst = packet?.data?.mst != null ? packet.data.mst.ToString() : "MST";

                        // Mở SaveFileDialog trên UI Thread
                        this.Invoke(new Action(async () =>
                        {
                            try
                            {
                                string dirLabel = excelIsMuaVao ? "MuaVao" : "BanRa";
                                bool isDetailReport = reportType == "bang_ke_chi_tiet";
                                string fromClean = excelFromDate?.Replace("/", "").Replace("-", "") ?? "";
                                string toClean = excelToDate?.Replace("/", "").Replace("-", "") ?? "";
                                string dateSuffix = !string.IsNullOrEmpty(fromClean) && !string.IsNullOrEmpty(toClean) ? $"_{fromClean}_{toClean}" : "";
                                string randSuffix = $"_{DateTime.Now:HHmmss}_{Random.Shared.Next(100, 999)}";
                                string defaultFileName = isDetailReport
                                    ? $"BangKeChiTiet_{dirLabel}_{excelMst}{dateSuffix}{randSuffix}.xlsx"
                                    : $"BaoCaoTongHop_{dirLabel}_{excelMst}{dateSuffix}{randSuffix}.xlsx";
                                string dialogTitle = isDetailReport
                                    ? "Chá»n nÆ¡i lÆ°u file Excel báº£ng kÃª chi tiáº¿t hÃ ng hÃ³a"
                                    : "Chá»n nÆ¡i lÆ°u file Excel bÃ¡o cÃ¡o tá»•ng há»£p";

                                // Lưu mặc định thẳng vào Downloads, không cần mở hộp thoại SaveFileDialog
                                string defaultDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), "Downloads");
                                if (!Directory.Exists(defaultDir))
                                {
                                    defaultDir = Environment.GetFolderPath(Environment.SpecialFolder.MyDocuments);
                                }
                                string selectedPath = Path.Combine(defaultDir, defaultFileName);
                                string selectedDir = Path.GetDirectoryName(selectedPath) ?? "";

                                // ThÃ´ng bÃ¡o Ä‘ang xá»­ lÃ½
                                var processingPayload = new
                                {
                                    action = "EXPORT_EXCEL_PROCESSING",
                                    message = isDetailReport ? "Äang táº¡o file Excel báº£ng kÃª chi tiáº¿t..." : "Äang táº¡o file Excel...",
                                    requestId = excelRequestId
                                };
                                await PostWebMessageAsJson(processingPayload);

                                string savedFilePath = "";
                                // Cháº¡y xuáº¥t Excel trÃªn background thread Ä‘á»ƒ khÃ´ng block UI
                                await Task.Run(() =>
                                {
                                    var exportService = new ExportExcelService();
                                    if (isDetailReport)
                                    {
                                        savedFilePath = exportService.ExportBangKeChiTiet(
                                            invoicesJsonRaw,
                                            selectedPath,
                                            excelIsMuaVao,
                                            excelFromDate,
                                            excelToDate,
                                            excelMst
                                        );
                                    }
                                    else
                                    {
                                        savedFilePath = exportService.ExportBaoCaoTongHop(
                                            invoicesJsonRaw,
                                            selectedPath,
                                            excelIsMuaVao,
                                            excelFromDate,
                                            excelToDate,
                                            excelMst
                                        );
                                    }
                                });

                                string finalPath = !string.IsNullOrEmpty(savedFilePath) && File.Exists(savedFilePath) ? savedFilePath : selectedPath;

                                // ThÃ nh cÃ´ng
                                var successPayload = new
                                {
                                    action = "EXPORT_EXCEL_DONE",
                                    message = isDetailReport ? "Xuáº¥t Báº£ng kÃª chi tiáº¿t hÃ ng hÃ³a thÃ nh cÃ´ng!" : "Xuáº¥t Excel thÃ nh cÃ´ng!",
                                    filePath = finalPath,
                                    requestId = excelRequestId
                                };
                                await PostWebMessageAsJson(successPayload);

                                // Tá»± má»Ÿ file Excel náº¿u ngÆ°á»i dÃ¹ng muá»‘n
                                try { Process.Start(new ProcessStartInfo(finalPath) { UseShellExecute = true }); }
                                catch { /* Bá» qua náº¿u khÃ´ng má»Ÿ Ä‘Æ°á»£c */ }
                            }
                            catch (Exception exInner)
                            {
                                Debug.WriteLine("Lỗi EXPORT_EXCEL (inner): " + exInner.Message);
                                var errorPayload = new
                                {
                                    action = "EXPORT_EXCEL_ERROR",
                                    message = "Lỗi xuất Excel: " + exInner.Message,
                                    requestId = excelRequestId
                                };
                                await PostWebMessageAsJson(errorPayload);
                            }
                        }));
                    }
                    catch (Exception exOuter)
                    {
                        Debug.WriteLine("Lỗi EXPORT_EXCEL (outer): " + exOuter.Message);
                    }
                    break;

                case "PARSE_XML_DIRECTORY":
                    try
                    {
                        string xmlRequestId = packet?.requestId != null ? packet.requestId.ToString() : null;
                        string directoryPath = packet?.data?.directoryPath != null ? packet.data.directoryPath.ToString() : null;

                        var parseOptions = new XmlParseOptions
                        {
                            TargetMst = packet?.data?.targetMst != null ? packet.data.targetMst.ToString() : (packet?.data?.mst != null ? packet.data.mst.ToString() : null),
                            IsMuaVao = packet?.data?.isMuaVao != null ? (bool)packet.data.isMuaVao : null,
                            FromDateStr = packet?.data?.fromDate != null ? packet.data.fromDate.ToString() : null,
                            ToDateStr = packet?.data?.toDate != null ? packet.data.toDate.ToString() : null
                        };

                        // Nếu không truyền directoryPath, mở FolderBrowserDialog để người dùng chọn
                        if (string.IsNullOrWhiteSpace(directoryPath))
                        {
                            this.Invoke(new Action(async () =>
                            {
                                using var fbd = new FolderBrowserDialog
                                {
                                    Description = "Chọn thư mục chứa các file XML Hóa đơn điện tử",
                                    UseDescriptionForTitle = true,
                                    ShowNewFolderButton = false
                                };

                                if (fbd.ShowDialog(this) == DialogResult.OK && !string.IsNullOrWhiteSpace(fbd.SelectedPath))
                                {
                                    await ExecuteParseXmlDirectoryAsync(fbd.SelectedPath, xmlRequestId, parseOptions);
                                }
                                else
                                {
                                    var cancelPayload = new
                                    {
                                        action = "PARSE_XML_CANCELLED",
                                        message = "Người dùng đã hủy chọn thư mục.",
                                        requestId = xmlRequestId
                                    };
                                    await PostWebMessageAsJson(cancelPayload);
                                }
                            }));
                        }
                        else
                        {
                            _ = ExecuteParseXmlDirectoryAsync(directoryPath, xmlRequestId, parseOptions);
                        }
                    }
                    catch (Exception exOuter)
                    {
                        Debug.WriteLine("Lỗi PARSE_XML_DIRECTORY: " + exOuter.Message);
                    }
                    break;

            }
        }

        /// <summary>
        /// Quét và bóc tách dữ liệu hóa đơn từ các file XML trong thư mục được chỉ định
        /// </summary>
        private async Task ExecuteParseXmlDirectoryAsync(string directoryPath, string? requestId, XmlParseOptions? options = null)
        {
            try
            {
                // Thông báo bắt đầu quét
                await PostWebMessageAsJson(new
                {
                    action = "PARSE_XML_START",
                    requestId = requestId,
                    directoryPath = directoryPath,
                    message = "Bắt đầu quét và đọc các file XML hóa đơn..."
                });

                // Lấy danh sách hóa đơn tóm tắt hiện có trong bộ nhớ RAM để ghép nối nếu có
                var existingInvoices = InvoiceMemoryService.Instance.GetInvoices();

                // Tạo đối tượng báo cáo tiến độ bóc tách
                var progress = new Progress<(int current, int total, string currentFile)>(async p =>
                {
                    try
                    {
                        await PostWebMessageAsJson(new
                        {
                            action = "PARSE_XML_PROGRESS",
                            requestId = requestId,
                            current = p.current,
                            total = p.total,
                            currentFile = p.currentFile
                        });
                    }
                    catch { }
                });

                // Chạy ngầm việc quét và bóc tách file XML
                var result = await XmlInvoiceParserService.Instance.ParseDirectoryAsync(directoryPath, options, existingInvoices, progress);

                // Trả kết quả hoàn tất về cho Frontend
                await PostWebMessageAsJson(new
                {
                    action = "PARSE_XML_DONE",
                    requestId = requestId,
                    success = result.Success,
                    message = result.Message,
                    totalFound = result.TotalFilesFound,
                    totalParsed = result.TotalFilesParsed,
                    totalSkipped = result.TotalFilesSkipped,
                    totalFailed = result.TotalFilesFailed,
                    invoices = result.Invoices,
                    errors = result.Errors
                });
            }
            catch (Exception ex)
            {
                Debug.WriteLine("[UserForm] Lỗi ExecuteParseXmlDirectoryAsync: " + ex.Message);
                await PostWebMessageAsJson(new
                {
                    action = "PARSE_XML_ERROR",
                    requestId = requestId,
                    message = "Lỗi bóc tách XML: " + ex.Message
                });
            }
        }
        private async Task SendSettingsDataToWeb()
        {
            try
            {
                // Gửi danh sách công ty từ cache SQLite lên giao diện Web
                string jsonData = JsonConvert.SerializeObject(_cachedCompanies ?? new List<CompanyModel>());
                string safeJson = JsonConvert.ToString(jsonData);
                await ExecuteScriptSerializedAsync($"window.loadInitialData({safeJson})");
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("Lỗi bắn dữ liệu Settings: " + ex.Message);
            }
        }
        private void UserForm_FormClosing(object sender, FormClosingEventArgs e)
        {
            if (_isForceClosing)
            {
                return;
            }

            // Hủy sự kiện đóng mặc định của Windows
            e.Cancel = true;

            // Gọi modal xác nhận thoát đẹp mắt trong WebView2
            try
            {
                if (webViewUser?.CoreWebView2 != null)
                {
                    _ = webViewUser.CoreWebView2.ExecuteScriptAsync(
                        "if (typeof window.showConfirm === 'function') { " +
                        "    window.showConfirm({ " +
                        "        title: 'Xác nhận thoát', " +
                        "        message: 'Bạn có chắc muốn thoát? Tất cả chương trình đang chạy sẽ bị đóng.', " +
                        "        confirmText: 'THOÁT NGAY', " +
                        "        cancelText: 'HỦY', " +
                        "        type: 'logout', " +
                        "        onConfirm: function() { " +
                        "            window.chrome.webview.postMessage({ action: 'FORCE_EXIT' }); " +
                        "        } " +
                        "    }); " +
                        "} else { " +
                        "    window.chrome.webview.postMessage({ action: 'FORCE_EXIT' }); " +
                        "}"
                    );
                }
                else
                {
                    _isForceClosing = true;
                    this.Close();
                }
            }
            catch
            {
                _isForceClosing = true;
                this.Close();
            }
        }

        private async void UserForm_FormClosed(object sender, FormClosedEventArgs e)
        {
            // Giải phóng bộ nhớ RAM hóa đơn
            InvoiceMemoryService.Instance.Clear("Đóng ứng dụng");

            // Đóng Chrome hoàn toàn khi thoát app
            await _taxService.ShutdownAsync();

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
            else
            {
                // Captcha lỗi — báo user thử lại
                await webViewUser.CoreWebView2.ExecuteScriptAsync(
                    "window.showAlert('Lỗi', 'Không lấy được captcha. Vui lòng thử lại!', 'error')"
                );
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
        private async Task LoadCompaniesToRAM()
        {
            try
            {
                _cachedCompanies = await _databaseService.GetAllAccountsAsync();
                System.Diagnostics.Debug.WriteLine($">>> SQLite: Đã nạp {_cachedCompanies.Count} công ty.");
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("Lỗi nạp cache từ SQLite: " + ex.Message);
            }
        }
        private async Task HandleInvoiceSearchStream(string from, string to, string invoiceType = "ban_ra", string invoiceCategory = "dien_tu", string checkResult = "", CancellationToken cancellationToken = default)
        {
            try
            {
                // Xóa RAM hóa đơn cũ trước khi nạp đợt mới
                InvoiceMemoryService.Instance.Clear("Bắt đầu tìm kiếm đợt mới");
                _currentInvoicesRAM.Clear();
                await _taxService.SearchInvoicesSSE(from, to, invoiceType, invoiceCategory, checkResult, (progressData) =>
                {
                    // Lưu hóa đơn vào RAM C# khi hoàn tất
                    if (progressData.IsDone && progressData.Invoices != null)
                    {
                        InvoiceMemoryService.Instance.SetInvoices(from, progressData.Invoices);
                        _currentInvoicesRAM = progressData.Invoices;
                    }

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


        private async Task HandleStartCrawl(string path, string requestId = null, string mst = "", string invoiceType = "ban_ra")
        {
            if (_isCrawling) return;
            _currentCrawlOutputDir = path;
            _crawlFolderIsDedicated = false;
            _currentCrawlMst = mst;
            _currentCrawlInvoiceType = invoiceType;
            _currentCrawlRequestId = requestId;

            var (status, message, finalOutputDir) = await _taxService.StartCrawlAsync(path, mst, invoiceType);
            if (!string.IsNullOrWhiteSpace(finalOutputDir))
            {
                _currentCrawlOutputDir = finalOutputDir;
                _crawlFolderIsDedicated = true;
            }

            if (status == "success")
            {
                _isCrawling = true;
                _ = RunStatusPolling();
            }
            else
            {
                string safeStartMsg = JsonConvert.ToString("Không thể bắt đầu: " + message);
                await webViewUser.CoreWebView2.ExecuteScriptAsync($"if(window.showAlert) window.showAlert('Lỗi khởi động', {safeStartMsg}, 'error');");
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

                // TỰ ĐỘNG BÓC TÁCH DỮ LIỆU XML NGAY SAU KHI TẢI XONG
                XmlParseResult parseResult = null;
                if (!string.IsNullOrWhiteSpace(_currentCrawlOutputDir) && Directory.Exists(_currentCrawlOutputDir))
                {
                    try
                    {
                        await PostWebMessageAsJson(new
                        {
                            action = "CRAWL_STATUS_UPDATE",
                            status = "Đang bóc tách chi tiết dữ liệu XML...",
                            is_running = true,
                            total_invoices = 0,
                            requestId = _currentCrawlRequestId
                        });

                        var parseOptions = new XmlParseOptions
                        {
                            TargetMst = _currentCrawlMst,
                            IsMuaVao = _currentCrawlInvoiceType == "mua_vao",
                            DeleteAfterParsing = _crawlFolderIsDedicated
                        };

                        parseResult = await XmlInvoiceParserService.Instance.ParseDirectoryAsync(
                            _currentCrawlOutputDir,
                            parseOptions,
                            InvoiceMemoryService.Instance.GetInvoices()
                        );

                        if (parseResult?.Invoices != null && parseResult.Invoices.Count > 0)
                        {
                            InvoiceMemoryService.Instance.SetParsedXmlInvoices(parseResult.Invoices);
                        }
                    }
                    catch (Exception exParse)
                    {
                        Debug.WriteLine("[UserForm] Lỗi tự động bóc tách XML sau khi cào: " + exParse.Message);
                    }
                }

                await PostWebMessageAsJson(new
                {
                    action = "CRAWL_FINISHED",
                    requestId = _currentCrawlRequestId,
                    output_dir = _currentCrawlOutputDir,
                    total_files_found = parseResult?.TotalFilesFound ?? 0,
                    parsed_xml_count = parseResult?.TotalFilesParsed ?? 0,
                    total_files_skipped = parseResult?.TotalFilesSkipped ?? 0,
                    total_files_failed = parseResult?.TotalFilesFailed ?? 0,
                    parsed_invoices = parseResult?.Invoices
                });

                // clear stored request id after finishing
                _currentCrawlRequestId = null;
                _currentCrawlOutputDir = null;
            }
        }
        private Task PostWebMessageAsJson(object data)
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
            return Task.CompletedTask;
        }



    }
}
