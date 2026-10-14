using Frontend.Models;
using Newtonsoft.Json;
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net.Http;
using System.Net.Http.Json;
using System.Reflection;
using System.Text;
using System.Threading.Tasks;

namespace Frontend.Services
{
    internal class TaxService
    {
        private readonly HttpClient _client = new HttpClient
        {
            BaseAddress = new Uri("http://127.0.0.1:18386"),
            Timeout = TimeSpan.FromSeconds(30)
        };

        // Client riêng cho SSE (stream dài hạn) — timeout do CancellationToken quản lý
        private readonly HttpClient _sseClient = new HttpClient
        {
            BaseAddress = new Uri("http://127.0.0.1:18386"),
            Timeout = System.Threading.Timeout.InfiniteTimeSpan
        };

        public async Task<string> Init()
        {
            var response = await _client.GetAsync("/api/tax/init");
            return await response.Content.ReadAsStringAsync();
        }

        // Mở dialog đăng nhập trên tab hiện tại (sau logout, không mở tab mới)
        public async Task<(string status, string message)> ShowLoginDialogAsync()
        {
            try
            {
                using var cts = new System.Threading.CancellationTokenSource(TimeSpan.FromSeconds(15));
                var response = await _client.GetAsync("/api/tax/showLogin", cts.Token);
                var content = await response.Content.ReadAsStringAsync();

                if (response.IsSuccessStatusCode)
                {
                    dynamic result = JsonConvert.DeserializeObject(content);
                    return (result.status.ToString(), result.message.ToString());
                }
                return ("error", $"Server báo lỗi: {response.StatusCode}");
            }
            catch (Exception ex)
            {
                return ("error", "Lỗi kết nối: " + ex.Message);
            }
        }

