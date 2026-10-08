// Lưu trạng thái công ty đã chọn trên toàn ứng dụng
window._selectedCompanyMst = window._selectedCompanyMst || null;
window._loggedInMst = window._loggedInMst || null;

// Cập nhật trạng thái nút xuất XML: chỉ cho phép bấm khi có dữ liệu
window.updateExportXmlButtonState = function (hasData) {
    const btn = document.getElementById('btn-export-xml');
    if (btn) {
        btn.disabled = !hasData;
    }
    const btnExcel = document.getElementById('btn-export-excel');
    if (btnExcel) {
        btnExcel.disabled = !hasData;
    }
    const btnDownloadDetail = document.getElementById('btn-download-detail');
    if (btnDownloadDetail && !window._isDownloadingDetail) {
        btnDownloadDetail.disabled = !hasData;
    }
};

// ============================================================
// BỘ NHỚ RAM QUẢN LÝ HÓA ĐƠN (IN-MEMORY INVOICE STORE)
// Chỉ lưu trữ dữ liệu của 1 công ty duy nhất đang làm việc
// ============================================================
window._currentInvoicesRAM = []; // Biến RAM chính thức lưu toàn bộ danh sách hóa đơn
window._invoiceCache = window._currentInvoicesRAM; // Giữ tương thích ngược

// 1. Nạp danh sách hóa đơn vào RAM
window.setInvoiceRAM = function (invoices) {
    window._currentInvoicesRAM = Array.isArray(invoices) ? invoices : [];
    window._invoiceCache = window._currentInvoicesRAM;
    console.log(`📥 [RAM] Đã lưu ${window._currentInvoicesRAM.length} hóa đơn vào RAM.`);
    if (typeof window.updateExportXmlButtonState === 'function') {
        window.updateExportXmlButtonState(window._currentInvoicesRAM.length > 0);
    }
};

// 2. Đọc dữ liệu hóa đơn từ RAM (dùng cho phân trang, lọc, xuất Excel)
window.getInvoiceRAM = function () {
    return window._currentInvoicesRAM || [];
};

// Đảm bảo đối tượng InvoiceRamManager luôn tồn tại trên toàn cục
window.InvoiceRamManager = window.InvoiceRamManager || {
    set: function (invoices, mst) { window.setInvoiceRAM(invoices); },
    get: function () { return window.getInvoiceRAM(); },
    clear: function (reason) { window.clearInvoiceRAM(reason); },
    count: function () { return (window._currentInvoicesRAM && window._currentInvoicesRAM.length) || 0; },
    hasData: function () { return this.count() > 0; }
};

// 3. Giải phóng hoàn toàn bộ nhớ RAM
window.clearInvoiceRAM = function (reason) {
    if (reason) console.log("🧹 [RAM] Giải phóng bộ nhớ hóa đơn:", reason);

    // Giải phóng mảng RAM lập tức cho Garbage Collector thu hồi
    if (window._currentInvoicesRAM && Array.isArray(window._currentInvoicesRAM)) {
        window._currentInvoicesRAM.length = 0;
    }
    window._currentInvoicesRAM = [];
    if (window._invoiceCache && Array.isArray(window._invoiceCache)) {
        window._invoiceCache.length = 0;
    }
    window._invoiceCache = [];

    // Xóa sạch dữ liệu hiển thị trên bảng
    const tableBody = document.getElementById('crawler-results-body');
    if (tableBody) {
        tableBody.innerHTML = `<tr><td colspan="7" class="py-20 text-center text-slate-300 italic">Chưa có dữ liệu để hiển thị</td></tr>`;
    }

    // Xóa các thanh phân trang
    const pagination = document.getElementById('crawler-pagination');
    if (pagination) pagination.innerHTML = '';
    const paginationInfo = document.getElementById('crawler-pagination-info');
    if (paginationInfo) paginationInfo.innerHTML = '';
    const paginationContainer = document.getElementById('pagination-container');
    if (paginationContainer) paginationContainer.innerHTML = '';

    // Vô hiệu hóa nút xuất XML / Excel
    if (typeof window.updateExportXmlButtonState === 'function') {
        window.updateExportXmlButtonState(false);
    }

    // Reset các biến đếm trạng thái
    const countEl = document.getElementById('cpd-count');
    if (countEl) countEl.innerText = '0';
    const statusEl = document.getElementById('cpd-status');
    if (statusEl) statusEl.textContent = '';
};

// Hàm vẽ danh sách công ty vào Dropdown
window.renderCompanyDropdown = function () {
    const selectCompany = document.getElementById('select-company');
    if (!selectCompany) return;

    if (!tempStorage || tempStorage.length === 0) {
        selectCompany.innerHTML = '<option value="">Không có dữ liệu</option>';
        return;
    }

    // MST ưu tiên: đang đăng nhập > MST đã chọn trước đó > MST đầu tiên
    const targetMst = window._loggedInMst || window._selectedCompanyMst;

    // Tạo HTML cho các option từ tempStorage
    const html = tempStorage.map(company => {
        const isSelected = (targetMst && String(company.mst) === String(targetMst)) ? 'selected' : '';
        return `
            <option value="${company.mst}" ${isSelected}>
                ${company.company || company.mst}
            </option>
        `;
    }).join('');

    selectCompany.innerHTML = html;

    // Nếu targetMst có trong danh sách thì chọn đúng MST đó
    if (targetMst && Array.from(selectCompany.options).some(o => o.value === targetMst)) {
        selectCompany.value = targetMst;
    } else if (selectCompany.value) {
        window._selectedCompanyMst = selectCompany.value;
    }

    // Nếu đang trong phiên đăng nhập thuế, khóa dropdown để tránh người dùng chọn nhầm công ty khác
    if (window._isLoggedIn) {
        selectCompany.disabled = true;
        selectCompany.title = "Đang trong phiên làm việc. Vui lòng đăng xuất trước khi chuyển sang công ty khác.";
    } else {
        selectCompany.disabled = false;
        selectCompany.title = "";
    }

    // Bắt sự kiện người dùng tự chuyển công ty trên dropdown để ghi nhớ & giải phóng RAM công ty cũ
    selectCompany.onchange = function () {
        const oldMst = window._selectedCompanyMst;
        const newMst = this.value;
        window._selectedCompanyMst = newMst;
        if (oldMst && oldMst !== newMst) {
            if (typeof window.clearInvoiceRAM === 'function') {
                window.clearInvoiceRAM("Chuyển sang công ty MST: " + newMst);
            }
        }
        console.log("AccOnline: Đã chọn công ty MST: " + this.value);
    };

    console.log("Hệ thống: Đã cập nhật danh sách công ty (MST đang chọn: " + selectCompany.value + ")");
};

// Receive messages from host (C#) via PostWebMessageAsJson
// Centralized listener: also supports per-request callbacks via window._pendingWebviewRequests
window._pendingWebviewRequests = window._pendingWebviewRequests || {};
if (window.chrome?.webview) {
    window.chrome.webview.addEventListener('message', function (e) {
        try {
            const payload = (typeof e.data === 'string') ? JSON.parse(e.data) : e.data;
            if (!payload) return;

            // If there's a pending per-request handler, invoke it and return.
            // NOTE: Do NOT auto-delete the handler here. The handler (creator) sets a timeout and is
            // responsible for clearing it and deleting its entry when it decides the request is complete.
            if (payload.requestId && window._pendingWebviewRequests[payload.requestId]) {
                try {
                    const h = window._pendingWebviewRequests[payload.requestId];
                    // call handler; let it manage completion and cleanup
                    try { h(payload); } catch (ex2) { console.error('pending handler threw', ex2); }
                } catch (ex) {
                    console.error('pending request handler lookup error', ex);
                }
                return;
            }

            // Request-level messages
            if (payload.action === "START_CRAWL_ACK") {
                console.log('START_CRAWL_ACK', payload);
                // If no per-request handler is waiting, give minimal UI feedback
                if (!payload.requestId || !window._pendingWebviewRequests[payload.requestId]) {
                    const statusLabel = document.getElementById('status-text');
                    if (statusLabel) statusLabel.innerText = 'Đã gửi tác vụ, đang chạy...';
                }
                return;
            }

            if (payload.action === "CRAWL_ERROR") {
                const msg = payload.message || payload.Message || 'Lỗi không xác định';
                console.error('CRAWL_ERROR', payload);
                if (payload.requestId && window._pendingWebviewRequests[payload.requestId]) {
                    // let the per-request handler deal with this error (it will clean itself up)
                } else {
                    // general fallback: show alert and restore UI
                    try { window.showAlert?.('Lỗi', msg, 'error'); } catch (e) { console.error(e); }
                    if (window.finalizeSearchUI) window.finalizeSearchUI();
                }
                return;
            }

            // Backwards-compatible SSE progress handling
            if (payload.action === 'sse_progress') {
                if (window.handleSseProgress) window.handleSseProgress(payload.data || payload.Data || payload);
                return;
            }

            // Generic action handlers
            if (payload.action === "CRAWL_STATUS_UPDATE") {
                const statusLabel = document.getElementById('status-text');
                if (statusLabel) statusLabel.innerText = "Trạng thái: " + (payload.status || payload.Status || '');
                return;
            }

            if (payload.action === "CRAWL_FINISHED") {
                try { window.showAlert?.('Đã hoàn thành', 'Đã cào xong hóa đơn!', 'success'); } catch { }
                if (window.finalizeSearchUI) window.finalizeSearchUI();
                return;
            }

            // ========== DOWNLOAD DETAIL EVENTS ==========
            if (payload.action === "DOWNLOAD_DETAIL_ACK") {
                console.log('DOWNLOAD_DETAIL_ACK', payload);
                return;
            }

            if (payload.action === "DOWNLOAD_PROGRESS") {
                window._updateDownloadProgress(payload);
                return;
            }

            if (payload.action === "DOWNLOAD_FINISHED") {
                window._onDownloadFinished(payload);
                return;
            }

            if (payload.action === "DOWNLOAD_STOPPED") {
                window._onDownloadStopped(payload);
                return;
            }

            if (payload.action === "DOWNLOAD_ERROR") {
                window._onDownloadError(payload);
                return;
            }

            // Other non-request messages can be handled here as needed
        } catch (err) {
            console.error('webview message handler error', err);
        }
    });
}

