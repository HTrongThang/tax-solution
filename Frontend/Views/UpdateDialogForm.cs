using System;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Net.Http;
using System.Threading.Tasks;
using System.Windows.Forms;
using Frontend.Services;

namespace Frontend.Views
{
    public class UpdateDialogForm : Form
    {
        private readonly UpdateCheckResult _updateInfo;
        private bool _allowExitWithoutAppKill = false;

        public UpdateDialogForm(UpdateCheckResult updateInfo)
        {
            _updateInfo = updateInfo ?? throw new ArgumentNullException(nameof(updateInfo));
            InitializeUI();
        }

        private void InitializeUI()
        {
            try
            {
                this.Icon = Icon.ExtractAssociatedIcon(Application.ExecutablePath);
            }
            catch { }
            this.Text = _updateInfo.IsForceUpdate ? "Cập nhật ứng dụng bắt buộc" : "Thông báo cập nhật ứng dụng";
            this.Size = new Size(580, 490);
            this.StartPosition = FormStartPosition.CenterScreen;
            this.FormBorderStyle = FormBorderStyle.FixedDialog;
            this.MaximizeBox = false;
            this.MinimizeBox = false;
            this.BackColor = Color.White;
            this.Font = new Font("Segoe UI", 9.5f, FontStyle.Regular);
            this.ShowInTaskbar = true;
            this.TopMost = true;

            // 1. Header Panel
            var pnlHeader = new Panel
            {
                Dock = DockStyle.Top,
                Height = 85,
                BackColor = _updateInfo.IsForceUpdate ? Color.FromArgb(239, 68, 68) : Color.FromArgb(30, 41, 59),
                Padding = new Padding(24, 16, 24, 16)
            };

            var lblHeaderTitle = new Label
            {
                Text = _updateInfo.IsForceUpdate ? "⚠️ YÊU CẦU CẬP NHẬT BẮT BUỘC" : "🚀 PHIÊN BẢN MỚI ĐÃ SẴN SÀNG",
                Font = new Font("Segoe UI", 12f, FontStyle.Bold),
                ForeColor = Color.White,
                AutoSize = true,
                Location = new Point(20, 16)
            };

            var lblHeaderSub = new Label
            {
                Text = $"Phiên bản hiện tại: v{_updateInfo.CurrentVersion}  ➜  Phiên bản mới nhất: v{_updateInfo.LatestVersion}",
                Font = new Font("Segoe UI", 9.5f, FontStyle.Regular),
                ForeColor = Color.FromArgb(241, 245, 249),
                AutoSize = true,
                Location = new Point(22, 45)
            };

            pnlHeader.Controls.Add(lblHeaderTitle);
            pnlHeader.Controls.Add(lblHeaderSub);
            this.Controls.Add(pnlHeader);

            // 2. Body Panel
            var pnlBody = new Panel
            {
                Location = new Point(20, 95),
                Size = new Size(525, 285),
                BackColor = Color.Transparent
            };

            var lblNotice = new Label
            {
                Location = new Point(0, 5),
                Size = new Size(525, 45),
                Font = new Font("Segoe UI", 9.5f, FontStyle.Regular),
                ForeColor = Color.FromArgb(51, 65, 85),
                Text = _updateInfo.IsForceUpdate
                    ? "Nhằm nâng cao chất lượng trải nghiệm và bảo mật dữ liệu, TaxSolution đã phát hành bản nâng cấp mới. Quý khách vui lòng bấm 'Cập nhật ngay' để tiếp tục sử dụng."
                    : "Đã có bản cập nhật mới với nhiều cải tiến và tối ưu hóa hệ thống. Bạn có muốn nâng cấp ngay không?"
            };
            pnlBody.Controls.Add(lblNotice);

            var lblNotesTitle = new Label
            {
                Text = "Nội dung nâng cấp & điểm mới:",
                UseMnemonic = false,
                Font = new Font("Segoe UI", 9.5f, FontStyle.Bold),
                ForeColor = Color.FromArgb(30, 41, 59),
                Location = new Point(0, 55),
                AutoSize = true
            };
            pnlBody.Controls.Add(lblNotesTitle);

            var txtNotes = new TextBox
            {
                Location = new Point(0, 80),
                Size = new Size(525, 195),
                Multiline = true,
                ReadOnly = true,
                TabStop = false,
                ScrollBars = ScrollBars.Vertical,
                BackColor = Color.FromArgb(248, 250, 252),
                ForeColor = Color.FromArgb(51, 65, 85),
                BorderStyle = BorderStyle.FixedSingle,
                Font = new Font("Segoe UI", 9.5f, FontStyle.Regular),
                Text = string.IsNullOrWhiteSpace(_updateInfo.ReleaseNotes) 
                    ? "- Nâng cấp hiệu năng và sửa lỗi hệ thống." 
                    : _updateInfo.ReleaseNotes.Replace("\n", Environment.NewLine)
            };
            txtNotes.GotFocus += (s, e) => { txtNotes.DeselectAll(); };
            pnlBody.Controls.Add(txtNotes);
            this.Controls.Add(pnlBody);

            // 3. Footer Action / Download Progress Panel
            var pnlFooter = new Panel
            {
                Dock = DockStyle.Bottom,
                Height = 75,
                BackColor = Color.FromArgb(248, 250, 252)
            };

            // Thanh tiến trình tải (ẩn mặc định)
            var lblStatus = new Label
            {
                Location = new Point(20, 10),
                Size = new Size(525, 20),
                Font = new Font("Segoe UI", 9.5f, FontStyle.Bold),
                ForeColor = Color.FromArgb(16, 185, 129),
                Visible = false
            };
            pnlFooter.Controls.Add(lblStatus);

            var prgDownload = new ProgressBar
            {
                Location = new Point(20, 35),
                Size = new Size(525, 22),
                Minimum = 0,
                Maximum = 100,
                Value = 0,
                Visible = false
            };
            pnlFooter.Controls.Add(prgDownload);

            // Nút bấm Cập nhật ngay
            var btnUpdate = new Button
            {
                Text = "📥 Cập nhật ngay",
                Size = new Size(180, 44),
                Location = new Point(_updateInfo.IsForceUpdate ? 180 : 345, 16),
                BackColor = Color.FromArgb(16, 185, 129), // Emerald
                ForeColor = Color.White,
                FlatStyle = FlatStyle.Flat,
                Font = new Font("Segoe UI", 10f, FontStyle.Bold),
                Cursor = Cursors.Hand
            };
            btnUpdate.FlatAppearance.BorderSize = 0;
            pnlFooter.Controls.Add(btnUpdate);

            Button? btnCancel = null;
            Button? btnExit = null;

            if (!_updateInfo.IsForceUpdate)
            {
                btnCancel = new Button
                {
                    Text = "Để sau",
                    Size = new Size(110, 44),
                    Location = new Point(220, 16),
                    BackColor = Color.FromArgb(226, 232, 240),
                    ForeColor = Color.FromArgb(71, 85, 105),
                    FlatStyle = FlatStyle.Flat,
                    Font = new Font("Segoe UI", 9.5f, FontStyle.Regular),
                    Cursor = Cursors.Hand
                };
                btnCancel.FlatAppearance.BorderSize = 0;
                btnCancel.Click += (s, e) =>
                {
                    _allowExitWithoutAppKill = true;
                    this.Close();
                };
                pnlFooter.Controls.Add(btnCancel);
            }
            else
            {
                btnExit = new Button
                {
                    Text = "Thoát",
                    Size = new Size(110, 44),
                    Location = new Point(370, 16),
                    BackColor = Color.FromArgb(226, 232, 240),
                    ForeColor = Color.FromArgb(100, 116, 139),
                    FlatStyle = FlatStyle.Flat,
                    Font = new Font("Segoe UI", 9.5f, FontStyle.Regular),
                    Cursor = Cursors.Hand
                };
                btnExit.FlatAppearance.BorderSize = 0;
                btnExit.Click += (s, e) =>
                {
                    Application.Exit();
                };
                pnlFooter.Controls.Add(btnExit);
            }

            btnUpdate.Click += async (s, e) =>
            {
                await StartDownloadAndInstallAsync(btnUpdate, btnCancel, btnExit, prgDownload, lblStatus);
            };

            this.Controls.Add(pnlFooter);

            // Bắt buộc thoát app nếu là Force Update và user bấm X đóng cửa sổ
            this.FormClosing += (s, e) =>
            {
                if (_updateInfo.IsForceUpdate && !_allowExitWithoutAppKill)
                {
                    Application.Exit();
                }
            };

            this.Shown += (s, e) =>
            {
                txtNotes.DeselectAll();
                btnUpdate.Focus();
            };
        }

