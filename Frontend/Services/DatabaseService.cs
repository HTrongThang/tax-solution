using Frontend.Models;
using Microsoft.Data.Sqlite;
using Newtonsoft.Json;
using System;
using System.Collections.Generic;
using System.IO;
using System.Threading;
using System.Threading.Tasks;

namespace Frontend.Services
{
    public class DatabaseService
    {
        private readonly string _dbPath;
        private readonly string _connectionString;
        private readonly SemaphoreSlim _dbLock = new SemaphoreSlim(1, 1);

        public DatabaseService()
        {
            string appDataDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "TaxSolution");
            Directory.CreateDirectory(appDataDir);
            _dbPath = Path.Combine(appDataDir, "tax_solution.db");
            _connectionString = $"Data Source={_dbPath}";
        }

        /// <summary>
        /// Khởi tạo database, tạo bảng nếu chưa có và tự động migrate dữ liệu từ file tax_accounts.json cũ.
        /// </summary>
        public async Task InitializeAsync()
        {
            await _dbLock.WaitAsync();
            try
            {
                using var conn = new SqliteConnection(_connectionString);
                await conn.OpenAsync();

                // 1. Tạo bảng TaxAccounts và Users
                using (var cmd = new SqliteCommand(SqlQueries.CreateTables, conn))
                {
                    await cmd.ExecuteNonQueryAsync();
                }

                // 2. Tự động di chuyển dữ liệu cũ từ tax_accounts.json nếu có
                await MigrateFromJsonIfNeeded(conn);

                // 3. Tự động di chuyển dữ liệu cũ từ users.json nếu có
                await MigrateUsersFromJsonIfNeeded(conn);
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("DatabaseService.InitializeAsync error: " + ex.Message);
            }
            finally
            {
                _dbLock.Release();
            }
        }

        private async Task MigrateFromJsonIfNeeded(SqliteConnection conn)
        {
            string oldJsonPath = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "TaxSolution", "tax_accounts.json");
            if (!File.Exists(oldJsonPath)) return;

