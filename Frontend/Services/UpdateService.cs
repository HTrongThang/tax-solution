using System;
using System.Diagnostics;
using System.Net.Http;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace Frontend.Services
{
    public class UpdateInfoRemote
    {
        [JsonPropertyName("latest_version")]
        public string LatestVersion { get; set; } = "";

        [JsonPropertyName("min_required_version")]
        public string MinRequiredVersion { get; set; } = "";

        [JsonPropertyName("download_url")]
        public string DownloadUrl { get; set; } = "";

        [JsonPropertyName("release_notes")]
        public string ReleaseNotes { get; set; } = "";

        [JsonPropertyName("is_force_update")]
        public bool IsForceUpdate { get; set; } = false;
    }

    public class UpdateCheckResult
    {
        public bool Success { get; set; }
        public bool HasUpdate { get; set; }
        public bool IsForceUpdate { get; set; }
        public string CurrentVersion { get; set; } = "";
        public string LatestVersion { get; set; } = "";
        public string DownloadUrl { get; set; } = "";
        public string ReleaseNotes { get; set; } = "";
        public string ErrorMessage { get; set; } = "";
    }

    public static class UpdateService
    {
        // URL file version.json trên GitHub Gist (public, độc lập với repo riêng tư)
        public const string VersionCheckUrl = "https://gist.githubusercontent.com/HTrongThang/9acd8fd06c46b40e3a99f9eda5a85602/raw/version.json";

        /// <summary>
        /// Gửi request kiểm tra phiên bản mới ngầm trên mạng
        /// </summary>
        public static async Task<UpdateCheckResult> CheckForUpdateAsync(string? customUrl = null)
        {
            var result = new UpdateCheckResult
            {
                CurrentVersion = AppVersionHelper.CurrentVersion
            };

            try
            {
                string targetUrl = string.IsNullOrWhiteSpace(customUrl) ? VersionCheckUrl : customUrl;
                // Thêm query timestamp để chống cache của GitHub CDN
                string urlWithTimestamp = targetUrl + (targetUrl.Contains("?") ? "&" : "?") + "t=" + DateTimeOffset.UtcNow.ToUnixTimeSeconds();

                using var client = new HttpClient();
                client.Timeout = TimeSpan.FromSeconds(5);
                client.DefaultRequestHeaders.Add("User-Agent", "TaxSolution-Client");

                var response = await client.GetAsync(urlWithTimestamp);
                if (!response.IsSuccessStatusCode)
                {
                    result.Success = false;
                    result.ErrorMessage = $"HTTP {(int)response.StatusCode}";
                    return result;
                }

                string jsonContent = await response.Content.ReadAsStringAsync();
                var options = new JsonSerializerOptions { PropertyNameCaseInsensitive = true };
                var info = JsonSerializer.Deserialize<UpdateInfoRemote>(jsonContent, options);

                if (info == null || string.IsNullOrWhiteSpace(info.LatestVersion))
                {
                    result.Success = false;
                    result.ErrorMessage = "Dữ liệu phiên bản không hợp lệ";
                    return result;
                }

                result.Success = true;
                result.LatestVersion = info.LatestVersion.Trim();
                string rawDownloadUrl = info.DownloadUrl?.Trim() ?? "";
                if (!string.IsNullOrWhiteSpace(rawDownloadUrl))
                {
                    // Nếu là đường dẫn cục bộ (D:\...) nhưng không tồn tại trên máy này -> fallback link web
                    if (!rawDownloadUrl.StartsWith("http://", StringComparison.OrdinalIgnoreCase) &&
                        !rawDownloadUrl.StartsWith("https://", StringComparison.OrdinalIgnoreCase) &&
                        !File.Exists(rawDownloadUrl))
                    {
                        rawDownloadUrl = "https://github.com/HTrongThang/tax-distribution/releases/latest";
                    }
                }
                else
                {
                    rawDownloadUrl = "https://github.com/HTrongThang/tax-distribution/releases/latest";
                }
                result.DownloadUrl = rawDownloadUrl;
                result.ReleaseNotes = info.ReleaseNotes?.Trim() ?? "";

                // So sánh phiên bản
                Version curVer = AppVersionHelper.VersionObj;
                if (Version.TryParse(result.LatestVersion, out var latestVer))
                {
                    result.HasUpdate = latestVer > curVer;
                }

                // Kiểm tra có bị ép cập nhật (Force Update) không
                bool forcedFlag = info.IsForceUpdate;
                if (!forcedFlag && Version.TryParse(info.MinRequiredVersion, out var minRequiredVer))
                {
                    forcedFlag = curVer < minRequiredVer;
                }

                result.IsForceUpdate = result.HasUpdate && forcedFlag;
                return result;
            }
            catch (Exception ex)
            {
                // Khi không có mạng hoặc request lỗi, không làm dừng ứng dụng
                result.Success = false;
                result.ErrorMessage = ex.Message;
                return result;
            }
        }

        /// <summary>
        /// Mở link tải bản mới trong trình duyệt mặc định
        /// </summary>
        public static void OpenDownloadPage(string downloadUrl)
        {
            try
            {
                // Luôn đảm bảo URL hợp lệ trước khi mở trình duyệt
                if (string.IsNullOrWhiteSpace(downloadUrl) ||
                    (!downloadUrl.StartsWith("http://", StringComparison.OrdinalIgnoreCase) &&
                     !downloadUrl.StartsWith("https://", StringComparison.OrdinalIgnoreCase)))
                {
                    downloadUrl = "https://github.com/HTrongThang/tax-distribution/releases/latest";
                }

                Process.Start(new ProcessStartInfo
                {
                    FileName = downloadUrl,
                    UseShellExecute = true
                });
            }
            catch (Exception ex)
            {
                MessageBox.Show($"Không thể tự động mở trình duyệt: {ex.Message}\nVui lòng truy cập: {downloadUrl}",
                    "Lỗi mở liên kết", MessageBoxButtons.OK, MessageBoxIcon.Warning);
            }
        }
    }
}
