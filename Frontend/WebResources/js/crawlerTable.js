// Hàm vẽ danh sách công ty vào Dropdown
window.renderCompanyDropdown = function () {
    const selectCompany = document.getElementById('select-company');
    if (!selectCompany) return;

    if (!tempStorage || tempStorage.length === 0) {
        selectCompany.innerHTML = '<option value="">Không có dữ liệu</option>';
        return;
    }

    // Tạo HTML cho các option từ tempStorage
    // Giả sử JSON trả về có trường { "taxCode": "...", "name": "..." }
    const html = tempStorage.map(company => `
        <option value="${company.mst}">
            ${company.company || company.mst}
        </option>
    `).join('');

    selectCompany.innerHTML = html;
    console.log("Hệ thống: Đã cập nhật danh sách công ty!");
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

    // Normalize fields
    const isDone = data && (data.is_done !== undefined ? data.is_done : (data.IsDone !== undefined ? data.IsDone : false));
    let invoices = data.invoices || data.Invoices || null;
    if (typeof invoices === 'string') {
        try { invoices = JSON.parse(invoices); } catch { invoices = null; }
    }

    // If server provides total count, render pagination; otherwise default to page size behaviour
    const total = data.total_on_tax || data.TotalOnTax || (Array.isArray(invoices) ? invoices.length : 0);

    if (isDone) {
        if (Array.isArray(invoices) && invoices.length) {
            // small delay to allow status UI to update first
            setTimeout(() => {
                try { renderInvoices(invoices); }
                catch (ex) { console.error('renderInvoices error', ex); }
                // handle pagination (client-side if invoices present)
                handlePagination({ invoices, totalItems: total, currentPage: 1, pageSize: 20 });
                if (window.finalizeSearchUI) window.finalizeSearchUI();
            }, 50);
        } else {
            // Không có hóa đơn → xóa sạch bảng cũ và hiện thông báo trống
            renderInvoices([]);
            handlePagination({ invoices: null, totalItems: 0, currentPage: 1, pageSize: 20 });
            if (window.finalizeSearchUI) window.finalizeSearchUI();
        }
    } else {
        // intermediate update: optionally render partial invoices
        if (Array.isArray(invoices) && invoices.length) {
            try { renderInvoices(invoices); }
            catch (ex) { console.error('renderInvoices error', ex); }
            handlePagination({ invoices, totalItems: total, currentPage: 1, pageSize: 20 });
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
            btn.innerHTML = '🔍 Tìm kiếm';
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
    document.getElementById('captcha-modal').classList.add('hidden');
    document.getElementById('input-captcha').value = '';
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

window.updateLoginStatus = function (isLoggedIn) {
    const btn = document.getElementById('btn-auth');
    if (!btn) return;

    btn.disabled = false;
    btn.style.opacity = "";
    btn.style.cursor = "";

    const baseClass = "ml-2 text-[10px] font-bold px-3 py-1 rounded-md transition-all uppercase active:scale-95 disabled:opacity-50 disabled:cursor-not-allowed ";
    const emeraldStyle = "bg-emerald-50 text-emerald-600 hover:bg-emerald-600 hover:text-white";
    const roseStyle = "bg-rose-50 text-rose-600 hover:bg-rose-600 hover:text-white";

    if (isLoggedIn) {
        btn.innerText = 'Đăng xuất';
        btn.className = baseClass + roseStyle;
        btn.dataset.state = "logged-in";
        window._isLoggedIn = true;
    } else {
        btn.innerText = 'Đăng nhập';
        btn.className = baseClass + emeraldStyle;
        btn.dataset.state = "logged-out";
        window._isLoggedIn = false;

        // Xóa trắng dữ liệu hóa đơn khi đăng xuất
        const tbody = document.getElementById('crawler-results-body');
        if (tbody) tbody.innerHTML = '';

        const paginationInfo = document.getElementById('crawler-pagination-info');
        if (paginationInfo) paginationInfo.innerHTML = '';

        const pagination = document.getElementById('crawler-pagination');
        if (pagination) pagination.innerHTML = '';

        // Reset status bar nếu có
        const statusEl = document.getElementById('cpd-status');
        if (statusEl) statusEl.textContent = '';

        const countEl = document.getElementById('cpd-count');
        if (countEl) countEl.textContent = '0';
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
    if (confirm("Bạn có chắc chắn muốn đăng xuất tài khoản thuế này không?")) {

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

// --- XỬ LÝ CHỌN NGÀY CHO ACCONLINE ---

document.addEventListener('DOMContentLoaded', function () {
    const dateFrom = document.getElementById('date-from');
    const dateTo = document.getElementById('date-to');

    // 1. Tự động đặt ngày max là hôm nay khi vừa mở App
    const today = new Date().toISOString().split('T')[0];
    dateFrom.max = today;
    dateTo.max = today;

    function handleDateValidation(e) {
        const MS_PER_DAY = 24 * 60 * 60 * 1000;

        // Use the date inputs' values directly and create Date objects
        const start = new Date(dateFrom.value);
        const end = new Date(dateTo.value);

        if (!dateFrom.value || !dateTo.value) return;

        // Use UTC midnight to avoid timezone/DST issues
        const utcStart = Date.UTC(start.getFullYear(), start.getMonth(), start.getDate());
        const utcEnd = Date.UTC(end.getFullYear(), end.getMonth(), end.getDate());

        const diffDays = Math.floor((utcEnd - utcStart) / MS_PER_DAY); // 0 when same day
        const diffDaysInclusive = diffDays + 1; // count including both endpoints

        // TRƯỜNG HỢP 1: Chọn ngày kết thúc nhỏ hơn ngày bắt đầu
        if (diffDays < 0) {
            window.showAlert('Lỗi logic', 'Ngày kết thúc không được nhỏ hơn ngày bắt đầu!', 'error');
            if (e.target.id === 'date-from') {
                dateTo.value = dateFrom.value;
            } else {
                dateFrom.value = dateTo.value;
            }
            return;
        }

        // TRƯỜNG HỢP 2: Khoảng cách vượt quá 31 ngày (1 month)
        if (diffDaysInclusive > 31) {
            window.showAlert('Giới hạn hệ thống', 'Hệ thống chỉ hỗ trợ tra cứu tối đa trong 1 tháng (31 ngày)!', 'error');

            if (e.target.id === 'date-from') {
                // Set "to" = from + 30 days (so inclusive range = 31 days)
                const newEnd = new Date(Date.UTC(start.getFullYear(), start.getMonth(), start.getDate()) + (30 * MS_PER_DAY));
                const finalEnd = newEnd > new Date() ? new Date() : newEnd;
                dateTo.value = finalEnd.toISOString().split('T')[0];
            } else {
                // Set "from" = to - 30 days (so inclusive range = 31 days)
                const newStart = new Date(Date.UTC(end.getFullYear(), end.getMonth(), end.getDate()) - (30 * MS_PER_DAY));
                dateFrom.value = newStart.toISOString().split('T')[0];
            }
        }
    }

    // Gán sự kiện thay đổi cho cả 2 ô
    dateFrom.addEventListener('change', handleDateValidation);
    dateTo.addEventListener('change', handleDateValidation);

    // Attach export button handlers
    // NOTE: btn-export-xml dùng onclick trực tiếp trong HTML vì component được inject động
    // (DOMContentLoaded chạy trước khi crawler_table.html được fetch và inject vào DOM)
    const btnPdf = document.getElementById('btn-export-pdf');
    // btn-export-pdf chưa có handler hoàn chỉnh, giữ nguyên
    if (btnPdf) btnPdf.addEventListener('click', () => window.handleExportRequest('pdf'));
});

// Hàm để C# bốc dữ liệu gửi sang Go
window.getFilterParams = function () {
    return {
        fromDate: document.getElementById('date-from').value,
        toDate: document.getElementById('date-to').value
    };
};
// Open detail modal (ensure function exists before renderInvoices uses it)
window.openDetailModal = function (inv) {

    if (!inv) return;
    const id = 'invoiceDetailModal';
    const existing = document.getElementById(id);
    if (existing) existing.remove();


    // Giả định hàm rawText đã được định nghĩa ở trên để giữ nguyên bản dữ liệu

    // Hàm giữ nguyên bản dữ liệu cào được
    const raw = (s) => (s && s !== "" && s !== "---") ? String(s).trim() : "---";

    // Hàm hỗ trợ định dạng hiển thị tiền tệ và đơn vị
    const fmt = (val, unit = "VND") => {
        if (!val || val === "---") return "0 " + unit;
        return val + " " + unit;
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
                            <th class="p-4 text-right w-56">Thành tiền (VND)</th>
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

    // Xóa sạch kết quả cũ trước khi search mới
    const tableBody = document.getElementById('crawler-results-body');
    if (tableBody) tableBody.innerHTML = '';
    const paginationEl = document.getElementById('pagination-container');
    if (paginationEl) paginationEl.innerHTML = '';

    // Reset bộ đếm hóa đơn về 0
    const cpdCount = document.getElementById('cpd-count');
    if (cpdCount) cpdCount.innerText = '0';
    const paginationInfo = document.getElementById('crawler-pagination-info');
    if (paginationInfo) paginationInfo.innerText = '';

    // 1. Lấy dữ liệu từ hàm bạn đã viết
    const params = window.getFilterParams();

    // 2. Kiểm tra nhanh (Double check lần cuối cho an toàn)
    if (!params.fromDate || !params.toDate) {
        if (searchBtn) { searchBtn.disabled = false; searchBtn.style.opacity = '1'; searchBtn.style.cursor = 'pointer'; searchBtn.innerHTML = '🔍 Tìm kiếm'; }
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
    if (diffDays + 1 > 31) {
        if (window.finalizeSearchUI) window.finalizeSearchUI();
        return window.showAlert('Giới hạn', 'Khoảng thời gian không được quá 31 ngày!', 'info');
    }

    // 3. Gửi sang C# (WinForms) để báo cho Backend Go bắt đầu Crawl
    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage({
            action: "SEARCH_INVOICE",
            data: {
                fromDate: params.fromDate,
                toDate: params.toDate
            }
        });

        // Có thể thêm hiệu ứng đổi text nút hoặc loading ở đây nếu muốn
        console.log("AccOnline: Đã gửi yêu cầu tra cứu từ " + params.fromDate + " đến " + params.toDate);

        // Watchdog: tự động finalize UI sau 35s nếu không nhận được IsDone signal
        if (window._searchWatchdogId) {
            clearTimeout(window._searchWatchdogId);
            window._searchWatchdogId = null;
        }
        const wd = window._searchWatchdogDuration || 35000;
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
    tableBody.innerHTML = '';

    if (!invoices || invoices.length === 0) {
        tableBody.innerHTML = `<tr><td colspan="7" class="py-20 text-center text-slate-300 italic">Chưa có dữ liệu để hiển thị</td></tr>`;
        return;
    }

    const fragment = document.createDocumentFragment();

    invoices.forEach((inv) => {
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
                    <span class="text-base font-bold text-slate-900">${inv.tong_thanh_toan}</span>
                    <!-- Tiền thuế: text-xs -->
                    <span class="text-xs text-slate-500 font-medium">Thuế: ${inv.tong_tien_thue} ${inv.don_vi_tien_te}</span>
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

// NOTE: renderInvoices only renders the rows given. Pagination is handled separately

// Client-side pagination helpers
window._invoiceCache = null;
window._pageSize = 20;

// Store invoices for client-side paging and show first page
window.paginateClient = function (invoices, pageSize = 20) {
    if (!Array.isArray(invoices)) return;
    window._invoiceCache = invoices;
    window._pageSize = pageSize || 20;
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
        // Chỉ cập nhật khi có total thực sự hoặc khi search kết thúc (isDone)
        if (total > 0) {
            cpdSub.innerText = `Tìm thấy tổng cộng ${total} hóa đơn`;
        } else if (isDone) {
            cpdSub.innerText = 'Không tìm thấy hóa đơn nào';
        }
        // Nếu total = 0 VÀ chưa done → giữ nguyên text cũ (không ghi đè)
    }
    if (cpdCount) {
        if (total > 0) cpdCount.innerText = total;
        else if (isDone) cpdCount.innerText = 0;
    }
    if (cpdSpinner) cpdSpinner.style.display = (isDone || isError) ? 'none' : 'block';
    if (cpdDone) cpdDone.style.display = isDone ? 'flex' : 'none';
    if (cpdError) cpdError.style.display = isError ? 'flex' : 'none';

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
        btn.disabled = false;
        btn.innerHTML = originalContent;
    }
}