// Centralized SSE progress handler so logic is in one place and reusable
window.handleSseProgress = function (data) {
    if (!data) return;

    // Update status box
    if (window.updateCrawlerStatus) window.updateCrawlerStatus(data);

    // Reset watchdog timer whenever progress arrives (gia hạn thêm 90s)
    if (window._searchWatchdogId) {
        clearTimeout(window._searchWatchdogId);
        window._searchWatchdogId = null;
    }

    // Normalize fields
    const isDone = data && (data.is_done !== undefined ? data.is_done : (data.IsDone !== undefined ? data.IsDone : false));
    if (!isDone) {
        window._searchWatchdogId = setTimeout(() => {
            console.warn('Search watchdog triggered (no progress for 90s) — finalizing UI');
            if (window.finalizeSearchUI) window.finalizeSearchUI();
        }, 90000);
    }

    let invoices = data.invoices || data.Invoices || null;
    if (typeof invoices === 'string') {
        try { invoices = JSON.parse(invoices); } catch { invoices = null; }
    }

    // If server provides total count, render pagination; otherwise default to page size behaviour
    const total = data.total_on_tax || data.TotalOnTax || (Array.isArray(invoices) ? invoices.length : 0);

    if (isDone) {
        if (Array.isArray(invoices) && invoices.length) {
            // Nạp toàn bộ dữ liệu vào RAM
            if (typeof window.setInvoiceRAM === 'function') {
                window.setInvoiceRAM(invoices);
            }
            // small delay to allow status UI to update first
            setTimeout(() => {
                try { renderInvoices(invoices); }
                catch (ex) { console.error('renderInvoices error', ex); }
                // handle pagination (client-side if invoices present)
                handlePagination({ invoices, totalItems: total, currentPage: 1, pageSize: 20 });
                if (typeof window.updateExportXmlButtonState === 'function') {
                    window.updateExportXmlButtonState(true);
                }
                if (window.finalizeSearchUI) window.finalizeSearchUI();
            }, 50);
        } else {
            // Không có hóa đơn → xóa sạch bảng cũ và hiện thông báo trống
            renderInvoices([]);
            handlePagination({ invoices: null, totalItems: 0, currentPage: 1, pageSize: 20 });
            if (typeof window.updateExportXmlButtonState === 'function') {
                window.updateExportXmlButtonState(false);
            }
            if (window.finalizeSearchUI) window.finalizeSearchUI();
        }
    } else {
        // intermediate update: optionally render partial invoices
        if (Array.isArray(invoices) && invoices.length) {
            try { renderInvoices(invoices); }
            catch (ex) { console.error('renderInvoices error', ex); }
            handlePagination({ invoices, totalItems: total, currentPage: 1, pageSize: 20 });
            if (typeof window.updateExportXmlButtonState === 'function') {
                window.updateExportXmlButtonState(true);
            }
        }
    }
};

// Restore UI when search completes or watchdog triggers
window.finalizeSearchUI = function () {
    try {
        // Enable lại nút search
        const btn = document.getElementById('btn-search');
        if (btn) {
            btn.disabled = false;
            btn.style.opacity = '1';
            btn.style.cursor = 'pointer';
            btn.innerHTML = '<span>🔍</span><span>Bắt đầu tìm kiếm</span>';
        }
        const openSearchBtn = document.getElementById('btn-open-search');
        if (openSearchBtn) {
            openSearchBtn.disabled = false;
            openSearchBtn.style.opacity = '1';
            openSearchBtn.style.cursor = 'pointer';
            openSearchBtn.innerHTML = '<span>🔍</span><span>Bộ lọc tra cứu</span>';
        }

        // Ẩn modal nếu đang hiển và chưa tự ẩn
        const modal = document.getElementById('crawl-progress-modal');
        if (modal && modal.style.display !== 'none') {
            setTimeout(() => {
                modal.style.transition = 'opacity 0.35s ease';
                modal.style.opacity = '0';
                setTimeout(() => {
                    modal.style.display = 'none';
                    modal.style.opacity = '1';
                    modal.style.transition = '';
                    const cpdSub = document.getElementById('cpd-sub');
                    if (cpdSub) cpdSub.innerText = '';
                }, 380);
            }, 2500);
        }

        // Clear watchdog
        if (window._searchWatchdogId) {
            clearTimeout(window._searchWatchdogId);
            window._searchWatchdogId = null;
        }
    } catch (e) { console.error('finalizeSearchUI error', e); }
};

// Unified pagination handler. If invoices array provided, use client-side paging and cache.
// Otherwise render empty pagination using totalItems and request pages from host on click.
window.handlePagination = function ({ invoices = null, totalItems = 0, currentPage = 1, pageSize = 20 }) {
    if (Array.isArray(invoices) && invoices.length) {
        // cache and show first page (or requested page)
        paginateClient(invoices, pageSize);
        showPage(currentPage);
    } else {
        // no client data: just render pagination UI and rely on host for page data
        renderPagination(totalItems, currentPage, pageSize);
    }
};
// Hàm 1: Trung gian - Gọi khi bấm nút
window.handleLoginClick = function (btn) {
    // Hiệu ứng loading để người dùng không bấm lung tung
    const originalText = btn.innerText;
    btn.innerText = "Đang kết nối...";
    btn.classList.add('opacity-50', 'pointer-events-none');

    if (window.chrome && window.chrome.webview) {
        // Gửi lệnh yêu cầu C# đi lấy ảnh
        window.chrome.webview.postMessage({
            action: "REQUEST_OPEN_CAPTCHA"
        });
    }

    // Tự phục hồi nút sau 2.5s nếu Backend Go gặp sự cố
    setTimeout(() => {
        btn.innerText = originalText;
        btn.classList.remove('opacity-50', 'pointer-events-none');
    }, 2500);
};

// Hủy tìm kiếm - gửi lệnh tới C#
window.handleCancelClick = function () {
    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage({ action: 'CANCEL_SEARCH' });
    }
};

// Hủy tiến trình từ Modal tiến độ (Search hoặc Crawl)
window.handleCancelCrawlModal = function (btn) {
    const cancelBtn = btn || document.getElementById('btn-cancel-crawl-modal');
    if (cancelBtn) {
        cancelBtn.disabled = true;
        cancelBtn.innerHTML = `
            <svg class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
            </svg>
            <span>Đang hủy...</span>
        `;
    }

    const cpdStatus = document.getElementById('cpd-status');
    if (cpdStatus) {
        cpdStatus.innerText = 'Đang dừng tiến trình theo yêu cầu...';
        cpdStatus.style.color = '#e11d48';
    }
    const cpdSubtitle = document.getElementById('cpd-subtitle');
    if (cpdSubtitle) {
        cpdSubtitle.innerText = 'Đang hủy...';
    }

    // Gửi lệnh hủy tới Backend C# & Go
    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage({ action: 'CANCEL_SEARCH' });
        window.chrome.webview.postMessage({ action: 'CANCEL_CRAWL' });
    }

    // Xử lý hủy các request đang chờ phản hồi (nếu đang trong luồng handleExportXml)
    if (window._pendingWebviewRequests) {
        Object.keys(window._pendingWebviewRequests).forEach(reqId => {
            try {
                const handler = window._pendingWebviewRequests[reqId];
                if (typeof handler === 'function') {
                    handler({ action: 'CRAWL_ERROR', message: 'Người dùng đã hủy tiến trình', requestId: reqId });
                }
            } catch (e) { }
            delete window._pendingWebviewRequests[reqId];
        });
    }

    // Khôi phục nút search nếu đang bị khóa
    if (typeof window.finalizeSearchUI === 'function') {
        window.finalizeSearchUI();
    }

    // Ẩn modal mượt mà sau 400ms
    setTimeout(() => {
        const modal = document.getElementById('crawl-progress-modal');
        if (modal) {
            modal.style.transition = 'opacity 0.3s ease';
            modal.style.opacity = '0';
            setTimeout(() => {
                modal.style.display = 'none';
                modal.style.opacity = '1';
                modal.style.transition = '';
                if (cancelBtn) {
                    cancelBtn.disabled = false;
                    cancelBtn.innerHTML = `
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M6 18L18 6M6 6l12 12"></path>
                        </svg>
                        <span>Hủy tiến trình</span>
                    `;
                }
            }, 300);
        }
        window.showAlert?.('Đã hủy', 'Đã dừng thao tác thành công!', 'info');
    }, 400);
};
// Hàm 2: Chỉ làm nhiệm vụ hiển thị (C# sẽ gọi hàm này)
window.openCaptcha = function (captchaBase64) {
    const modal = document.getElementById('captcha-modal');
    const img = document.getElementById('img-captcha');
    const input = document.getElementById('input-captcha');
    if (captchaBase64) {
        img.src = captchaBase64;
        modal.classList.remove('hidden');

        if (input) {
            input.disabled = false;
            input.value = '';
            setTimeout(() => input.focus(), 200);
        }
    } else {
        window.showAlert('Lỗi', 'Không thể lấy mã Captcha từ hệ thống Thuế!', 'error');
    }
};

// Hàm đóng Popup
window.closeCaptcha = function () {
    const modal = document.getElementById('captcha-modal');
    if (modal) modal.classList.add('hidden');
    const input = document.getElementById('input-captcha');
    if (input) {
        input.value = '';
        input.disabled = false;
    }
};

// Hàm bấm đổi mã Captcha khác
window.refreshCaptcha = function () {
    const img = document.getElementById('img-captcha');
    if (img) img.style.opacity = '0.5';
    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage({ action: 'REFRESH_CAPTCHA' });
    }
};

window.handleLogout = function () {
    // Báo cho C# biết người dùng muốn Logout
    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage({
            action: "REQUEST_LOGOUT"
        });
    }
};

window.submitCaptcha = function () {
    const inputCaptcha = document.getElementById('input-captcha');
    const selectCompany = document.getElementById('select-company');

    const code = inputCaptcha ? inputCaptcha.value : '';
    const mst = selectCompany ? selectCompany.value : '';

    // 1. Kiểm tra đầu vào
    if (!mst) {
        return window.showAlert('Thông báo', 'Vui lòng chọn một công ty từ danh sách!', 'error');
    }
    if (!code) {
        return window.showAlert('Thông báo', 'Vui lòng nhập mã Captcha!', 'error');
    }


    // 2. Gửi tin nhắn về C#
    if (window.chrome && window.chrome.webview) {
        window._selectedCompanyMst = mst;
        window.chrome.webview.postMessage({
            action: "SUBMIT_CAPTCHA",
            data: {
                taxCode: mst,
                captcha: code
            }
        });
    }

    if (inputCaptcha) inputCaptcha.disabled = true;
};

window.updateLoginStatus = function (isLoggedIn, loggedInMst = null) {
    const btn = document.getElementById('btn-auth');
    const selectCompany = document.getElementById('select-company');

    const baseClass = "ml-2 text-[10px] font-bold px-3 py-1 rounded-md transition-all uppercase active:scale-95 disabled:opacity-50 disabled:cursor-not-allowed ";
    const emeraldStyle = "bg-emerald-50 text-emerald-600 hover:bg-emerald-600 hover:text-white";
    const roseStyle = "bg-rose-50 text-rose-600 hover:bg-rose-600 hover:text-white";

    if (isLoggedIn) {
        window._isLoggedIn = true;
        const activeMst = loggedInMst || window._loggedInMst || window._selectedCompanyMst;
        if (activeMst) {
            window._loggedInMst = activeMst;
            window._selectedCompanyMst = activeMst;
        }

        if (selectCompany) {
            if (window._loggedInMst) {
                selectCompany.value = window._loggedInMst;
            }
            selectCompany.disabled = true;
            selectCompany.title = "Đang trong phiên làm việc. Vui lòng đăng xuất trước khi chuyển sang công ty khác.";
        }

        if (btn) {
            btn.disabled = false;
            btn.style.opacity = "";
            btn.style.cursor = "";
            btn.innerText = 'Đăng xuất';
            btn.className = baseClass + roseStyle;
            btn.dataset.state = "logged-in";
        }
    } else {
        window._isLoggedIn = false;
        window._loggedInMst = null;

        if (selectCompany) {
            selectCompany.disabled = false;
            selectCompany.title = "";
        }

        if (btn) {
            btn.disabled = false;
            btn.style.opacity = "";
            btn.style.cursor = "";
            btn.innerText = 'Đăng nhập';
            btn.className = baseClass + emeraldStyle;
            btn.dataset.state = "logged-out";
        }

        // Xóa trắng dữ liệu hóa đơn trong RAM khi đăng xuất
        if (typeof window.clearInvoiceRAM === 'function') {
            window.clearInvoiceRAM("Đăng xuất tài khoản");
        }
    }
};