            try
            {
                string jsonContent = await File.ReadAllTextAsync(oldJsonPath);
                var oldList = JsonConvert.DeserializeObject<List<CompanyModel>>(jsonContent);

                if (oldList != null && oldList.Count > 0)
                {
                    using var transaction = conn.BeginTransaction();
                    foreach (var item in oldList)
                    {
                        if (string.IsNullOrWhiteSpace(item.Mst)) continue;

                        using var cmd = new SqliteCommand(SqlQueries.UpsertTaxAccount, conn, transaction);
                        cmd.Parameters.AddWithValue("@Mst", item.Mst.Trim());
                        cmd.Parameters.AddWithValue("@CompanyName", item.CompanyName ?? string.Empty);
                        // Mã hóa mật khẩu bằng DPAPI trước khi lưu
                        cmd.Parameters.AddWithValue("@PasswordEncrypted", SecurityHelper.ProtectPassword(item.Password ?? string.Empty));
                        await cmd.ExecuteNonQueryAsync();
                    }

                    await transaction.CommitAsync();
                    System.Diagnostics.Debug.WriteLine($">>> SQLite: Đã migrate {oldList.Count} công ty từ file JSON cũ.");
                }

                // Đổi tên file cũ thành .bak để lưu trữ an toàn mà không xóa mất của người dùng
                string backupPath = oldJsonPath + ".bak";
                if (File.Exists(backupPath)) File.Delete(backupPath);
                File.Move(oldJsonPath, backupPath);
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("MigrateFromJsonIfNeeded error: " + ex.Message);
            }
        }

        private async Task MigrateUsersFromJsonIfNeeded(SqliteConnection conn)
        {
            string oldJsonPath = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "TaxSolution", "users.json");
            if (!File.Exists(oldJsonPath)) return;

            try
            {
                string jsonContent = await File.ReadAllTextAsync(oldJsonPath);
                using var doc = System.Text.Json.JsonDocument.Parse(jsonContent);

                if (doc.RootElement.ValueKind == System.Text.Json.JsonValueKind.Array)
                {
                    using var transaction = conn.BeginTransaction();
                    int count = 0;
                    foreach (var item in doc.RootElement.EnumerateArray())
                    {
                        if (!item.TryGetProperty("username", out var uEl) || !item.TryGetProperty("password", out var pEl))
                            continue;

                        string u = uEl.GetString()?.Trim() ?? string.Empty;
                        string p = pEl.GetString()?.Trim() ?? string.Empty;
                        string role = item.TryGetProperty("role", out var rEl) ? (rEl.GetString() ?? "Staff") : "Staff";
                        string createdAt = item.TryGetProperty("created_at", out var cEl) ? (cEl.GetString() ?? DateTime.Now.ToString("dd/MM/yyyy HH:mm")) : DateTime.Now.ToString("dd/MM/yyyy HH:mm");

                        if (string.IsNullOrEmpty(u) || string.IsNullOrEmpty(p)) continue;

                        using var cmd = new SqliteCommand(SqlQueries.InsertUserMigration, conn, transaction);
                        cmd.Parameters.AddWithValue("@Username", u);
                        cmd.Parameters.AddWithValue("@PasswordHash", p);
                        cmd.Parameters.AddWithValue("@Role", role);
                        cmd.Parameters.AddWithValue("@CreatedAt", createdAt);
                        await cmd.ExecuteNonQueryAsync();
                        count++;
                    }

                    await transaction.CommitAsync();
                    System.Diagnostics.Debug.WriteLine($">>> SQLite: Đã migrate {count} nhân viên từ file users.json cũ.");
                }

                // Đổi tên file cũ thành .bak để lưu trữ an toàn
                string backupPath = oldJsonPath + ".bak";
                if (File.Exists(backupPath)) File.Delete(backupPath);
                File.Move(oldJsonPath, backupPath);
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("MigrateUsersFromJsonIfNeeded error: " + ex.Message);
            }
        }

        /// <summary>
        /// Lấy toàn bộ danh sách công ty từ SQLite (Mật khẩu được tự động giải mã vào CompanyModel.Password).
        /// </summary>
        public async Task<List<CompanyModel>> GetAllAccountsAsync()
        {
            await _dbLock.WaitAsync();
            var result = new List<CompanyModel>();
            try
            {
                using var conn = new SqliteConnection(_connectionString);
                await conn.OpenAsync();

                using var cmd = new SqliteCommand(SqlQueries.SelectAllTaxAccounts, conn);
                using var reader = await cmd.ExecuteReaderAsync();

                while (await reader.ReadAsync())
                {
                    string mst = reader.GetString(0);
                    string companyName = reader.IsDBNull(1) ? string.Empty : reader.GetString(1);
                    string encPass = reader.IsDBNull(2) ? string.Empty : reader.GetString(2);

                    // Giải mã mật khẩu bằng DPAPI để C# và Go backend có thể sử dụng
                    string plainPass = SecurityHelper.UnprotectPassword(encPass);

                    result.Add(new CompanyModel
                    {
                        Mst = mst,
                        CompanyName = companyName,
                        Password = plainPass
                    });
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("DatabaseService.GetAllAccountsAsync error: " + ex.Message);
            }
            finally
            {
                _dbLock.Release();
            }

            return result;
        }

        /// <summary>
        /// Thêm mới hoặc cập nhật công ty (Mật khẩu được tự động mã hóa trước khi lưu).
        /// </summary>
        public async Task UpsertAccountAsync(CompanyModel company)
        {
            if (company == null || string.IsNullOrWhiteSpace(company.Mst)) return;

            await _dbLock.WaitAsync();
            try
            {
                using var conn = new SqliteConnection(_connectionString);
                await conn.OpenAsync();

                using var cmd = new SqliteCommand(SqlQueries.UpsertTaxAccount, conn);
                cmd.Parameters.AddWithValue("@Mst", company.Mst.Trim());
                cmd.Parameters.AddWithValue("@CompanyName", company.CompanyName ?? string.Empty);
                // Mã hóa DPAPI
                cmd.Parameters.AddWithValue("@PasswordEncrypted", SecurityHelper.ProtectPassword(company.Password ?? string.Empty));

                await cmd.ExecuteNonQueryAsync();
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("DatabaseService.UpsertAccountAsync error: " + ex.Message);
                throw;
            }
            finally
            {
                _dbLock.Release();
            }
        }

        /// <summary>
        /// Xóa một tài khoản công ty theo Mã số thuế.
        /// </summary>
        public async Task DeleteAccountAsync(string mst)
        {
            if (string.IsNullOrWhiteSpace(mst)) return;

            await _dbLock.WaitAsync();
            try
            {
                using var conn = new SqliteConnection(_connectionString);
                await conn.OpenAsync();

                using var cmd = new SqliteCommand(SqlQueries.DeleteTaxAccountByMst, conn);
                cmd.Parameters.AddWithValue("@Mst", mst.Trim());

                await cmd.ExecuteNonQueryAsync();
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("DatabaseService.DeleteAccountAsync error: " + ex.Message);
                throw;
            }
            finally
            {
                _dbLock.Release();
            }
        }

        /// <summary>
        /// Xác thực thông tin đăng nhập của nhân viên từ SQLite.
        /// </summary>
        public async Task<bool> ValidateUserAsync(string username, string passwordHash)
        {
            if (string.IsNullOrWhiteSpace(username) || string.IsNullOrWhiteSpace(passwordHash)) return false;

            await _dbLock.WaitAsync();
            try
            {
                using var conn = new SqliteConnection(_connectionString);
                await conn.OpenAsync();

                using var cmd = new SqliteCommand(SqlQueries.ValidateUserLogin, conn);
                cmd.Parameters.AddWithValue("@Username", username.Trim());
                cmd.Parameters.AddWithValue("@PasswordHash", passwordHash.Trim());

                long count = (long)(await cmd.ExecuteScalarAsync() ?? 0L);
                return count > 0;
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("DatabaseService.ValidateUserAsync error: " + ex.Message);
                return false;
            }
            finally
            {
                _dbLock.Release();
            }
        }

        /// <summary>
        /// Kiểm tra tên đăng nhập đã tồn tại trong SQLite chưa (không phân biệt hoa thường).
        /// </summary>
        public async Task<bool> UserExistsAsync(string username)
        {
            if (string.IsNullOrWhiteSpace(username)) return false;

            await _dbLock.WaitAsync();
            try
            {
                using var conn = new SqliteConnection(_connectionString);
                await conn.OpenAsync();

                using var cmd = new SqliteCommand(SqlQueries.CheckUserExists, conn);
                cmd.Parameters.AddWithValue("@Username", username.Trim());

                long count = (long)(await cmd.ExecuteScalarAsync() ?? 0L);
                return count > 0;
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("DatabaseService.UserExistsAsync error: " + ex.Message);
                return false;
            }
            finally
            {
                _dbLock.Release();
            }
        }

        /// <summary>
        /// Thêm nhân viên mới vào bảng Users trong SQLite.
        /// </summary>
        public async Task AddUserAsync(string username, string passwordHash, string role = "Staff")
        {
            if (string.IsNullOrWhiteSpace(username) || string.IsNullOrWhiteSpace(passwordHash)) return;

            await _dbLock.WaitAsync();
            try
            {
                using var conn = new SqliteConnection(_connectionString);
                await conn.OpenAsync();

                using var cmd = new SqliteCommand(SqlQueries.InsertUser, conn);
                cmd.Parameters.AddWithValue("@Username", username.Trim());
                cmd.Parameters.AddWithValue("@PasswordHash", passwordHash.Trim());
                cmd.Parameters.AddWithValue("@Role", role);

                await cmd.ExecuteNonQueryAsync();
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine("DatabaseService.AddUserAsync error: " + ex.Message);
                throw;
            }
            finally
            {
                _dbLock.Release();
            }
        }
    }
}
