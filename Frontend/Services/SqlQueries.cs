namespace Frontend.Services
{
    /// <summary>
    /// File tập trung toàn bộ các câu lệnh SQL của hệ thống TaxSolution.
    /// Giúp quản trị, bảo trì hoặc tùy biến câu truy vấn SQL tại một nơi duy nhất.
    /// </summary>
    public static class SqlQueries
    {
        // =========================================================================
        // 1. KHỞI TẠO BẢNG DỮ LIỆU (DATABASE SCHEMA)
        // =========================================================================
        public const string CreateTables = @"
            CREATE TABLE IF NOT EXISTS TaxAccounts (
                Id INTEGER PRIMARY KEY AUTOINCREMENT,
                Mst TEXT UNIQUE NOT NULL,
                CompanyName TEXT,
                PasswordEncrypted TEXT NOT NULL,
                CreatedAt TEXT DEFAULT (datetime('now', 'localtime')),
                UpdatedAt TEXT DEFAULT (datetime('now', 'localtime'))
            );

            CREATE TABLE IF NOT EXISTS Users (
                Id INTEGER PRIMARY KEY AUTOINCREMENT,
                Username TEXT UNIQUE NOT NULL,
                PasswordHash TEXT NOT NULL,
                Role TEXT NOT NULL DEFAULT 'Staff',
                CreatedAt TEXT DEFAULT (datetime('now', 'localtime'))
            );";

        // =========================================================================
        // 2. BẢNG DOANH NGHIỆP / TÀI KHOẢN THUẾ (TaxAccounts)
        // =========================================================================

        /// <summary>
        /// Lấy toàn bộ danh sách tài khoản doanh nghiệp.
        /// </summary>
        public const string SelectAllTaxAccounts = @"
            SELECT Mst, CompanyName, PasswordEncrypted, Address, TaxDepartment, Status, OrgType, CreatedAt 
            FROM TaxAccounts 
            ORDER BY Id DESC;";

        /// <summary>
        /// Các cột thông tin doanh nghiệp bổ sung (thêm bằng ALTER TABLE cho DB cũ).
        /// </summary>
        public static readonly string[] AddTaxAccountInfoColumns = new[]
        {
            "ALTER TABLE TaxAccounts ADD COLUMN Address TEXT;",
            "ALTER TABLE TaxAccounts ADD COLUMN TaxDepartment TEXT;",
            "ALTER TABLE TaxAccounts ADD COLUMN Status TEXT;",
            "ALTER TABLE TaxAccounts ADD COLUMN OrgType TEXT;"
        };

        /// <summary>
        /// Thêm mới hoặc cập nhật công ty (nếu trùng MST thì update thông tin mới).
        /// </summary>
        public const string UpsertTaxAccount = @"
            INSERT INTO TaxAccounts (Mst, CompanyName, PasswordEncrypted, Address, TaxDepartment, Status, OrgType, UpdatedAt)
            VALUES (@Mst, @CompanyName, @PasswordEncrypted, @Address, @TaxDepartment, @Status, @OrgType, datetime('now', 'localtime'))
            ON CONFLICT(Mst) DO UPDATE SET
                CompanyName = excluded.CompanyName,
                PasswordEncrypted = excluded.PasswordEncrypted,
                Address = COALESCE(excluded.Address, TaxAccounts.Address),
                TaxDepartment = COALESCE(excluded.TaxDepartment, TaxAccounts.TaxDepartment),
                Status = COALESCE(excluded.Status, TaxAccounts.Status),
                OrgType = COALESCE(excluded.OrgType, TaxAccounts.OrgType),
                UpdatedAt = datetime('now', 'localtime');";

        /// <summary>
        /// Xóa tài khoản doanh nghiệp theo Mã số thuế.
        /// </summary>
        public const string DeleteTaxAccountByMst = @"
            DELETE FROM TaxAccounts 
            WHERE Mst = @Mst;";

        // =========================================================================
        // 3. BẢNG NGƯỜI DÙNG / NHÂN VIÊN (Users)
        // =========================================================================

        /// <summary>
        /// Thêm nhân viên mới vào hệ thống.
        /// </summary>
        public const string InsertUser = @"
            INSERT INTO Users (Username, PasswordHash, Role, CreatedAt)
            VALUES (@Username, @PasswordHash, @Role, datetime('now', 'localtime'));";

        /// <summary>
        /// Kiểm tra tên đăng nhập đã tồn tại trong hệ thống chưa (không phân biệt chữ hoa/thường).
        /// </summary>
        public const string CheckUserExists = @"
            SELECT COUNT(1) 
            FROM Users 
            WHERE LOWER(Username) = LOWER(@Username);";

        /// <summary>
        /// Xác thực thông tin đăng nhập của nhân viên (kiểm tra username và hash mật khẩu).
        /// </summary>
        public const string ValidateUserLogin = @"
            SELECT COUNT(1) 
            FROM Users 
            WHERE LOWER(Username) = LOWER(@Username) AND PasswordHash = @PasswordHash;";

        /// <summary>
        /// Câu lệnh phục vụ migrate từ file users.json cũ sang SQLite (nếu trùng thì bỏ qua).
        /// </summary>
        public const string InsertUserMigration = @"
            INSERT INTO Users (Username, PasswordHash, Role, CreatedAt)
            VALUES (@Username, @PasswordHash, @Role, @CreatedAt)
            ON CONFLICT(Username) DO NOTHING;";
    }
}
