/**
 * ============================================================
 * InvoiceRamManager: QUẢN LÝ BỘ NHỚ RAM HÓA ĐƠN (FRONTEND)
 * Thiết kế theo mô hình 1 file duy nhất chứa đầy đủ: LƯU, LẤY, XÓA
 * Gọi dùng ở bất kỳ file nào:
 *   - InvoiceRamManager.set(invoices, mst) : Lưu vào RAM
 *   - InvoiceRamManager.get()              : Lấy từ RAM ra dùng (xuất Excel, vẽ bảng)
 *   - InvoiceRamManager.clear(reason)      : Xóa sạch RAM
 *   - InvoiceRamManager.count()            : Đếm số lượng hóa đơn
 *   - InvoiceRamManager.hasData()          : Kiểm tra có dữ liệu không
 * ============================================================
 */
(function (window) {
    'use strict';

    const InvoiceRamManager = {
        _invoices: [],
        _currentMst: null,
        _lastUpdated: null,

        /**
         * 1. LƯU (SET): Nạp danh sách hóa đơn vào RAM
         * @param {Array} invoices - Danh sách hóa đơn từ Cục thuế trả về
         * @param {string} mst - Mã số thuế công ty đang làm việc
         */
        set: function (invoices, mst) {
            this._invoices = Array.isArray(invoices) ? invoices : [];
            this._currentMst = mst || window._selectedCompanyMst || null;
            this._lastUpdated = new Date();

            // Đồng bộ tương thích ngược với các biến cache cũ
            window._invoiceCache = this._invoices;
            window._currentInvoicesRAM = this._invoices;

            console.log(`📥 [InvoiceRamManager] Đã lưu ${this._invoices.length} hóa đơn vào RAM (MST: ${this._currentMst}).`);

            // Tự động bật nút xuất XML / Excel khi có dữ liệu
            if (typeof window.updateExportXmlButtonState === 'function') {
                window.updateExportXmlButtonState(this._invoices.length > 0);
            }
        },

        /**
         * 2. LẤY (GET): Đọc toàn bộ danh sách hóa đơn từ RAM
         * @returns {Array} Danh sách hóa đơn đang giữ trong RAM
         */
        get: function () {
            return this._invoices || [];
        },

        /**
         * 3. ĐẾM (COUNT): Lấy số lượng hóa đơn hiện có trong RAM
         * @returns {number}
         */
        count: function () {
            return (this._invoices && this._invoices.length) || 0;
        },

        /**
         * 4. KIỂM TRA (HAS DATA): Kiểm tra trong RAM có dữ liệu hay không
         * @returns {boolean}
         */
        hasData: function () {
            return this.count() > 0;
        },

        /**
         * 5. XÓA (CLEAR): Giải phóng hoàn toàn bộ nhớ RAM và dọn sạch giao diện
         * @param {string} reason - Lý do xóa (đổi công ty, tìm kiếm mới, đăng xuất)
         */
        clear: function (reason) {
            console.log("🧹 [InvoiceRamManager] Giải phóng RAM hóa đơn. Lý do:", reason || "Không rõ");

            // Kích hoạt dọn dẹp mảng lập tức cho Garbage Collector của V8
            if (this._invoices && Array.isArray(this._invoices)) {
                this._invoices.length = 0;
            }
            this._invoices = [];
            this._currentMst = null;
            this._lastUpdated = null;

            // Đồng bộ với các biến cũ
            window._invoiceCache = [];
            window._currentInvoicesRAM = [];

            // Làm sạch bảng dữ liệu hiển thị (DOM)
            const tableBody = document.getElementById('crawler-results-body');
            if (tableBody) {
                tableBody.innerHTML = `<tr><td colspan="7" class="py-20 text-center text-slate-300 italic">Chưa có dữ liệu để hiển thị</td></tr>`;
            }

            // Xóa sạch các thanh phân trang
            const pagination = document.getElementById('crawler-pagination');
            if (pagination) pagination.innerHTML = '';
            const paginationInfo = document.getElementById('crawler-pagination-info');
            if (paginationInfo) paginationInfo.innerHTML = '';
            const paginationContainer = document.getElementById('pagination-container');
            if (paginationContainer) paginationContainer.innerHTML = '';

            // Khóa nút xuất file
            if (typeof window.updateExportXmlButtonState === 'function') {
                window.updateExportXmlButtonState(false);
            }

            // Reset các nhãn trạng thái
            const countEl = document.getElementById('cpd-count');
            if (countEl) countEl.innerText = '0';
            const statusEl = document.getElementById('cpd-status');
            if (statusEl) statusEl.textContent = '';
        }
    };

    // Đăng ký toàn cục
    window.InvoiceRamManager = InvoiceRamManager;

    // Các alias viết tắt thuận tiện
    window.setInvoiceRAM = function (invoices, mst) { InvoiceRamManager.set(invoices, mst); };
    window.getInvoiceRAM = function () { return InvoiceRamManager.get(); };
    window.clearInvoiceRAM = function (reason) { InvoiceRamManager.clear(reason); };

})(window);