        /// <summary>
        /// Tự động tải file cài đặt về %TEMP% và khởi chạy nâng cấp
        /// </summary>
        private async Task StartDownloadAndInstallAsync(Button btnUpdate, Button? btnCancel, Button? btnExit, ProgressBar prg, Label lblStatus)
        {
            btnUpdate.Visible = false;
            if (btnCancel != null) btnCancel.Visible = false;
            if (btnExit != null) btnExit.Visible = false;

            lblStatus.Visible = true;
            prg.Visible = true;
            lblStatus.Text = "Đang kết nối tải bản cập nhật...";
            prg.Value = 5;

            string targetUrl = _updateInfo.DownloadUrl;

            // Tự động suy luận link direct download .exe nếu URL đang là release page
            if (targetUrl.Contains("github.com") && !targetUrl.EndsWith(".exe", StringComparison.OrdinalIgnoreCase))
            {
                string baseRelease = targetUrl.TrimEnd('/');
                if (baseRelease.EndsWith("/latest", StringComparison.OrdinalIgnoreCase))
                {
                    baseRelease = baseRelease.Substring(0, baseRelease.Length - "/latest".Length);
                }
                targetUrl = $"{baseRelease}/download/v{_updateInfo.LatestVersion}/TaxSolution_Setup_v{_updateInfo.LatestVersion}.exe";
            }

            // Nếu targetUrl là đường dẫn file cục bộ nhưng không tồn tại trên máy hiện tại -> chuyển sang link online
            if (!targetUrl.StartsWith("http://", StringComparison.OrdinalIgnoreCase) &&
                !targetUrl.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
            {
                if (!File.Exists(targetUrl))
                {
                    targetUrl = $"https://github.com/HTrongThang/tax-distribution/releases/download/v{_updateInfo.LatestVersion}/TaxSolution_Setup_v{_updateInfo.LatestVersion}.exe";
                }
            }

            string tempFile = Path.Combine(Path.GetTempPath(), $"TaxSolution_Setup_v{_updateInfo.LatestVersion}.exe");

            try
            {
                // Hỗ trợ kiểm thử đường dẫn file cục bộ (nếu có)
                if (File.Exists(targetUrl) || (targetUrl.StartsWith("file://") && File.Exists(new Uri(targetUrl).LocalPath)))
                {
                    string localSource = File.Exists(targetUrl) ? targetUrl : new Uri(targetUrl).LocalPath;
                    lblStatus.Text = "Đang tải bản cập nhật: 100%...";
                    prg.Value = 70;
                    await Task.Delay(500);
                    File.Copy(localSource, tempFile, true);
                    prg.Value = 100;
                    lblStatus.Text = "Tải hoàn tất! Đang khởi động bộ cài đặt...";
                    await Task.Delay(800);

                    _allowExitWithoutAppKill = true;
                    Process.Start(new ProcessStartInfo
                    {
                        FileName = tempFile,
                        Arguments = "/SILENT /SP- /CLOSEAPPLICATIONS",
                        UseShellExecute = true
                    });
                    Application.Exit();
                    return;
                }

                using var client = new HttpClient();
                client.Timeout = TimeSpan.FromMinutes(10);
                client.DefaultRequestHeaders.Add("User-Agent", "TaxSolution-AutoUpdater");

                using var response = await client.GetAsync(targetUrl, HttpCompletionOption.ResponseHeadersRead);

                if (!response.IsSuccessStatusCode)
                {
                    throw new Exception($"Không thể tải trực tiếp file exe (Mã lỗi HTTP {(int)response.StatusCode}).");
                }

                var totalBytes = response.Content.Headers.ContentLength ?? -1L;
                using var stream = await response.Content.ReadAsStreamAsync();
                using var fileStream = new FileStream(tempFile, FileMode.Create, FileAccess.Write, FileShare.None);

                var buffer = new byte[16384]; // 16KB buffer
                long totalRead = 0;
                int bytesRead;

                while ((bytesRead = await stream.ReadAsync(buffer, 0, buffer.Length)) > 0)
                {
                    await fileStream.WriteAsync(buffer, 0, bytesRead);
                    totalRead += bytesRead;

                    if (totalBytes > 0)
                    {
                        int progress = (int)((totalRead * 100) / totalBytes);
                        prg.Value = Math.Min(100, Math.Max(0, progress));
                        double readMB = Math.Round(totalRead / (1024.0 * 1024.0), 1);
                        double totalMB = Math.Round(totalBytes / (1024.0 * 1024.0), 1);
                        lblStatus.Text = $"Đang tải bản cập nhật: {progress}% ({readMB} MB / {totalMB} MB)...";
                    }
                    else
                    {
                        double readMB = Math.Round(totalRead / (1024.0 * 1024.0), 1);
                        lblStatus.Text = $"Đang tải bản cập nhật: {readMB} MB...";
                    }
                }

                fileStream.Close();

                lblStatus.Text = "Tải hoàn tất! Đang khởi động bộ cài đặt...";
                prg.Value = 100;
                await Task.Delay(800);

                // Khởi chạy bộ cài đặt ở chế độ TỰ ĐỘNG CHẠY NGẦM (Silent Install)
                // Khách không phải bấm Next hay chọn quyền gì nữa, app tự đè và mở lại
                _allowExitWithoutAppKill = true;
                Process.Start(new ProcessStartInfo
                {
                    FileName = tempFile,
                    Arguments = "/SILENT /SP- /CLOSEAPPLICATIONS",
                    UseShellExecute = true
                });

                // Thoát ứng dụng hiện tại để bộ cài đặt tiến hành nâng cấp
                Application.Exit();
            }
            catch (Exception ex)
            {
                // Fallback nếu không có direct link (ví dụ chưa upload file release lên GitHub)
                lblStatus.Text = "Mở trình duyệt để tải bản cập nhật...";
                await Task.Delay(500);

                UpdateService.OpenDownloadPage(_updateInfo.DownloadUrl);

                if (_updateInfo.IsForceUpdate)
                {
                    _allowExitWithoutAppKill = true;
                    MessageBox.Show(
                        $"Đang mở trang tải về trên trình duyệt do: {ex.Message}\nỨng dụng sẽ đóng để bạn tiến hành cài đặt.",
                        "Thông báo cập nhật",
                        MessageBoxButtons.OK,
                        MessageBoxIcon.Information
                    );
                    Application.Exit();
                }
                else
                {
                    btnUpdate.Visible = true;
                    if (btnCancel != null) btnCancel.Visible = true;
                    lblStatus.Visible = false;
                    prg.Visible = false;
                }
            }
        }
    }
}