// Hàm trung gian điều hướng sự kiện
window.handleAuthAction = function (btn) {
    if (btn.dataset.state === "logged-in") {
        return handleLogoutClick(); // Gọi hàm Logout
    }

    btn.disabled = true;
    btn.innerText = "Đang kết nối...";

    handleLoginClick(btn); // Gửi sang C#
};
window.handleLogoutClick = function () {
    if (typeof window.showConfirm !== 'function') {
        if (!confirm("Bạn có chắc chắn muốn đăng xuất tài khoản thuế này không?")) return;
        proceedLogout();
        return;
    }

    window.showConfirm({
        title: "Đăng xuất tài khoản thuế?",
        message: "Phiên làm việc hiện tại sẽ kết thúc. Bạn có chắc chắn muốn đăng xuất khỏi trang thuế không?",
        confirmText: "ĐĂNG XUẤT",
        cancelText: "HỦY",
        type: "logout",
        onConfirm: proceedLogout
    });

    function proceedLogout() {
        // Disable nút + hiển thị loading
        const btn = document.getElementById('btn-auth');
        if (btn) {
            btn.disabled = true;
            btn._prevText = btn.innerText;
            btn.innerText = "⏳ Đang xuất...";
            btn.style.opacity = "0.6";
            btn.style.cursor = "not-allowed";
        }

        // Gửi tin nhắn sang WinForms (C#)
        if (window.chrome && window.chrome.webview) {
            window.chrome.webview.postMessage({
                action: "REQUEST_LOGOUT"
            });
            console.log("Hệ thống: Đã gửi yêu cầu Logout sang C#");
        } else {
            console.error("Lỗi: Không tìm thấy kết nối WebView2!");
            // Reset nút nếu lỗi
            if (btn) {
                btn.disabled = false;
                btn.innerText = btn._prevText || "Đăng xuất";
                btn.style.opacity = "";
                btn.style.cursor = "";
            }
        }
    }
};

// --- ĐIỀU KHIỂN CHIỀU & HÌNH THỨC HÓA ĐƠN ---
window._currentInvoiceType = 'ban_ra'; // 'ban_ra' | 'mua_vao'
window._currentInvoiceCategory = 'dien_tu'; // 'dien_tu' | 'may_tinh_tien'

window.setInvoiceType = function (type) {
    window._currentInvoiceType = type;
    const btnBanRa = document.getElementById('tab-invoice-ban-ra');
    const btnMuaVao = document.getElementById('tab-invoice-mua-vao');
    if (type === 'ban_ra') {
        btnBanRa?.classList.add('bg-white', 'text-emerald-700', 'shadow-xs');
        btnBanRa?.classList.remove('text-slate-500');
        btnMuaVao?.classList.remove('bg-white', 'text-emerald-700', 'shadow-xs');
        btnMuaVao?.classList.add('text-slate-500');
    } else {
        btnMuaVao?.classList.add('bg-white', 'text-emerald-700', 'shadow-xs');
        btnMuaVao?.classList.remove('text-slate-500');
        btnBanRa?.classList.remove('bg-white', 'text-emerald-700', 'shadow-xs');
        btnBanRa?.classList.add('text-slate-500');
    }
    window.updateFilterBadge();
};

window.setInvoiceCategory = function (cat) {
    window._currentInvoiceCategory = cat;
    const btnDienTu = document.getElementById('tab-cat-dien-tu');
    const btnMayTinhTien = document.getElementById('tab-cat-may-tinh-tien');
    if (cat === 'dien_tu') {
        btnDienTu?.classList.add('bg-white', 'text-emerald-700', 'shadow-xs');
        btnDienTu?.classList.remove('text-slate-500');
        btnMayTinhTien?.classList.remove('bg-white', 'text-emerald-700', 'shadow-xs');
        btnMayTinhTien?.classList.add('text-slate-500');
    } else {
        btnMayTinhTien?.classList.add('bg-white', 'text-emerald-700', 'shadow-xs');
        btnMayTinhTien?.classList.remove('text-slate-500');
        btnDienTu?.classList.remove('bg-white', 'text-emerald-700', 'shadow-xs');
        btnDienTu?.classList.add('text-slate-500');
    }
    window.updateFilterBadge();
};

window.updateFilterBadge = function () {
    const dirBadge = document.getElementById('badge-dir-text');
    const catBadge = document.getElementById('badge-cat-text');
    if (dirBadge) {
        dirBadge.innerText = window._currentInvoiceType === 'mua_vao' ? '📥 Mua vào' : '📤 Bán ra';
    }
    if (catBadge) {
        catBadge.innerText = window._currentInvoiceCategory === 'may_tinh_tien' ? '🖩 Máy tính tiền' : '⚡ HĐĐT';
    }
};

// --- ĐIỀU KHIỂN SEARCH FILTER DIALOG MODAL ---

window.openSearchDialog = function () {
    const modal = document.getElementById('search-filter-modal');
    if (!modal) return;

    // Cập nhật tên công ty đang chọn
    const compSelect = document.getElementById('select-company');
    const compNameEl = document.getElementById('search-modal-company-name');
    if (compNameEl && compSelect) {
        compNameEl.innerText = compSelect.options[compSelect.selectedIndex]?.text || compSelect.value || 'Chưa chọn công ty';
    }

    modal.classList.remove('hidden');
};

window.closeSearchDialog = function () {
    const modal = document.getElementById('search-filter-modal');
    if (modal) modal.classList.add('hidden');
};

window.handleSearchFromDialog = function () {
    window.closeSearchDialog();
    window.handleSearchClick();
};

window.updateFilterSummaryBadge = function () {
    const summaryText = document.getElementById('filter-summary-text');
    const fromEl = document.getElementById('date-from');
    const toEl = document.getElementById('date-to');
    const periodEl = document.getElementById('select-period');
    if (!summaryText) return;

    const periodLabel = periodEl?.options[periodEl.selectedIndex]?.text || '';
    const fromVal = fromEl?.value || '';
    const toVal = toEl?.value || '';

    const formatShort = (v) => {
        if (!v) return '';
        const m = v.match(/^(\d{4})-(\d{2})-(\d{2})$/);
        return m ? `${m[3]}/${m[2]}` : v;
    };

    if (periodLabel && fromVal && toVal) {
        summaryText.innerText = `${periodLabel}: ${formatShort(fromVal)} → ${formatShort(toVal)}`;
    } else if (fromVal && toVal) {
        summaryText.innerText = `${formatShort(fromVal)} → ${formatShort(toVal)}`;
    } else {
        summaryText.innerText = 'Chưa chọn kỳ';
    }
};

// --- XỬ LÝ KỲ THEO QUÝ VÀ CHỌN NGÀY ---

window.initPeriodAndDatePicker = function (forceReset = false) {
    const selectPeriod = document.getElementById('select-period');
    const dateFrom = document.getElementById('date-from');
    const dateTo = document.getElementById('date-to');

    if (!selectPeriod || !dateFrom || !dateTo) return;

    // Khôi phục giá trị đã chọn nếu người dùng switch tab
    if (!forceReset && window._savedPeriodState) {
        selectPeriod.innerHTML = window._savedPeriodState.optionsHtml;
        selectPeriod.value = window._savedPeriodState.selectedPeriod;
        dateFrom.value = window._savedPeriodState.dateFrom;
        dateTo.value = window._savedPeriodState.dateTo;
        window.updateFilterSummaryBadge();
        return;
    }

    if (!forceReset && dateFrom.value && dateTo.value && selectPeriod.options.length > 5) {
        window.updateFilterSummaryBadge();
        return;
    }

    const now = new Date();
    const currentYear = now.getFullYear();
    const prevYear = currentYear - 1;
    const currentMonth = now.getMonth(); // 0 - 11

    // Xác định quý hiện tại
    let currentQ = 1;
    if (currentMonth >= 0 && currentMonth <= 2) currentQ = 1;
    else if (currentMonth >= 3 && currentMonth <= 5) currentQ = 2;
    else if (currentMonth >= 6 && currentMonth <= 8) currentQ = 3;
    else currentQ = 4;

    // Thiết lập danh sách options kỳ theo Tháng và Quý
    selectPeriod.innerHTML = `
        <optgroup label="Năm ${currentYear}">
            <option value="this-month">Tháng này</option>
            <option value="last-month">Tháng trước</option>
            <option value="q1-${currentYear}">Quý 1 (${currentYear})</option>
            <option value="q2-${currentYear}">Quý 2 (${currentYear})</option>
            <option value="q3-${currentYear}">Quý 3 (${currentYear})</option>
            <option value="q4-${currentYear}">Quý 4 (${currentYear})</option>
        </optgroup>
        <optgroup label="Năm ${prevYear}">
            <option value="q1-${prevYear}">Quý 1 (${prevYear})</option>
            <option value="q2-${prevYear}">Quý 2 (${prevYear})</option>
            <option value="q3-${prevYear}">Quý 3 (${prevYear})</option>
            <option value="q4-${prevYear}">Quý 4 (${prevYear})</option>
        </optgroup>
        <option value="custom">Tùy chọn...</option>
    `;

    // Chọn Tháng này làm mặc định
    const defaultVal = 'this-month';
    selectPeriod.value = defaultVal;
    window.handlePeriodChange(defaultVal);
};

window.handlePeriodChange = function (val) {
    const fromEl = document.getElementById('date-from');
    const toEl = document.getElementById('date-to');
    if (!fromEl || !toEl) return;

    if (val === 'custom') return;

    const pad = (n) => String(n).padStart(2, '0');
    const now = new Date();

    if (val === 'this-month') {
        const y = now.getFullYear();
        const m = now.getMonth();
        const lastDay = new Date(y, m + 1, 0).getDate();
        fromEl.value = `${y}-${pad(m + 1)}-01`;
        toEl.value = `${y}-${pad(m + 1)}-${pad(lastDay)}`;
    } else if (val === 'last-month') {
        const y = now.getMonth() === 0 ? now.getFullYear() - 1 : now.getFullYear();
        const m = now.getMonth() === 0 ? 11 : now.getMonth() - 1;
        const lastDay = new Date(y, m + 1, 0).getDate();
        fromEl.value = `${y}-${pad(m + 1)}-01`;
        toEl.value = `${y}-${pad(m + 1)}-${pad(lastDay)}`;
    } else if (/^q[1-4]-\d{4}$/.test(val)) {
        const mQ = val.match(/^q([1-4])-(\d{4})$/);
        const q = parseInt(mQ[1], 10);
        const y = mQ[2];
        if (q === 1) {
            fromEl.value = `${y}-01-01`;
            toEl.value = `${y}-03-31`;
        } else if (q === 2) {
            fromEl.value = `${y}-04-01`;
            toEl.value = `${y}-06-30`;
        } else if (q === 3) {
            fromEl.value = `${y}-07-01`;
            toEl.value = `${y}-09-30`;
        } else if (q === 4) {
            fromEl.value = `${y}-10-01`;
            toEl.value = `${y}-12-31`;
        }
    }

    window._savePeriodState();
    if (typeof window.updateFilterSummaryBadge === 'function') {
        window.updateFilterSummaryBadge();
    }
};

