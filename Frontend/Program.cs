using System.Diagnostics;
using System.Threading;
using Frontend.Views;

namespace Frontend
{
    internal static class Program
    {
        private static Process? _backendProcess;

        /// <summary>
        ///  The main entry point for the application.
        /// </summary>
        [STAThread]
        static void Main()
        {
            const string appGuid = "Global\\TaxSolution_SingleInstance_Mutex";
            using var mutex = new Mutex(false, appGuid);
            if (!mutex.WaitOne(0, false))
            {
                // Đã có 1 bản đang chạy -> Thoát ngay để không mở trùng app hoặc hiện trùng popup
                return;
            }

            // Khởi động Backend (TaxBot.exe) tự động
            StartBackend();

            // Chờ Backend sẵn sàng (tối đa 10 giây)
            WaitForBackend();

            // Tắt Backend khi app đóng
            Application.ApplicationExit += (s, e) => StopBackend();

            ApplicationConfiguration.Initialize();
            Application.Run(new MainForm());
        }

        private static void StartBackend()
        {
            try
            {
                // Nếu backend đã được bật sẵn ở chế độ Debug/Dev (VS Code, dlv), không khởi động TaxBot.exe nữa để tránh trùng port 18386
                try
                {
                    using var pingClient = new System.Net.Http.HttpClient { Timeout = TimeSpan.FromMilliseconds(600) };
                    var ping = pingClient.GetAsync("http://127.0.0.1:18386/api/tax/status").Result;
                    if (ping.IsSuccessStatusCode)
                    {
                        Debug.WriteLine("✅ Backend đã đang chạy sẵn trên port 18386 (Dev mode).");
                        return;
                    }
                }
                catch { }

                // 1. Quét và tắt sạch tiến trình TaxBot cũ còn sót lại để giải phóng port 18386
                foreach (var p in Process.GetProcessesByName("TaxBot"))
                {
                    try
                    {
                        p.Kill();
                        p.WaitForExit(1000);
                    }
                    catch { }
                }

                string appDir = AppDomain.CurrentDomain.BaseDirectory;
                string backendPath = Path.Combine(appDir, "TaxBot.exe");

                if (!File.Exists(backendPath))
                {
                    // Fallback tìm ở thư mục Backend khi chạy debug
                    string devPath = Path.GetFullPath(Path.Combine(appDir, @"..\..\..\..\Backend\TaxBot.exe"));
                    if (File.Exists(devPath))
                    {
                        backendPath = devPath;
                    }
                    else
                    {
                        Debug.WriteLine($"TaxBot.exe không tìm thấy tại: {backendPath}");
                        return;
                    }
                }

                // Log file để debug khi TaxBot crash
                string logDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "TaxSolution");
                Directory.CreateDirectory(logDir);
                string logFile = Path.Combine(logDir, "backend.log");

                _backendProcess = new Process
                {
                    StartInfo = new ProcessStartInfo
                    {
                        FileName = backendPath,
                        WorkingDirectory = appDir,
                        UseShellExecute = false,
                        CreateNoWindow = true,
                        RedirectStandardOutput = true,
                        RedirectStandardError = true
                    }
                };
                _backendProcess.Start();

                // Ghi log ra file (async để không block)
                Task.Run(() =>
                {
                    try
                    {
                        using var writer = new StreamWriter(logFile, false);
                        writer.WriteLine($"=== TaxBot started at {DateTime.Now} ===");
                        writer.WriteLine($"WorkingDirectory: {appDir}");
                        writer.Flush();

                        while (!_backendProcess.HasExited)
                        {
                            string? line = _backendProcess.StandardOutput.ReadLine();
                            if (line != null) { writer.WriteLine(line); writer.Flush(); }
                        }

                        string errors = _backendProcess.StandardError.ReadToEnd();
                        if (!string.IsNullOrEmpty(errors))
                        {
                            writer.WriteLine("=== ERRORS ===");
                            writer.WriteLine(errors);
                        }
                        writer.WriteLine($"=== TaxBot exited with code {_backendProcess.ExitCode} ===");
                    }
                    catch { }
                });

                Debug.WriteLine("✅ TaxBot.exe đã khởi động");
            }
            catch (Exception ex)
            {
                Debug.WriteLine($"Lỗi khởi động TaxBot: {ex.Message}");
            }
        }

        private static void WaitForBackend()
        {
            using var client = new System.Net.Http.HttpClient();
            client.Timeout = TimeSpan.FromSeconds(2);

            for (int i = 0; i < 20; i++) // 20 x 500ms = 10 giây
            {
                try
                {
                    var response = client.GetAsync("http://127.0.0.1:18386/api/tax/status").Result;
                    if (response.IsSuccessStatusCode)
                    {
                        Debug.WriteLine($"✅ Backend sẵn sàng sau {(i + 1) * 500}ms");
                        return;
                    }
                }
                catch { }
                Thread.Sleep(500);
            }
            Debug.WriteLine("⚠️ Backend chưa sẵn sàng sau 10 giây");
        }

        private static void StopBackend()
        {
            try
            {
                if (_backendProcess != null && !_backendProcess.HasExited)
                {
                    _backendProcess.Kill();
                    _backendProcess.Dispose();
                    Debug.WriteLine("🛑 TaxBot.exe đã dừng");
                }
            }
            catch { }
        }
    }
}