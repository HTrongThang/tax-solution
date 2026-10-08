using System;
using System.Drawing;
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
            this.Text = _updateInfo.IsForceUpdate ? "Cập nhật ứng dụng bắt buộc" : "Thông báo cập nhật ứng dụng";
            this.Size = new Size(580, 460);
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
                Size = new Size(525, 265),
                BackColor = Color.Transparent
            };

            var lblNotice = new Label
            {
                Location = new Point(0, 5),
                Size = new Size(525, 45),
                Font = new Font("Segoe UI", 9.5f, FontStyle.Regular),
                ForeColor = _updateInfo.IsForceUpdate ? Color.FromArgb(185, 28, 28) : Color.FromArgb(71, 85, 105),
                Text = _updateInfo.IsForceUpdate
                    ? "Phiên bản bạn đang sử dụng đã lỗi thời hoặc cần được nâng cấp theo chuẩn dữ liệu mới. Vui lòng cập nhật ngay để tiếp tục sử dụng."
                    : "Đã có bản cập nhật mới với nhiều cải tiến và tối ưu hóa hệ thống. Bạn có muốn nâng cấp ngay không?"
            };
            pnlBody.Controls.Add(lblNotice);

            var lblNotesTitle = new Label
            {
                Text = "Điểm mới trong bản cập nhật này:",
                Font = new Font("Segoe UI", 9.5f, FontStyle.Bold),
                ForeColor = Color.FromArgb(30, 41, 59),
                Location = new Point(0, 55),
                AutoSize = true
            };
            pnlBody.Controls.Add(lblNotesTitle);

            var txtNotes = new TextBox
            {
                Location = new Point(0, 80),
                Size = new Size(525, 175),
                Multiline = true,
                ReadOnly = true,
                ScrollBars = ScrollBars.Vertical,
                BackColor = Color.FromArgb(248, 250, 252),
                ForeColor = Color.FromArgb(51, 65, 85),
                BorderStyle = BorderStyle.FixedSingle,
                Font = new Font("Segoe UI", 9.5f, FontStyle.Regular),
                Text = string.IsNullOrWhiteSpace(_updateInfo.ReleaseNotes) 
                    ? "- Nâng cấp hiệu năng và sửa lỗi hệ thống." 
                    : _updateInfo.ReleaseNotes.Replace("\n", Environment.NewLine)
            };
            pnlBody.Controls.Add(txtNotes);
            this.Controls.Add(pnlBody);

            // 3. Footer Action Buttons
            var pnlFooter = new Panel
            {
                Dock = DockStyle.Bottom,
                Height = 65,
                BackColor = Color.FromArgb(248, 250, 252)
            };

            // Nút Tải cập nhật (chính)
            var btnUpdate = new Button
            {
                Text = "📥 Tải bản cập nhật",
                Size = new Size(180, 42),
                Location = new Point(_updateInfo.IsForceUpdate ? 180 : 345, 12),
                BackColor = Color.FromArgb(16, 185, 129), // Emerald
                ForeColor = Color.White,
                FlatStyle = FlatStyle.Flat,
                Font = new Font("Segoe UI", 10f, FontStyle.Bold),
                Cursor = Cursors.Hand
            };
            btnUpdate.FlatAppearance.BorderSize = 0;
            btnUpdate.Click += (s, e) =>
            {
                UpdateService.OpenDownloadPage(_updateInfo.DownloadUrl);
                if (_updateInfo.IsForceUpdate)
                {
                    _allowExitWithoutAppKill = true;
                    MessageBox.Show("Trình duyệt đang tải bản cài đặt mới.\nỨng dụng sẽ đóng để bạn tiến hành cài đặt.",
                        "Thông báo", MessageBoxButtons.OK, MessageBoxIcon.Information);
                    Application.Exit();
                }
                else
                {
                    _allowExitWithoutAppKill = true;
                    this.Close();
                }
            };
            pnlFooter.Controls.Add(btnUpdate);

            // Nút Đóng / Để sau
            if (!_updateInfo.IsForceUpdate)
            {
                var btnCancel = new Button
                {
                    Text = "Để sau",
                    Size = new Size(110, 42),
                    Location = new Point(220, 12),
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
                // Nếu ép buộc cập nhật: hiển thị nút Thoát
                var btnExit = new Button
                {
                    Text = "Thoát",
                    Size = new Size(110, 42),
                    Location = new Point(370, 12),
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

            this.Controls.Add(pnlFooter);

            // Bắt buộc thoát app nếu là Force Update và user bấm X đóng cửa sổ
            this.FormClosing += (s, e) =>
            {
                if (_updateInfo.IsForceUpdate && !_allowExitWithoutAppKill)
                {
                    Application.Exit();
                }
            };
        }
    }
}