window.handleDateInputChange = function () {
    const fromEl = document.getElementById('date-from');
    const toEl = document.getElementById('date-to');
    const periodSelect = document.getElementById('select-period');
    if (!fromEl || !toEl) return;

    const fromVal = fromEl.value;
    const toVal = toEl.value;
    if (!fromVal || !toVal) return;

    if (fromVal > toVal) {
        window.showAlert('Lỗi logic', 'Ngày kết thúc không được nhỏ hơn ngày bắt đầu!', 'error');
        toEl.value = fromVal;
        return;
    }

    // Check if this date range matches any option in select-period
    if (periodSelect) {
        let matched = false;
        for (let opt of periodSelect.options) {
            if (opt.value === 'custom') continue;
            const val = opt.value;
            let expectedFrom = '';
            let expectedTo = '';
            const mQ = val.match(/^q([1-4])-(\d{4})$/);
            if (mQ) {
                const q = parseInt(mQ[1], 10);
                const y = mQ[2];
                if (q === 1) { expectedFrom = `${y}-01-01`; expectedTo = `${y}-03-31`; }
                else if (q === 2) { expectedFrom = `${y}-04-01`; expectedTo = `${y}-06-30`; }
                else if (q === 3) { expectedFrom = `${y}-07-01`; expectedTo = `${y}-09-30`; }
                else if (q === 4) { expectedFrom = `${y}-10-01`; expectedTo = `${y}-12-31`; }
            }
            if (expectedFrom && expectedFrom === fromVal && expectedTo === toVal) {
                periodSelect.value = val;
                matched = true;
                break;
            }
        }
        if (!matched) {
            periodSelect.value = 'custom';
        }
    }

    window._savePeriodState();
    if (typeof window.updateFilterSummaryBadge === 'function') {
        window.updateFilterSummaryBadge();
    }
};

window._savePeriodState = function () {
    const selectPeriod = document.getElementById('select-period');
    const dateFrom = document.getElementById('date-from');
    const dateTo = document.getElementById('date-to');
    if (selectPeriod && dateFrom && dateTo) {
        window._savedPeriodState = {
            optionsHtml: selectPeriod.innerHTML,
            selectedPeriod: selectPeriod.value,
            dateFrom: dateFrom.value,
            dateTo: dateTo.value
        };
    }
};

// --- EXCEL DROPDOWN MENU ---
window.toggleExcelDropdown = function () {
    const menu = document.getElementById('excel-dropdown-menu');
    if (!menu) return;
    menu.classList.toggle('hidden');
};

window.closeExcelDropdown = function () {
    const menu = document.getElementById('excel-dropdown-menu');
    if (menu) menu.classList.add('hidden');
};

document.addEventListener('DOMContentLoaded', function () {
    if (typeof window.initPeriodAndDatePicker === 'function') {
        window.initPeriodAndDatePicker();
    }
    if (typeof window.updateFilterSummaryBadge === 'function') {
        window.updateFilterSummaryBadge();
    }
    const btnPdf = document.getElementById('btn-export-pdf');
    if (btnPdf) btnPdf.addEventListener('click', () => window.handleExportRequest('pdf'));

    // Click bên ngoài dropdown → tự đóng
    document.addEventListener('click', function (e) {
        const wrapper = document.getElementById('excel-dropdown-wrapper');
        if (wrapper && !wrapper.contains(e.target)) {
            window.closeExcelDropdown();
        }
    });
});

// Hàm để C# bốc dữ liệu gửi sang Go (chuẩn hóa dạng DD/MM/YYYY)
window.getFilterParams = function () {
    const fromEl = document.getElementById('date-from');
    const toEl = document.getElementById('date-to');

    const formatToDdMmYyyy = (val) => {
        if (!val) return '';
        val = val.trim();
        const mIso = val.match(/^(\d{4})-(\d{2})-(\d{2})$/);
        if (mIso) return `${mIso[3]}/${mIso[2]}/${mIso[1]}`;
        if (/^\d{1,2}\/\d{1,2}\/\d{4}$/.test(val)) return val;
        return val;
    };

    return {
        fromDate: formatToDdMmYyyy(fromEl ? fromEl.value : ''),
        toDate: formatToDdMmYyyy(toEl ? toEl.value : ''),
        rawFrom: fromEl ? fromEl.value : '',
        rawTo: toEl ? toEl.value : '',
        invoiceType: window._currentInvoiceType || 'ban_ra',
        invoiceCategory: window._currentInvoiceCategory || 'dien_tu'
    };
};
// Hàm chuẩn hóa và bảo đảm tính chính xác của dữ liệu hóa đơn (tự sửa lỗi nếu cột tiền bị gán nhầm trạng thái)
window.sanitizeInvoiceData = function (inv) {
    if (!inv) return;
    const isStatusString = (v) => {
        if (!v || v === '---') return false;
        const s = String(v).trim().toLowerCase();
        return s.includes('hóa đơn') || s.includes('mới') || s.includes('thay thế') || s.includes('điều chỉnh') || s.includes('hủy') || !/\d/.test(s);
    };

    // Nếu tong_thanh_toan bị lưu nhầm trạng thái (ví dụ "Hóa đơn mới")
    if (isStatusString(inv.tong_thanh_toan)) {
        const statusVal = inv.tong_thanh_toan;
        // 1. Phục hồi trạng thái chuẩn nếu trường trạng thái đang thiếu hoặc chứa số
        if (!inv.trang_thai || inv.trang_thai === '---' || /\d{3}/.test(inv.trang_thai)) {
            inv.trang_thai = statusVal;
        }

        // 2. Tìm lại tiền thanh toán thực tế:
        // TH1: tong_phi chứa số tiền (do lỗi gán nhầm cột 10 vào tong_phi trước đây)
        if (inv.tong_phi && !isStatusString(inv.tong_phi) && /\d/.test(inv.tong_phi)) {
            inv.tong_thanh_toan = inv.tong_phi;
            inv.tong_phi = "0";
        } else {
            // TH2: Tính tổng từ chưa thuế + tiền thuế - chiết khấu
            const parseNum = (v) => {
                if (!v) return 0;
                const clean = String(v).replace(/\./g, '').replace(/,/g, '.').replace(/[^\d.-]/g, '');
                return parseFloat(clean) || 0;
            };
            const chuaThue = parseNum(inv.tong_chua_thue);
            const tienThue = parseNum(inv.tong_tien_thue);
            const chietKhau = parseNum(inv.chiet_khau);
            const total = chuaThue + tienThue - chietKhau;
            if (total > 0) {
                inv.tong_thanh_toan = new Intl.NumberFormat('vi-VN').format(total);
            }
        }
    }
};

