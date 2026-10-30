/**
 * Quản lý logic cho trang Admin - TaxScraper
 */

// 1. Hàm ẩn/hiện mật khẩu
function togglePass(inputId, iconId) {
    const input = document.getElementById(inputId);
    const icon = document.getElementById(iconId);

    if (input.type === "password") {
        input.type = "text";
        icon.classList.add("text-emerald-500");
        icon.classList.remove("text-gray-300");
    } else {
        input.type = "password";
        icon.classList.remove("text-emerald-500");
        icon.classList.add("text-gray-300");
    }
}

// 2. Quản lý trạng thái nút bấm (Loading spinner)
function setSubmitLoading(isLoading) {
    const btn = document.getElementById('btnSubmitAdmin');
    if (!btn) return;
    if (isLoading) {
        btn.disabled = true;
        btn.dataset.originalHtml = btn.innerHTML;
        btn.classList.add('opacity-80', 'cursor-not-allowed');
        btn.innerHTML = `
            <svg class="animate-spin h-5 w-5 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            <span>Đang ghi lên hệ thống...</span>
        `;
    } else {
        btn.disabled = false;
        btn.classList.remove('opacity-80', 'cursor-not-allowed');
        if (btn.dataset.originalHtml) {
            btn.innerHTML = btn.dataset.originalHtml;
        } else {
            btn.innerHTML = '<span>Xác nhận cấp quyền</span>';
        }
    }
}
window.setSubmitLoading = setSubmitLoading;

function quayLaiDangNhap() {
    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage(JSON.stringify({ action: 'NAVIGATE_TO_LOGIN' }));
    } else {
        window.location.href = '/login.html';
    }
}
window.quayLaiDangNhap = quayLaiDangNhap;

// 3. Hàm xử lý gửi dữ liệu thêm User
function postThemThanhVien() {
    const u = document.getElementById('new_user').value.trim();
    const p = document.getElementById('new_pass').value.trim();
    const cp = document.getElementById('confirm_pass').value.trim();

    // Kiểm tra dữ liệu đầu vào
    if (!u || !p || !cp) {
        showAlert("Lỗi nhập liệu", "Vui lòng điền đầy đủ các trường bắt buộc!", "error");
        return;
    }

    if (p !== cp) {
        showAlert("Mật khẩu không khớp", "Xác nhận mật khẩu không chính xác. Vui lòng kiểm tra lại!", "error");
        return;
    }

    const fullName = document.getElementById('new_fullname').value.trim();
    const hwid = document.getElementById('new_hwid').value.trim();
    const maxTax = parseInt(document.getElementById('new_max_tax').value, 10);
    const allowAI = document.getElementById('new_allow_ai').checked;
    const startVal = document.getElementById('new_start').value;
    const endVal = document.getElementById('new_end').value;

    if (isNaN(maxTax) || maxTax < 0) {
        showAlert("Giới hạn không hợp lệ", "Số MST tối đa phải là số từ 0 trở lên.", "error");
        return;
    }
    if (startVal && endVal && startVal > endVal) {
        showAlert("Ngày không hợp lệ", "Ngày kết thúc phải sau ngày bắt đầu.", "error");
        return;
    }

    // Hiển thị vòng xoay loading
    setSubmitLoading(true);

    // Gửi JSON về C# qua WebView2
    try {
        window.chrome.webview.postMessage(JSON.stringify({
            action: 'ADMIN_THEM_USER',
            username: u,
            password: p,
            fullName: fullName,
            hwid: hwid,
            maxTaxAccounts: maxTax,
            allowAI: allowAI,
            startDate: toViDate(startVal),
            endDate: toViDate(endVal)
        }));
    } catch (err) {
        setSubmitLoading(false);
        showAlert("Lỗi hệ thống", "Không thể kết nối với ứng dụng chính.", "error");
    }
}

// 4. Hàm làm mới form nhập liệu và quay lại đăng nhập
function lamMoiForm() {
    setSubmitLoading(false);
    document.getElementById('new_user').value = '';
    document.getElementById('new_pass').value = '';
    document.getElementById('confirm_pass').value = '';
    document.getElementById('new_fullname').value = '';
    document.getElementById('new_hwid').value = window.hardwareId || localStorage.getItem('hardware_id') || '';
    document.getElementById('new_max_tax').value = '5';
    document.getElementById('new_allow_ai').checked = false;
    const now = new Date();
    const yyyy = now.getFullYear();
    const mm = String(now.getMonth() + 1).padStart(2, '0');
    const dd = String(now.getDate()).padStart(2, '0');
    document.getElementById('new_start').value = `${yyyy}-${mm}-${dd}`;

    const nextYear = new Date();
    nextYear.setFullYear(nextYear.getFullYear() + 1);
    const y2 = nextYear.getFullYear();
    const m2 = String(nextYear.getMonth() + 1).padStart(2, '0');
    const d2 = String(nextYear.getDate()).padStart(2, '0');
    document.getElementById('new_end').value = `${y2}-${m2}-${d2}`;

    // Tự động quay lại trang đăng nhập sau 1.5 giây để người dùng kịp đọc thông báo
    setTimeout(function () {
        quayLaiDangNhap();
    }, 1500);
}

// yyyy-mm-dd (ô chọn ngày) -> dd/MM/yyyy (Google Sheet)
function toViDate(v) {
    if (!v) return '';
    const p = v.split('-');
    return p[2] + '/' + p[1] + '/' + p[0];
}

// 4. Lắng nghe sự kiện bàn phím
document.addEventListener('DOMContentLoaded', async function () {
    // 1. Tự động điền ngày bắt đầu (hôm nay) và ngày kết thúc (1 năm sau) nếu chưa có
    const startInput = document.getElementById('new_start');
    const endInput = document.getElementById('new_end');
    const now = new Date();
    const yyyy = now.getFullYear();
    const mm = String(now.getMonth() + 1).padStart(2, '0');
    const dd = String(now.getDate()).padStart(2, '0');
    if (startInput && !startInput.value) {
        startInput.value = `${yyyy}-${mm}-${dd}`;
    }
    if (endInput && !endInput.value) {
        const nextYear = new Date();
        nextYear.setFullYear(nextYear.getFullYear() + 1);
        const y2 = nextYear.getFullYear();
        const m2 = String(nextYear.getMonth() + 1).padStart(2, '0');
        const d2 = String(nextYear.getDate()).padStart(2, '0');
        endInput.value = `${y2}-${m2}-${d2}`;
    }

    const hwInput = document.getElementById('new_hwid');
    if (!hwInput) return;
    const cached = window.hardwareId || localStorage.getItem('hardware_id');
    if (cached && !hwInput.value) hwInput.value = cached;
    try {
        if (window.chrome && window.chrome.webview) {
            window.chrome.webview.postMessage(JSON.stringify({ action: 'GET_HWID' }));
        }
    } catch (_) { }
    try {
        const res = await fetch('http://127.0.0.1:18386/api/auth/hwid');
        const json = await res.json();
        const hwid = json && json.data && json.data.hwid;
        if (hwid) {
            window.hardwareId = hwid;
            localStorage.setItem('hardware_id', hwid);
            if (!hwInput.value) hwInput.value = hwid;
        }
    } catch (_) { }
});

document.addEventListener('keydown', function (e) {
    if (e.key === 'Enter') {
        // Nếu modal alert/confirm đang mở thì để modal xử lý phím Enter
        if (document.getElementById('dynamicModal') || document.getElementById('confirmModal')) {
            return;
        }
        e.preventDefault();
        postThemThanhVien();
    }
});