        public async Task<string> GetCaptchaBase64()
        {
            try
            {
                // 1. Gọi API
                var response = await _client.GetAsync("/api/tax/captcha");
                var content = await response.Content.ReadAsStringAsync();

                // Xử lý cả trường hợp lỗi 500 (không crash app)
                if (!response.IsSuccessStatusCode)
                {
                    System.Diagnostics.Debug.WriteLine($"GetCaptcha lỗi {response.StatusCode}: {content}");
                    return null;
                }

                // 2. Parse dùng Newtonsoft
                dynamic result = JsonConvert.DeserializeObject<dynamic>(content);

                // 3. Kiểm tra và trả về
                if (result != null && (result.status == "success" || result.Status == "success"))
                {
                    return result.data ?? result.Data;
                }
                else
                {
                    string msg = result?.message ?? "Lỗi không xác định từ Backend";
                    System.Diagnostics.Debug.WriteLine("GetCaptcha: " + msg);
                    return null;
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("Lỗi GetCaptcha: " + ex.Message);
                return null;
            }
        }

        public async Task<CrawlStatus> GetStatus()
            {
                try
                {
                    return await _client.GetFromJsonAsync<CrawlStatus>("/api/tax/status");
                }
                catch
                {
                    return new CrawlStatus { current_status = "Đang chờ Server..." };
                }
            }

        public async Task<(string Status, string Message)> LoginTax(string taxCode, string password, string captcha)
        {
            try
            {
                var loginData = new { tax_code = taxCode, Password = password, Captcha = captcha };
                var content = new StringContent(JsonConvert.SerializeObject(loginData), Encoding.UTF8, "application/json");

                // Gọi API Login
                var response = await _client.PostAsync("/api/tax/login", content);
                var responseBody = await response.Content.ReadAsStringAsync();

                // Parse kết quả dùng Newtonsoft.Json
                dynamic result = JsonConvert.DeserializeObject(responseBody);

                // Trả về cả Status (để check error_captcha) và Message (để hiển thị)
                return (result?.status?.ToString() ?? "error", result?.message?.ToString() ?? "Lỗi không xác định");
            }
            catch (Exception ex)
            {
                return ("error", "Không thể kết nối, hãy khởi động lại ứng dụng: " + ex.Message);
            }
        }
        // Soft logout: đăng xuất trang thuế, giữ Chrome mở
        public async Task<(string status, string message)> LogoutTaxAsync()
        {
            try
            {
                using var cts = new System.Threading.CancellationTokenSource(TimeSpan.FromSeconds(15));
                var response = await _client.PostAsync("/api/tax/logout", null, cts.Token);
                var content = await response.Content.ReadAsStringAsync();

                if (response.IsSuccessStatusCode)
                {
                    dynamic result = JsonConvert.DeserializeObject(content);
                    return (result.status.ToString(), result.message.ToString());
                }
                else
                {
                    return ("error", $"Server báo lỗi: {response.StatusCode}");
                }
            }
            catch (Exception ex)
            {
                return ("error", "Không thể kết nối tới Backend Go: " + ex.Message);
            }
        }

        // Shutdown: Đóng Chrome hoàn toàn. Gọi khi thoát app.
        public async Task ShutdownAsync()
        {
            try
            {
                using var cts = new System.Threading.CancellationTokenSource(TimeSpan.FromSeconds(10));
                await _client.PostAsync("/api/tax/shutdown", null, cts.Token);
            }
            catch
            {
                // Bỏ qua lỗi khi shutdown (app đang đóng)
            }
        }

        // Optional: fetch specific page (backend must support this endpoint)
        public async Task<List<dynamic>> GetInvoicesPageAsync(int page, int pageSize)
        {
            try
            {
                var response = await _client.GetAsync($"/api/tax/invoices?page={page}&pageSize={pageSize}");
                response.EnsureSuccessStatusCode();
                var body = await response.Content.ReadAsStringAsync();
                var data = JsonConvert.DeserializeObject<List<dynamic>>(body);
                return data;
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("GetInvoicesPageAsync error: " + ex.Message);
                return null;
            }
        }

        public async Task SearchInvoicesSSE(string fromDate, string toDate, string invoiceType = "ban_ra", string invoiceCategory = "dien_tu", string checkResult = "", Action<StreamData> onProgress = null, System.Threading.CancellationToken cancellationToken = default)
        {
            // Improved SSE reader with cancellation support and better logging
            try
            {
                // Dùng ParseExact với format cố định để tránh lỗi do Regional Settings khác nhau giữa các máy
                // (Máy VN: dd/MM/yyyy, Máy EN-US: MM/dd/yyyy → crash khi ngày > 12)
                string fDate, tDate;
                string trimmedFrom = fromDate.Trim();
                string trimmedTo = toDate.Trim();

                // Hỗ trợ cả 2 format phổ biến: dd/MM/yyyy và yyyy-MM-dd (ISO)
                string[] supportedFormats = { "dd/MM/yyyy", "yyyy-MM-dd", "d/M/yyyy", "M/d/yyyy" };
                var culture = System.Globalization.CultureInfo.InvariantCulture;

                if (DateTime.TryParseExact(trimmedFrom, supportedFormats, culture, System.Globalization.DateTimeStyles.None, out DateTime startDate))
                {
                    fDate = startDate.ToString("dd/MM/yyyy", culture);
                }
                else
                {
                    fDate = trimmedFrom; // Fallback: giữ nguyên chuỗi gốc
                }

                if (DateTime.TryParseExact(trimmedTo, supportedFormats, culture, System.Globalization.DateTimeStyles.None, out DateTime endDate))
                {
                    tDate = endDate.ToString("dd/MM/yyyy", culture);
                }
                else
                {
                    tDate = trimmedTo; // Fallback: giữ nguyên chuỗi gốc
                }

                string safeType = Uri.EscapeDataString(string.IsNullOrEmpty(invoiceType) ? "ban_ra" : invoiceType);
                string safeCat = Uri.EscapeDataString(string.IsNullOrEmpty(invoiceCategory) ? "dien_tu" : invoiceCategory);
                string safeCheckResult = Uri.EscapeDataString(checkResult ?? "");
                var requestUri = $"/api/tax/searchInvoice?from={fDate}&to={tDate}&type={safeType}&category={safeCat}&check_result={safeCheckResult}";
                using (var response = await _sseClient.GetAsync(requestUri, HttpCompletionOption.ResponseHeadersRead, cancellationToken))
                {
                    // Xử lý riêng 409 Conflict (hệ thống đang bận)
                    if ((int)response.StatusCode == 409)
                    {
                        onProgress?.Invoke(new StreamData
                        {
                            Status = "⚠️ Hệ thống đang bận xử lý tác vụ khác. Vui lòng đợi hoàn thành trước.",
                            IsDone = true
                        });
                        return;
                    }
                    response.EnsureSuccessStatusCode();

                    using (var stream = await response.Content.ReadAsStreamAsync())
                    using (var reader = new StreamReader(stream))
                    {
                        var sb = new System.Text.StringBuilder();
                        while (true)
                        {
                            if (cancellationToken.IsCancellationRequested)
                            {
                                onProgress?.Invoke(new StreamData { Status = "Đã hủy bởi người dùng", IsDone = true });
                                break;
                            }
                            var line = await reader.ReadLineAsync();
                            if (line == null)
                            {
                                if (sb.Length > 0)
                                {
                                    try
                                    {
                                        var json = sb.ToString();
                                        var data = JsonConvert.DeserializeObject<StreamData>(json);
                                        if (data != null) onProgress?.Invoke(data);
                                        if (data?.IsDone == true) break;
                                    }
                                    catch (Exception ex)
                                    {
                                        System.Diagnostics.Debug.WriteLine("SSE final chunk parse failed: " + ex.Message + " payload:" + sb.ToString());
                                    }
                                }
                                break;
                            }

                            var trimmed = line.TrimEnd('\r');

                            if (string.IsNullOrWhiteSpace(trimmed))
                            {
                                if (sb.Length > 0)
                                {
                                    try
                                    {
                                        var json = sb.ToString();
                                        var data = JsonConvert.DeserializeObject<StreamData>(json);
                                        if (data != null)
                                        {
                                            onProgress?.Invoke(data);
                                            if (data.IsDone) break;
                                        }
                                    }
                                    catch (Exception ex)
                                    {
                                        System.Diagnostics.Debug.WriteLine("Parse SSE event failed: " + ex.Message + " payload:" + sb.ToString());
                                    }
                                    sb.Clear();
                                }
                                if (cancellationToken.IsCancellationRequested)
                                {
                                    onProgress?.Invoke(new StreamData { Status = "Đã hủy bởi người dùng", IsDone = true });
                                    break;
                                }
                                continue;
                            }

                            if (trimmed.StartsWith("data:"))
                            {
                                var payload = trimmed.Substring(5);
                                if (payload.StartsWith(" ")) payload = payload.Substring(1);
                                if (sb.Length > 0) sb.Append('\n');
                                sb.Append(payload);
                            }
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                if (ex is OperationCanceledException)
                {
                    System.Diagnostics.Debug.WriteLine("SearchInvoicesSSE canceled");
                    onProgress?.Invoke(new StreamData { Status = "Đã hủy", IsDone = true });
                }
                else
                {
                    System.Diagnostics.Debug.WriteLine("SearchInvoicesSSE error: " + ex.Message);
                    onProgress?.Invoke(new StreamData { Status = "Lỗi kết nối: " + ex.Message, IsDone = true });
                }
            }
        }
        public async Task<(string Status, string Message, string OutputDir)> StartCrawlAsync(string path, string mst = "", string invoiceType = "ban_ra")
        {
            try
            {
                // Gửi đường dẫn lưu file hoặc tham số cấu hình xuống Go
                var payload = new { path = path, mst = mst, invoice_type = invoiceType };

                // Gọi đến endpoint /api/tax/start-crawl mà mình vừa thiết lập ở Go
                var response = await _client.PostAsJsonAsync("/api/tax/start-crawl", payload);

                var content = await response.Content.ReadAsStringAsync();
                dynamic result = JsonConvert.DeserializeObject(content);

                if (response.IsSuccessStatusCode)
                {
                    string finalOutputDir = result?.output_dir?.ToString() ?? path;
                    return (result?.status?.ToString() ?? "success", result?.message?.ToString() ?? "Đã bắt đầu tiến trình", finalOutputDir);
                }
                else
                {
                    return ("error", result?.message?.ToString() ?? "Server từ chối yêu cầu", path);
                }
            }
            catch (Exception ex)
            {
                return ("error", $"Lỗi kết nối khi gọi StartCrawl: {ex.Message}", path);
            }
        }

        public async Task StopTaxAsync()
        {
            try
            {
                using var cts = new System.Threading.CancellationTokenSource(TimeSpan.FromSeconds(5));
                await _client.PostAsync("/api/tax/stop", null, cts.Token);
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("StopTaxAsync error: " + ex.Message);
            }
        }
    }
}