// Open detail modal (ensure function exists before renderInvoices uses it)
window.openDetailModal = function (inv) {
    if (!inv) return;
    if (typeof window.sanitizeInvoiceData === 'function') {
        window.sanitizeInvoiceData(inv);
    }
    const id = 'invoiceDetailModal';
    const existing = document.getElementById(id);
    if (existing) existing.remove();


    // Giả định hàm rawText đã được định nghĩa ở trên để giữ nguyên bản dữ liệu

    // Hàm giữ nguyên bản dữ liệu cào được
    const raw = (s) => (s && s !== "" && s !== "---") ? String(s).trim() : "---";

    // Xác định đơn vị tiền tệ thực tế của hóa đơn (VND, USD, EUR...)
    const curUnit = (typeof inv.don_vi_tien_te === 'string' && /^[A-Za-z]{2,5}$/.test(inv.don_vi_tien_te.trim()))
        ? inv.don_vi_tien_te.trim().toUpperCase()
        : "VND";

    // Hàm hỗ trợ định dạng hiển thị tiền tệ và đơn vị
    const fmt = (val, unit = curUnit) => {
        const safeUnit = (typeof unit === 'string' && /^[A-Za-z]{2,5}$/.test(unit.trim())) ? unit.trim().toUpperCase() : curUnit;
        if (!val || val === "---") return "0 " + safeUnit;
        return val + " " + safeUnit;
    };

    const data = {
        stt: raw(inv.stt),
        mstBan: raw(inv.mst_nguoi_ban),
        mauSo: raw(inv.ky_hieu_mau_so),
        kyHieu: raw(inv.ky_hieu),
        soHD: raw(inv.so_hoa_don),
        ngayLap: raw(inv.ngay_lap),
        thongTin: raw(inv.thong_tin_hoa_don).replace(/\n/g, '<br>'),
        chuaThue: fmt(inv.tong_chua_thue, inv.don_vi_tien_te),
        tienThue: fmt(inv.tong_tien_thue, inv.don_vi_tien_te),
        chietKhau: fmt(inv.chiet_khau, inv.don_vi_tien_te),
        phi: fmt(inv.tong_phi, inv.don_vi_tien_te),
        tongCong: fmt(inv.tong_thanh_toan, inv.don_vi_tien_te),
        trangThai: raw(inv.trang_thai),
        xuLy: raw(inv.ket_qua_xu_ly),
        ketQua: raw(inv.ket_qua_kiem_tra),
        lienQuan: raw(inv.hoa_don_lien_quan),
        thongTinLienQuan: raw(inv.thong_tin_lien_quan)
    };

    const finalHtml = `
<div id="${id}" class="fixed inset-0 z-[9999] flex items-center justify-center p-4 bg-black/40 backdrop-blur-sm no-print">
    <div class="relative w-full max-w-4xl bg-white border border-black flex flex-col shadow-2xl animate-in zoom-in-95 duration-200 print:shadow-none print:border-none print:m-0">
        
        <!-- Toolbar (Ẩn khi in) -->
        <div class="px-10 py-3 bg-white border-b border-black flex justify-end gap-3 no-print">
            <button onclick="window.print()" class="px-5 py-2 border border-black text-[12px] font-bold uppercase text-black hover:bg-black hover:text-white transition-all">
               In hóa đơn
            </button>
            <button onclick="this.closest('#${id}').remove()" class="px-5 py-2 bg-black text-white text-[12px] font-bold uppercase hover:bg-zinc-800 transition-all">
                Đóng [X]
            </button>
        </div>

        <div class="p-5 overflow-y-auto max-h-[90vh] print:p-0 print:overflow-visible text-black font-sans">
            
            <!-- Header -->
            <div class="text-center mb-5">
                <h1 class="text-4xl font-bold uppercase tracking-widest">Hóa đơn giá trị gia tăng</h1>
                <p class="text-xs italic mt-3">(Bản thể hiện của hóa đơn điện tử)</p>
                <div class="flex justify-center mt-6">
                    <div class="h-[3px] w-24 bg-black"></div>
                </div>
            </div>

            <!-- Info Grid: Đã chỉnh lại để thẳng hàng tuyệt đối và chữ to hơn -->
            <div class="grid grid-cols-2 gap-x-24 gap-y-6 mb-6 text-base">
                <div class="space-y-4">
                    <div class="flex items-end border-b border-black pb-1">
                        <span class="text-[11px] font-bold uppercase w-32 shrink-0">Ngày lập:</span>
                        <span class="font-medium">${data.ngayLap}</span>
                    </div>
                    <div class="flex items-end border-b border-black pb-1">
                        <span class="text-[11px] font-bold uppercase w-32 shrink-0">MST Người bán:</span>
                        <span class="font-mono font-bold">${data.mstBan}</span>
                    </div>
                </div>
                <div class="space-y-4">
                    <div class="flex items-end border-b border-black pb-1">
                        <span class="text-[11px] font-bold uppercase w-32 shrink-0">Số hóa đơn:</span>
                        <span class="text-2xl font-mono font-bold leading-none tracking-tighter">${data.soHD}</span>
                    </div>
                    <div class="flex items-end border-b border-black pb-1">
                        <span class="text-[11px] font-bold uppercase w-32 shrink-0">Ký hiệu:</span>
                        <span class="font-bold">${data.mauSo}${data.kyHieu}</span>
                    </div>
                </div>
            </div>

            <!-- Mục I -->
            <div class="mb-10">
                <h3 class="text-[12px] font-black uppercase tracking-[0.2em] mb-4 border-l-8 border-black pl-4">
                    I. Nội dung cung cấp
                </h3>
                <div class="p-6 border border-black text-base leading-relaxed min-h-[80px]">
                    ${data.thongTin}
                </div>
            </div>

            <!-- Mục II -->
            <div class="mb-6">
                <h3 class="text-[12px] font-black uppercase tracking-[0.2em] mb-4 border-l-8 border-black pl-4">
                    II. Chi tiết giá trị
                </h3>
                <table class="w-full border-collapse text-base">
                    <thead>
                        <tr class="border-y-2 border-black font-bold uppercase text-[11px]">
                            <th class="p-4 text-left">Diễn giải dịch vụ / hàng hóa</th>
                            <th class="p-4 text-right w-56">Thành tiền (${curUnit})</th>
                        </tr>
                    </thead>
                    <tbody class="divide-y divide-zinc-300">
                        <tr>
                            <td class="p-4 italic">Tổng tiền hàng chưa thuế GTGT</td>
                            <td class="p-4 text-right tabular-nums">${data.chuaThue}</td>
                        </tr>
                        <tr>
                            <td class="p-4 italic">Tiền thuế giá trị gia tăng</td>
                            <td class="p-4 text-right tabular-nums">${data.tienThue}</td>
                        </tr>
                        <tr>
                            <td class="p-4 italic">Chiết khấu / Phí khác</td>
                            <td class="p-4 text-right tabular-nums">${data.chietKhau}</td>
                        </tr>
                        <tr class="border-t-2 border-black">
                            <td class="p-6 font-bold uppercase text-[14px]">Tổng cộng thanh toán</td>
                            <td class="p-6 text-right text-1xl font-bold tabular-nums font-mono">
                                ${data.tongCong}
                            </td>
                        </tr>
                    </tbody>
                </table>
            </div>

        </div>
    </div>

    <style>
        @media print {
            .no-print { display: none !important; }
            body { background: white !important; padding: 0 !important; color: black !important; }
            #${id} { position: absolute !important; padding: 0 !important; background: white !important; }
            .print\\:shadow-none { box-shadow: none !important; }
            .print\\:border-none { border: none !important; }
            * { border-color: black !important; }
        }
    </style>
</div>
`;
    document.body.insertAdjacentHTML('beforeend', finalHtml);

};
window.handleSearchClick = function () {
    // Disable nút search ngay để tránh click nhiều lần
    const searchBtn = document.getElementById('btn-search');
    if (searchBtn) {
        if (searchBtn.disabled) return; // Đang search rồi, bỏ qua
        searchBtn.disabled = true;
        searchBtn.style.opacity = '0.6';
        searchBtn.style.cursor = 'not-allowed';
        searchBtn.innerHTML = '⏳ Đang tìm...';
    }
    const openSearchBtn = document.getElementById('btn-open-search');
    if (openSearchBtn) {
        openSearchBtn.disabled = true;
        openSearchBtn.style.opacity = '0.6';
        openSearchBtn.style.cursor = 'not-allowed';
        openSearchBtn.innerHTML = '<span>⏳</span><span>Đang tìm...</span>';
    }

    // Xóa sạch kết quả cũ trong RAM trước khi search mới
    if (typeof window.clearInvoiceRAM === 'function') {
        window.clearInvoiceRAM("Bắt đầu tìm kiếm đợt mới");
    }

    // Reset toàn bộ modal tiến trình về trạng thái ban đầu để không hiển thị kết quả cũ
    const modal = document.getElementById('crawl-progress-modal');
    const cpdCount = document.getElementById('cpd-count');
    const cpdStatus = document.getElementById('cpd-status');
    const cpdSub = document.getElementById('cpd-sub');
    const cpdTitle = document.getElementById('cpd-title');
    const cpdSubtitle = document.getElementById('cpd-subtitle');
    const cpdSpinner = document.getElementById('cpd-spinner-svg');
    const cpdDone = document.getElementById('cpd-icon-done');
    const cpdError = document.getElementById('cpd-icon-error');
    const cpdFolderWrap = document.getElementById('cpd-folder-wrap');
    const cpdCancelWrap = document.getElementById('cpd-cancel-wrap');
    const cancelBtn = document.getElementById('btn-cancel-crawl-modal');

    if (cpdCount) cpdCount.innerText = '0';
    if (cpdSub) cpdSub.innerText = '';
    if (cpdStatus) {
        cpdStatus.innerText = 'Đang chuẩn bị tìm kiếm...';
        cpdStatus.style.color = '#1e293b';
    }
    if (cpdTitle) {
        cpdTitle.innerText = 'Đang tìm kiếm hóa đơn...';
        cpdTitle.style.color = '#0f172a';
    }
    if (cpdSubtitle) cpdSubtitle.innerText = 'Vui lòng không đóng cửa sổ';
    if (cpdSpinner) cpdSpinner.style.display = 'block';
    if (cpdDone) cpdDone.style.display = 'none';
    if (cpdError) cpdError.style.display = 'none';
    if (cpdFolderWrap) cpdFolderWrap.style.display = 'none';
    if (cpdCancelWrap) cpdCancelWrap.style.display = 'flex';
    if (cancelBtn) {
        cancelBtn.disabled = false;
        cancelBtn.innerHTML = `
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
            <span>Hủy tiến trình</span>
        `;
    }
    if (modal) {
        modal.style.transition = '';
        modal.style.opacity = '1';
        modal.style.display = 'flex';
    }

    const paginationInfo = document.getElementById('crawler-pagination-info');
    if (paginationInfo) paginationInfo.innerText = '';

    if (typeof window.updateExportXmlButtonState === 'function') {
        window.updateExportXmlButtonState(false);
    }

    // 1. Lấy dữ liệu từ hàm bạn đã viết
    const params = window.getFilterParams();

    // 2. Kiểm tra nhanh (Double check lần cuối cho an toàn)
    if (!params.fromDate || !params.toDate) {
        if (searchBtn) { searchBtn.disabled = false; searchBtn.style.opacity = '1'; searchBtn.style.cursor = 'pointer'; searchBtn.innerHTML = '<span>🔍</span><span>Bắt đầu tìm kiếm</span>'; }
        if (openSearchBtn) { openSearchBtn.disabled = false; openSearchBtn.style.opacity = '1'; openSearchBtn.style.cursor = 'pointer'; openSearchBtn.innerHTML = '<span>🔍</span><span>Bộ lọc tra cứu</span>'; }
        return window.showAlert('Thông báo', 'Vui lòng chọn đầy đủ khoảng thời gian!', 'error');
    }


    // Normalize to ISO and validate using UTC days
    const toIso = v => {
        if (!v) return '';
        v = v.trim();
        if (/^\d{4}-\d{2}-\d{2}$/.test(v)) return v;
        const m1 = v.match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})$/);
        if (m1) return `${m1[3]}-${m1[2].padStart(2, '0')}-${m1[1].padStart(2, '0')}`;
        const parsed = new Date(v);
        if (!isNaN(parsed.getTime())) return parsed.toISOString().split('T')[0];
        return '';
    };

    const MS_PER_DAY = 24 * 60 * 60 * 1000;
    const startIso = toIso(params.fromDate);
    const endIso = toIso(params.toDate);

    if (!startIso || !endIso) {
        if (window.finalizeSearchUI) window.finalizeSearchUI();
        return window.showAlert('Thông báo', 'Vui lòng chọn đầy đủ khoảng thời gian!', 'warning');
    }

    const start = new Date(startIso);
    const end = new Date(endIso);

    const utcStart = Date.UTC(start.getFullYear(), start.getMonth(), start.getDate());
    const utcEnd = Date.UTC(end.getFullYear(), end.getMonth(), end.getDate());
    const diffDays = Math.floor((utcEnd - utcStart) / MS_PER_DAY);

    if (diffDays < 0) {
        if (window.finalizeSearchUI) window.finalizeSearchUI();
        return window.showAlert('Lỗi logic', 'Ngày kết thúc không được nhỏ hơn ngày bắt đầu!', 'error');
    }
    if (diffDays > 93) {
        if (window.finalizeSearchUI) window.finalizeSearchUI();
        return window.showAlert('Giới hạn tra cứu', 'Hệ thống chỉ hỗ trợ tra cứu tối đa 1 Quý (không quá 93 ngày) để tránh quá tải dữ liệu!', 'warning');
    }

    // 3. Gửi sang C# (WinForms) để báo cho Backend Go bắt đầu Crawl
    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage({
            action: "SEARCH_INVOICE",
            data: {
                fromDate: params.fromDate,
                toDate: params.toDate,
                invoiceType: params.invoiceType,
                invoiceCategory: params.invoiceCategory
            }
        });

        // Có thể thêm hiệu ứng đổi text nút hoặc loading ở đây nếu muốn
        console.log("AccOnline: Đã gửi yêu cầu tra cứu từ " + params.fromDate + " đến " + params.toDate);

        // Watchdog: tự động finalize UI sau 180s nếu hoàn toàn không có phản hồi
        if (window._searchWatchdogId) {
            clearTimeout(window._searchWatchdogId);
            window._searchWatchdogId = null;
        }
        const wd = window._searchWatchdogDuration || 180000;
        window._searchWatchdogId = setTimeout(() => {
            console.warn('Search watchdog triggered — finalizing UI');
            if (window.finalizeSearchUI) window.finalizeSearchUI();
        }, wd);
    } else {
        // Nếu không có WebView2, re-enable lại button
        if (window.finalizeSearchUI) window.finalizeSearchUI();
        window.showAlert('Lỗi', 'Không tìm thấy kết nối ứng dụng (WebView2)!', 'error');
    }
};


