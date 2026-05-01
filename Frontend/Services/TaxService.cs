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
            BaseAddress = new Uri("http://localhost:8080"),
            Timeout = System.Threading.Timeout.InfiniteTimeSpan
        };

        public async Task<string> Init() => await _client.GetStringAsync("/api/tax/init");

        public async Task<string> GetCaptchaBase64()
        {
            try
            {
                // 1. Gọi API
                var response = await _client.GetAsync("/api/tax/captcha");

                // Đảm bảo request thành công (200 OK)
                response.EnsureSuccessStatusCode();

                var content = await response.Content.ReadAsStringAsync();

                // 2. Parse dùng Newtonsoft (Ép kiểu về dynamic để dùng dấu chấm)
                dynamic result = JsonConvert.DeserializeObject<dynamic>(content);

                // 3. Kiểm tra và trả về (Lưu ý: Go trả về Status viết hoa hay thường thì C# phải viết y hệt)
                if (result != null && (result.status == "success" || result.Status == "success"))
                {
                    // Trả về trường Data (viết hoa chữ D nếu Go định nghĩa vậy)
                    return result.data ?? result.Data;
                }
                else
                {
                    string msg = result?.message ?? "Lỗi không xác định từ Backend";
                    throw new Exception(msg);
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("Lỗi GetCaptcha: " + ex.Message);
                throw; // Đẩy lỗi ra ngoài để Form hiện Dialog
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
        public async Task<(string status, string message)> LogoutTaxAsync()
        {
            try
            {
                using (var client = new HttpClient())
                {
                    client.Timeout = TimeSpan.FromSeconds(10);
                    var response = await client.PostAsync("http://localhost:8080/api/tax/logout", null);
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
            }
            catch (Exception ex)
            {
                return ("error", "Không thể kết nối tới Backend Go: " + ex.Message);
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

        public async Task SearchInvoicesSSE(string fromDate, string toDate, Action<StreamData> onProgress, System.Threading.CancellationToken cancellationToken = default)
        {
            // Improved SSE reader with cancellation support and better logging
            try
            {
                DateTime startDate = DateTime.Parse(fromDate);
                DateTime endDate = DateTime.Parse(toDate);

                string fDate = startDate.ToString("dd/MM/yyyy");
                string tDate = endDate.ToString("dd/MM/yyyy");

                var requestUri = $"/api/tax/searchInvoice?from={fDate}&to={tDate}";
                using (var response = await _client.GetAsync(requestUri, HttpCompletionOption.ResponseHeadersRead, cancellationToken))
                {
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
        public async Task<(string Status, string Message)> StartCrawlAsync(string path)
        {
            try
            {
                // Gửi đường dẫn lưu file hoặc tham số cấu hình xuống Go
                var payload = new { path = path };

                // Gọi đến endpoint /api/tax/start-crawl mà mình vừa thiết lập ở Go
                var response = await _client.PostAsJsonAsync("/api/tax/start-crawl", payload);

                var content = await response.Content.ReadAsStringAsync();
                dynamic result = JsonConvert.DeserializeObject(content);

                if (response.IsSuccessStatusCode)
                {
                    return (result?.status?.ToString() ?? "success", result?.message?.ToString() ?? "Đã bắt đầu tiến trình");
                }
                else
                {
                    return ("error", result?.message?.ToString() ?? "Server từ chối yêu cầu");
                }
            }
            catch (Exception ex)
            {
                return ("error", $"Lỗi kết nối khi gọi StartCrawl: {ex.Message}");
            }
        }
    }
}