// Export / Download handlers for XML and PDF
window.handleExportRequest = function (type) {
    // type: 'xml' | 'pdf'
    const params = window.getFilterParams();
    const selectCompany = document.getElementById('select-company');
    const taxCode = selectCompany ? selectCompany.value : '';

    if (!params.fromDate || !params.toDate) {
        return window.showAlert('Thông báo', 'Vui lòng chọn đầy đủ khoảng thời gian trước khi xuất!', 'warning');
    }

    // Prepare payload
    const payload = {
        type: type,
        fromDate: params.fromDate,
        toDate: params.toDate,
        taxCode: taxCode
    };

    // Prefer host (C#) to perform authenticated download since it has API credentials
    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage({ action: 'REQUEST_DOWNLOAD_INVOICES', data: payload });
        console.log('Download requested via WebView host', payload);
        return;
    }

    // Fallback: if page exposes direct endpoints, try to fetch and trigger download
    try {
        const endpoint = (type === 'xml') ? (window.API_DOWNLOAD_XML || window.apiDownloadXml) : (window.API_DOWNLOAD_PDF || window.apiDownloadPdf);
        if (endpoint) {
            // Expect endpoint to accept query params: fromDate, toDate, taxCode
            const q = new URL(endpoint, window.location.origin);
            q.searchParams.set('fromDate', params.fromDate);
            q.searchParams.set('toDate', params.toDate);
            if (taxCode) q.searchParams.set('taxCode', taxCode);

            fetch(q.toString(), { credentials: 'include' })
                .then(r => {
                    if (!r.ok) throw new Error('Server returned ' + r.status);
                    return r.blob();
                })
                .then(blob => {
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = `invoices_${params.fromDate}_${params.toDate}.${type === 'pdf' ? 'pdf' : 'zip'}`;
                    document.body.appendChild(a);
                    a.click();
                    a.remove();
                    URL.revokeObjectURL(url);
                })
                .catch(err => window.showAlert('Lỗi tải', err.message || String(err), 'error'));
            return;
        }
    } catch (e) {
        console.error('fallback download error', e);
    }

    window.showAlert('Lỗi', 'Không thể thực hiện tải xuống — không tìm thấy kết nối phù hợp.', 'error');
};
function renderInvoices(invoices) {
    const tableBody = document.getElementById('crawler-results-body');
    if (!tableBody) return;
    tableBody.innerHTML = '';

    if (!invoices || invoices.length === 0) {
        tableBody.innerHTML = `<tr><td colspan="7" class="py-20 text-center text-slate-300 italic">Chưa có dữ liệu để hiển thị</td></tr>`;
        if (!window._invoiceCache || window._invoiceCache.length === 0) {
            if (typeof window.updateExportXmlButtonState === 'function') {
                window.updateExportXmlButtonState(false);
            }
        }
        return;
    }

    if (typeof window.updateExportXmlButtonState === 'function') {
        window.updateExportXmlButtonState(true);
    }

    const fragment = document.createDocumentFragment();

    invoices.forEach((inv) => {
        if (typeof window.sanitizeInvoiceData === 'function') {
            window.sanitizeInvoiceData(inv);
        }
        const row = document.createElement('tr');
        row.className = "hover:bg-blue-50/40 transition-all cursor-pointer group";

        row.innerHTML = `
            <!-- STT: Tăng từ text-xs lên text-sm -->
            <td class="py-4 px-4 text-sm text-slate-500 font-mono text-center">${inv.stt}</td>
            
            <td class="py-4 px-4">
                <div class="flex flex-col gap-0.5">
                    <!-- MST: Tăng lên text-base để dễ đọc -->
                    <span class="text-base font-semibold text-slate-800">${inv.mst_nguoi_ban}</span>
                    <!-- Ký hiệu: Tăng từ 10px lên text-xs (12px) -->
                    <span class="text-xs text-slate-500 font-medium">${inv.ky_hieu_mau_so} | ${inv.ky_hieu}</span>
                </div>
            </td>

            <td class="py-4 px-4">
                <div class="flex flex-col gap-0.5">
                    <!-- Số hóa đơn: text-base -->
                    <span class="text-base font-bold text-blue-600">HĐ: ${inv.so_hoa_don}</span>
                    <!-- Ngày lập: text-sm -->
                    <span class="text-sm text-slate-600">${inv.ngay_lap}</span>
                </div>
            </td>

            <td class="py-4 px-4 text-right">
                <div class="flex flex-col gap-0.5">
                    <!-- Tổng tiền: text-base font-bold -->
                    <span class="text-base font-bold text-slate-900">${inv.tong_thanh_toan} <span class="text-xs font-semibold text-slate-500">${inv.don_vi_tien_te || 'VND'}</span></span>
                    <!-- Tiền thuế: text-xs -->
                    <span class="text-xs text-slate-500 font-medium">Thuế: ${inv.tong_tien_thue} ${inv.don_vi_tien_te || 'VND'}</span>
                </div>
            </td>

            <td class="py-4 px-4 text-center">
                <div class="flex flex-col items-center gap-1.5">
                    <!-- Kết quả kiểm tra: Tăng lên text-xs -->
                    <span class="text-xs leading-tight font-bold ${inv.ket_qua_kiem_tra.includes('không') ? 'text-red-600' : 'text-slate-600'}">
                        ${inv.ket_qua_kiem_tra}
                    </span>
                    <!-- Kết quả xử lý: Tăng lên 11px cho rõ -->
                    <span class="text-[11px] text-slate-400 italic font-medium">${inv.ket_qua_xu_ly}</span>
                </div>
            </td>

            <td class="py-4 px-4 text-center">
                <!-- Status Badge: Tăng lên text-xs và padding dày hơn xíu -->
                <span class="inline-block px-3 py-1 rounded text-xs font-black uppercase tracking-wider ${getStatusStyle(inv.trang_thai)}">
                    ${inv.trang_thai}
                </span>
            </td>

            <td class="py-4 px-4 text-right">
                <button class="btn-view text-slate-400 group-hover:text-blue-600 transition-colors" aria-label="Xem chi tiết">
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M9 5l7 7-7 7"></path>
                    </svg>
                </button>
            </td>
        `;

        // Attach click handlers: button stops propagation so only one handler fires
        const viewBtn = row.querySelector('.btn-view');
        if (viewBtn) {
            viewBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                try { console.log('invoice view button clicked', inv.so_hoa_don || inv.id || inv.stt); } catch (ex) { }
                if (typeof window.openDetailModal === 'function') {
                    window.openDetailModal(inv);
                } else {
                    console.error('openDetailModal is not defined');
                }
            });
        }

        // Row click opens detail as well (click outside the button)
        row.addEventListener('click', () => {
            try { console.log('invoice row clicked', inv.so_hoa_don || inv.id || inv.stt); } catch (ex) { }
            if (typeof window.openDetailModal === 'function') {
                window.openDetailModal(inv);
            } else {
                console.error('openDetailModal is not defined');
            }
        });

        fragment.appendChild(row);
    });

    tableBody.appendChild(fragment);
}
window.renderInvoices = renderInvoices;

// NOTE: renderInvoices only renders the rows given. Pagination is handled separately

// Client-side pagination helpers
window._invoiceCache = null;
window._pageSize = 20;

// Store invoices for client-side paging and show first page
window.paginateClient = function (invoices, pageSize = 20) {
    if (!Array.isArray(invoices)) return;
    if (typeof window.setInvoiceRAM === 'function') {
        window.setInvoiceRAM(invoices);
    } else {
        window._invoiceCache = invoices;
    }
    window._pageSize = pageSize || 20;
    if (typeof window.updateExportXmlButtonState === 'function') {
        window.updateExportXmlButtonState(invoices.length > 0);
    }
    window.showPage(1);
};

// Show a specific page from cached invoices
window.showPage = function (page) {
    if (!window._invoiceCache || window._invoiceCache.length === 0) return;
    const total = window._invoiceCache.length;
    const pageSize = window._pageSize || 20;
    const totalPages = Math.max(1, Math.ceil(total / pageSize));
    const currentPage = Math.min(Math.max(1, page || 1), totalPages);

    const start = (currentPage - 1) * pageSize;
    const slice = window._invoiceCache.slice(start, start + pageSize);

    // render current page rows
    renderInvoices(slice);

    // update pagination UI
    renderPagination(total, currentPage, pageSize);
};

// Pagination helper: accepts total items, current page, page size
window.renderPagination = function (totalItems, currentPage, pageSize) {
    const container = document.getElementById('crawler-pagination');
    const info = document.getElementById('crawler-pagination-info');
    if (!container) return;

    const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
    currentPage = Math.min(Math.max(1, currentPage), totalPages);

    // Build nicer pagination UI
    container.innerHTML = '';
    // Prev button
    const prev = document.createElement('button');
    prev.className = `px-3 py-1 rounded-lg border border-slate-200 hover:bg-slate-50 ${currentPage === 1 ? 'opacity-50 cursor-not-allowed' : ''}`;
    prev.innerText = '‹';
    prev.disabled = (currentPage === 1);
    prev.dataset.page = currentPage - 1;
    container.appendChild(prev);

    // Page numbers (condensed)
    const windowSize = 5;
    const start = Math.max(1, currentPage - Math.floor(windowSize / 2));
    const end = Math.min(totalPages, start + windowSize - 1);
    if (start > 1) {
        const first = document.createElement('button');
        first.className = 'px-2 py-1 rounded-md text-sm';
        first.innerText = '1';
        first.dataset.page = 1;
        container.appendChild(first);
        if (start > 2) {
            const dots = document.createElement('span');
            dots.className = 'px-2 text-slate-400';
            dots.innerText = '...';
            container.appendChild(dots);
        }
    }

    for (let p = start; p <= end; p++) {
        const b = document.createElement('button');
        b.className = p === currentPage ? 'px-3 py-1 rounded-md bg-slate-100 font-semibold' : 'px-2 py-1 rounded-md';
        b.innerText = p.toString();
        b.dataset.page = p;
        container.appendChild(b);
    }

    if (end < totalPages) {
        if (end < totalPages - 1) {
            const dots = document.createElement('span');
            dots.className = 'px-2 text-slate-400';
            dots.innerText = '...';
            container.appendChild(dots);
        }
        const last = document.createElement('button');
        last.className = 'px-2 py-1 rounded-md text-sm';
        last.innerText = totalPages.toString();
        last.dataset.page = totalPages;
        container.appendChild(last);
    }

    // Next button
    const next = document.createElement('button');
    next.className = `px-3 py-1 rounded-lg border border-slate-200 hover:bg-slate-50 ${currentPage === totalPages ? 'opacity-50 cursor-not-allowed' : ''}`;
    next.innerText = '›';
    next.disabled = (currentPage === totalPages);
    next.dataset.page = currentPage + 1;
    container.appendChild(next);

    // Info left
    if (info) {
        info.innerText = `Trang ${currentPage} trên ${totalPages} - Tổng ${totalItems} hóa đơn`;
    }

    // Attach handlers
    container.querySelectorAll('button[data-page]').forEach(btn => {
        btn.addEventListener('click', () => {
            const p = parseInt(btn.getAttribute('data-page'));
            if (isNaN(p) || p < 1) return;
            // If we have client cache, use it
            if (window._invoiceCache && window._invoiceCache.length) {
                window.showPage(p);
            } else {
                // request page from host
                if (window.chrome && window.chrome.webview) {
                    console.log('pagination click', p, pageSize);
                    window.chrome.webview.postMessage({ action: 'REQUEST_PAGE', data: { page: p, pageSize } });
                }
            }
        });
    });
};

// Hàm bổ trợ màu sắc trạng thái
function getStatusStyle(status) {
    const s = status.toLowerCase();
    if (s.includes("thay thế")) return "bg-orange-50 text-orange-600 ring-1 ring-orange-200";
    if (s.includes("hủy")) return "bg-red-50 text-red-600 ring-1 ring-red-200";
    if (s.includes("gốc") || s.includes("mới")) return "bg-emerald-50 text-emerald-600 ring-1 ring-emerald-200";
    return "bg-slate-50 text-slate-500 ring-1 ring-slate-200";
}
window.updateCrawlerStatus = function (data) {

    // Dùng #crawl-progress-modal (dialog giữa màn hình) thay vì #crawler-status-box đã ẩn
    const modal = document.getElementById('crawl-progress-modal');
    const cpdStatus = document.getElementById('cpd-status');
    const cpdSub = document.getElementById('cpd-sub');
    const cpdCount = document.getElementById('cpd-count');
    const cpdTitle = document.getElementById('cpd-title');
    const cpdSubtitle = document.getElementById('cpd-subtitle');
    const cpdSpinner = document.getElementById('cpd-spinner-svg');
    const cpdDone = document.getElementById('cpd-icon-done');
    const cpdError = document.getElementById('cpd-icon-error');
    if (!modal) return;

    const status = data.status || data.Status || '';
    const isDone = (data.is_done !== undefined) ? data.is_done : (data.IsDone || false);
    const total = data.total_on_tax || data.TotalOnTax || 0;
    const isError = status.includes('❌') || status.toLowerCase().includes('lỗi') || status.toLowerCase().includes('timeout');

    // Hiển thị modal
    modal.style.display = 'flex';
    modal.style.opacity = '1';
    modal.style.transition = '';

    if (cpdTitle) {
        cpdTitle.innerText = isError ? 'Lỗi tìm kiếm' : isDone ? 'Tìm kiếm hoàn tất!' : 'Đang tìm kiếm hóa đơn...';
        cpdTitle.style.color = isError ? '#dc2626' : isDone ? '#059669' : '#0f172a';
    }
    if (cpdSubtitle) {
        cpdSubtitle.innerText = isError
            ? 'Kiểm tra lại ngày và thử lại'
            : isDone ? 'Kết quả đã được hiển thị bên dưới'
                : 'Vui lòng không đóng cửa sổ';
    }
    if (cpdStatus) {
        cpdStatus.innerText = status;
        cpdStatus.style.color = isError ? '#dc2626' : '#1e293b';
    }
    if (cpdSub) {
        // Cập nhật khi có total thực sự hoặc khi search kết thúc (isDone)
        if (total > 0) {
            cpdSub.innerText = `Tìm thấy tổng cộng ${total} hóa đơn`;
        } else if (isDone) {
            cpdSub.innerText = 'Không tìm thấy hóa đơn nào';
        } else {
            cpdSub.innerText = data.sub || data.sub_detail || data.SubDetail || '';
        }
    }
    if (cpdCount) {
        cpdCount.innerText = total > 0 ? total : 0;
    }
    if (cpdSpinner) cpdSpinner.style.display = (isDone || isError) ? 'none' : 'block';
    if (cpdDone) cpdDone.style.display = isDone ? 'flex' : 'none';
    if (cpdError) cpdError.style.display = isError ? 'flex' : 'none';
    const cancelWrap = document.getElementById('cpd-cancel-wrap');
    if (cancelWrap) cancelWrap.style.display = (isDone || isError) ? 'none' : 'flex';

    // Tự ẩn sau khi xong
    if (isDone || isError) {
        setTimeout(() => {
            modal.style.transition = 'opacity 0.35s ease';
            modal.style.opacity = '0';
            setTimeout(() => {
                modal.style.display = 'none';
                modal.style.opacity = '1';
                modal.style.transition = '';
            }, 380);
        }, isDone ? 3000 : 5000);
    }
};
/**
 * Hàm xử lý xuất XML và Cào dữ liệu hóa đơn.
 * Quy trình: Chọn thư mục -> Gửi lệnh Cào -> Chờ phản hồi từ C# (Backend) -> Hoàn tất.
 * @param {HTMLElement} btn - Nút bấm được click để điều khiển trạng thái UI.
 */
async function handleExportXml(btn) {
    const originalContent = btn.innerHTML;

    // --- HELPER: Điều khiển dialog tiến độ #crawl-progress-modal ---
    const modal = document.getElementById('crawl-progress-modal');
    const cpdStatus = document.getElementById('cpd-status');
    const cpdSub = document.getElementById('cpd-sub');
    const cpdCount = document.getElementById('cpd-count');
    const cpdTitle = document.getElementById('cpd-title');
    const cpdSubtitle = document.getElementById('cpd-subtitle');
    const cpdSpinner = document.getElementById('cpd-spinner-svg');
    const cpdDone = document.getElementById('cpd-icon-done');
    const cpdError = document.getElementById('cpd-icon-error');

    function showProgress(status, subDetail = '', isDone = false, isError = false) {
        if (!modal) return;
        modal.style.display = 'flex';
        if (cpdStatus) {
            cpdStatus.innerText = status || '';
            cpdStatus.style.color = isError ? '#dc2626' : isDone ? '#059669' : '#1e293b';
        }
        if (cpdSub) cpdSub.innerText = subDetail || '';
        if (cpdSpinner) cpdSpinner.style.display = (isDone || isError) ? 'none' : 'block';
        if (cpdDone) cpdDone.style.display = isDone ? 'flex' : 'none';
        if (cpdError) cpdError.style.display = isError ? 'flex' : 'none';
        const cpdCancel = document.getElementById('cpd-cancel-wrap');
        if (cpdCancel) cpdCancel.style.display = (isDone || isError) ? 'none' : 'flex';
        if (cpdTitle) {
            cpdTitle.innerText = isError ? 'Có lỗi xảy ra' : isDone ? 'Tải hoàn tất! 🎉' : 'Đang tải hóa đơn...';
            cpdTitle.style.color = isError ? '#dc2626' : isDone ? '#059669' : '#0f172a';
        }
        if (cpdSubtitle) {
            cpdSubtitle.innerText = isError
                ? 'Kiểm tra lại và thử lại'
                : isDone ? 'File XML đã được lưu vào thư mục'
                    : 'Vui lòng không đóng cửa sổ';
        }
    }

    function setProgressFolder(folderPath) {
        const wrap = document.getElementById('cpd-folder-wrap');
        const folder = document.getElementById('cpd-folder');
        if (wrap && folder && folderPath) {
            folder.innerText = folderPath;
            wrap.style.display = 'block';
        }
    }

    function setProgressCount(n) {
        if (cpdCount) cpdCount.innerText = n || 0;
    }

    function hideProgress(delayMs = 0) {
        if (!modal) return;
        setTimeout(() => {
            modal.style.transition = 'opacity 0.35s ease';
            modal.style.opacity = '0';
            setTimeout(() => {
                modal.style.display = 'none';
                modal.style.opacity = '1';
                modal.style.transition = '';
            }, 380);
        }, delayMs);
    }

    // --- END HELPER ---

    // 0. Kiểm tra dữ liệu hóa đơn: chỉ cho phép xuất khi có dữ liệu
    const hasData = Array.isArray(window._invoiceCache) && window._invoiceCache.length > 0;
    if (!hasData) {
        btn.disabled = true;
        window.showAlert?.('Thông báo', 'Chưa có dữ liệu hóa đơn để xuất! Vui lòng tìm kiếm dữ liệu trước.', 'warning');
        return;
    }

    // 1. Kiểm tra môi trường WebView2
    if (!(window.chrome?.webview)) {
        window.showAlert?.('Lỗi', 'WebView2 chưa sẵn sàng!', 'error');
        return;
    }

    // 2. Khóa nút + hiện tiến độ ban đầu
    btn.disabled = true;
    btn.innerHTML = `<span class="text-sm">⏳</span> Đang chuẩn bị...`;
    showProgress('Đang chờ chọn thư mục lưu...', 'Vui lòng chọn thư mục trong hộp thoại vừa mở.');

    // 3. Khởi tạo requestId
    const requestId = Math.random().toString(36).slice(2);
    window._pendingWebviewRequests = window._pendingWebviewRequests || {};

    // 4. Promise bao bọc toàn bộ luồng bất đồng bộ
    const promise = new Promise((resolve, reject) => {
        // Timeout 5 phút (crawl nhiều hóa đơn có thể lâu)
        const timer = setTimeout(() => {
            try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
            reject(new Error('Timeout — quá trình tải quá lâu (5 phút)'));
        }, 5 * 60 * 1000);

        window._pendingWebviewRequests[requestId] = (payload) => {
            try {
                const action = payload && payload.action;

                // Bước 1: Người dùng đã chọn thư mục, Backend bắt đầu nhận lệnh
                if (action === 'OUTPUT_DIR_SELECTED') {
                    const out = payload.output_dir || payload.outputDir || '';
                    if (!out) {
                        clearTimeout(timer);
                        try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
                        reject(new Error('Người dùng đã hủy chọn thư mục'));
                        return;
                    }
                    showProgress(
                        'Đã chọn thư mục, đang gửi lệnh...',
                        'Hệ thống sẽ bắt đầu từ trang 1 sau khi nhận được lệnh...'
                    );
                    setProgressFolder(out);
                    btn.innerHTML = `<span class="text-sm">⏳</span> Đang gửi lệnh...`;

                    window.chrome.webview.postMessage({
                        action: 'START_CRAWL',
                        output_dir: out,
                        mst: window._loggedInMst || window._selectedCompanyMst || '',
                        invoice_type: window._currentInvoiceType || 'ban_ra',
                        requestId
                    });
                    return;
                }

                // Bước 2: Backend xác nhận đã nhận và bắt đầu chạy
                if (action === 'START_CRAWL_ACK') {
                    showProgress(
                        'Hệ thống đang bắt đầu tải hóa đơn...',
                        'Đang mở từng hóa đơn và tải file ZIP về...'
                    );
                    btn.innerHTML = `<span class="text-sm">⏳</span> Đang tải...`;
                    return;
                }

                // Bước 3: Cập nhật tiến độ thực tế từ polling (C# gửi mỗi 1 giây)
                if (action === 'CRAWL_STATUS_UPDATE') {
                    const st = payload.status || payload.Status || '';
                    const total = payload.total_invoices || payload.TotalInvoices || 0;
                    const sub = total > 0 ? `Đã xử lý được ${total} hóa đơn` : 'Hệ thống đang khởi động...';
                    showProgress(st, sub);
                    setProgressCount(total);
                    btn.innerHTML = `<span class="text-sm">⏳</span> ${total > 0 ? total + ' HĐ' : 'Đang tải...'}`;
                    return;
                }

                // Bước 4: Lỗi trong quá trình xử lý
                if (action === 'CRAWL_ERROR') {
                    clearTimeout(timer);
                    try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
                    const msg = payload.message || payload.Message || 'Lỗi không xác định';
                    showProgress(msg, 'Kiểm tra log để biết thêm chi tiết', false, true);
                    hideProgress(6000);
                    reject(new Error(msg));
                    return;
                }

                // Bước 5: Hoàn tất thành công
                if (action === 'CRAWL_FINISHED') {
                    clearTimeout(timer);
                    try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
                    resolve(payload);
                    return;
                }

                // Fallback
                if (payload && (payload.result || payload.status === 'done')) {
                    clearTimeout(timer);
                    try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
                    resolve(payload);
                }

            } catch (ex) {
                clearTimeout(timer);
                try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
                reject(ex);
            }
        };
    });

    // 5. Kích hoạt: mở hộp thoại chọn thư mục
    window.chrome.webview.postMessage({
        action: 'REQUEST_SELECT_OUTPUT_DIR',
        requestId
    });

    // 6. Chờ kết quả
    try {
        const result = await promise;
        const total = result?.total_invoices || result?.TotalInvoices || '';
        const msg = total ? `Đã tải xong ${total} hóa đơn!` : 'Hoàn tất xuất XML!';
        showProgress(msg, 'File XML đã được lưu vào thư mục bạn chọn.', true);
        // Tự đóng status box sau 5 giây
        setTimeout(hideProgress, 5000);
        window.showAlert?.('Thành công', msg, 'success');
    } catch (err) {
        console.error('Export XML failed:', err);
        const errMsg = err.message || 'Không xác định';
        showProgress('Quá trình tải thất bại', errMsg, false, true);
        setTimeout(hideProgress, 6000);
        window.showAlert?.('Lỗi xuất XML', errMsg, 'error');
    } finally {
        const stillHasData = Array.isArray(window._invoiceCache) && window._invoiceCache.length > 0;
        btn.disabled = !stillHasData;
        btn.innerHTML = originalContent;
    }
}

// ============================================================
// CHỨC NĂNG TẢI CHI TIẾT (DOWNLOAD DETAIL)
// Dùng lại luồng START_CRAWL (backend CrawlAllPages) với UI nâng cao
// ============================================================

window._isDownloadingDetail = false;
window._downloadTimerInterval = null;
window._downloadTimerSeconds = 0;

// Bấm nút [🗂️ Tải chi tiết]
function handleDownloadDetail() {
    // 1. Kiểm tra dữ liệu
    const invoices = window._invoiceCache;
    if (!Array.isArray(invoices) || invoices.length === 0) {
        window.showAlert?.('Thông báo', 'Chưa có dữ liệu hóa đơn! Vui lòng bấm Tìm kiếm trước.', 'warning');
        return;
    }
    if (!(window.chrome?.webview)) {
        window.showAlert?.('Lỗi', 'WebView2 chưa sẵn sàng!', 'error');
        return;
    }
    if (window._isDownloadingDetail) return;

    // 2. Bật trạng thái
    window._isDownloadingDetail = true;
    const btnDetail = document.getElementById('btn-download-detail');
    const btnSearch = document.getElementById('btn-open-search');
    const btnStop = document.getElementById('btn-stop-download');
    const timerEl = document.getElementById('download-timer');

    const spinnerSvg = '<svg class="w-3.5 h-3.5 animate-spin text-emerald-600 inline-block" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path></svg>';

    if (btnDetail) { btnDetail.disabled = true; btnDetail.innerHTML = `${spinnerSvg} <span>Đang chờ...</span>`; }
    if (btnSearch) btnSearch.disabled = true;
    if (btnStop) { btnStop.style.display = 'flex'; btnStop.disabled = false; btnStop.innerHTML = '<span class="text-sm">⏹</span> Dừng'; }
    if (timerEl) { timerEl.style.display = 'inline'; timerEl.innerText = '00:00'; }

    // 3. Timer đếm giờ
    window._downloadTimerSeconds = 0;
    window._downloadTimerInterval = setInterval(() => {
        window._downloadTimerSeconds++;
        const m = String(Math.floor(window._downloadTimerSeconds / 60)).padStart(2, '0');
        const s = String(window._downloadTimerSeconds % 60).padStart(2, '0');
        if (timerEl) timerEl.innerText = `${m}:${s}`;
    }, 1000);

    // 4. Gửi lệnh mở hộp thoại chọn thư mục — dùng cùng luồng START_CRAWL
    const requestId = 'dd_' + Math.random().toString(36).slice(2);

    // Timeout 10 phút
    const timeoutTimer = setTimeout(() => {
        try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
        _onDownloadError({ message: 'Timeout — quá trình tải quá lâu (10 phút)' });
    }, 10 * 60 * 1000);

    window._pendingWebviewRequests[requestId] = (payload) => {
        try {
            const action = payload && payload.action;

            // C# trả về thư mục đã chọn
            if (action === 'OUTPUT_DIR_SELECTED') {
                const out = payload.output_dir || payload.outputDir || '';
                if (!out) {
                    clearTimeout(timeoutTimer);
                    try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
                    _onDownloadStopped({ message: 'Người dùng đã hủy chọn thư mục' });
                    return;
                }
                if (btnDetail) btnDetail.innerHTML = `${spinnerSvg} <span>Đang gửi lệnh...</span>`;

                // Gửi START_CRAWL dùng backend có sẵn
                window.chrome.webview.postMessage({
                    action: 'START_CRAWL',
                    output_dir: out,
                    mst: window._loggedInMst || window._selectedCompanyMst || '',
                    invoice_type: window._currentInvoiceType || 'ban_ra',
                    requestId
                });
                return;
            }

            // Backend ACK
            if (action === 'START_CRAWL_ACK') {
                if (btnDetail) btnDetail.innerHTML = `${spinnerSvg} <span>Đang tải...</span>`;
                return;
            }

            // Cập nhật tiến độ
            if (action === 'CRAWL_STATUS_UPDATE') {
                const total = payload.total_invoices || 0;
                const st = payload.status || '';
                if (btnDetail) {
                    btnDetail.innerHTML = `${spinnerSvg} <span>${total > 0 ? total + ' HĐ' : (st || 'Đang tải...')}</span>`;
                }
                return;
            }

            // Lỗi
            if (action === 'CRAWL_ERROR') {
                clearTimeout(timeoutTimer);
                try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
                _onDownloadError({ message: payload.message || 'Lỗi không xác định' });
                return;
            }

            // Hoàn tất
            if (action === 'CRAWL_FINISHED') {
                clearTimeout(timeoutTimer);
                try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
                if (payload.status === 'stopped' || payload.is_stopped) {
                    _onDownloadStopped(payload);
                } else {
                    _onDownloadFinished(payload);
                }
                return;
            }
        } catch (ex) {
            clearTimeout(timeoutTimer);
            try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
            _onDownloadError({ message: ex.message || 'Lỗi không xác định' });
        }
    };

    // 5. Kích hoạt: mở hộp thoại chọn thư mục
    window.chrome.webview.postMessage({
        action: 'REQUEST_SELECT_OUTPUT_DIR',
        requestId
    });
}

// Bấm nút [⏹ Dừng]
function handleStopDownload() {
    if (!(window.chrome?.webview)) return;
    const btnStop = document.getElementById('btn-stop-download');
    if (btnStop) { btnStop.innerHTML = '<span class="text-sm">⏳</span> Đang dừng...'; btnStop.disabled = true; }
    // Gửi lệnh dừng dùng endpoint /stop có sẵn
    window.chrome.webview.postMessage({ action: 'STOP_CRAWL' });
}

// Hoàn tất
function _onDownloadFinished(payload) {
    window._isDownloadingDetail = false;
    _stopDownloadTimer();
    _restoreDownloadUI();

    const parsedCount = payload?.parsed_xml_count || (Array.isArray(payload?.parsed_invoices) ? payload.parsed_invoices.length : 0);
    const totalFound = payload?.total_files_found || 0;
    const totalSkipped = payload?.total_files_skipped || 0;
    const totalFailed = payload?.total_files_failed || 0;

    if (parsedCount > 0) {
        window._parsedXmlInvoices = payload?.parsed_invoices || [];
        console.log(`[XML Parser] Đã bóc tách thành công ${parsedCount} hóa đơn:`, window._parsedXmlInvoices);

        let msg = `Đã bóc tách thành công: ${parsedCount} hóa đơn`;
        if (totalFound > 0) {
            msg += ` (Tổng quét: ${totalFound} file`;
            if (totalSkipped > 0) msg += `, Bỏ qua: ${totalSkipped}`;
            if (totalFailed > 0) msg += `, Lỗi: ${totalFailed}`;
            msg += `)`;
        }
        window.showAlert?.('Thành công', msg, 'success');
    } else {
        window.showAlert?.('Thành công', 'Đã tải xong toàn bộ hóa đơn chi tiết!', 'success');
    }
}

// Dừng
function _onDownloadStopped(payload) {
    window._isDownloadingDetail = false;
    _stopDownloadTimer();
    _restoreDownloadUI();
    window.showAlert?.('Thông báo', payload.message || 'Đã dừng tiến trình', 'warning');
}

// Lỗi
function _onDownloadError(payload) {
    window._isDownloadingDetail = false;
    _stopDownloadTimer();
    _restoreDownloadUI();
    window.showAlert?.('Lỗi', payload.message || 'Lỗi trong quá trình tải', 'error');
}

function _stopDownloadTimer() {
    if (window._downloadTimerInterval) { clearInterval(window._downloadTimerInterval); window._downloadTimerInterval = null; }
}

function _restoreDownloadUI() {
    const btnDetail = document.getElementById('btn-download-detail');
    const btnSearch = document.getElementById('btn-open-search');
    const btnStop = document.getElementById('btn-stop-download');
    const timerEl = document.getElementById('download-timer');
    const hasData = Array.isArray(window._invoiceCache) && window._invoiceCache.length > 0;
    if (btnDetail) { btnDetail.disabled = !hasData; btnDetail.innerHTML = '<span class="text-sm">🗂️</span> Tải chi tiết'; }
    if (btnSearch) btnSearch.disabled = false;
    if (btnStop) { btnStop.style.display = 'none'; btnStop.disabled = false; btnStop.innerHTML = '<span class="text-sm">⏹</span> Dừng'; }
    setTimeout(() => { if (timerEl) timerEl.style.display = 'none'; }, 4000);
}
