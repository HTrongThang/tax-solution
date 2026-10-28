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

// 2. Hàm xử lý gửi dữ liệu thêm User
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

    // Gửi JSON về C# qua WebView2 (C# gọi lamMoiForm khi cấp tài khoản thành công)
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
        showAlert("Lỗi hệ thống", "Không thể kết nối với ứng dụng chính.", "error");
    }
}

// 3. Hàm làm mới form nhập liệu
function lamMoiForm() {
    document.getElementById('new_user').value = '';
    document.getElementById('new_pass').value = '';
    document.getElementById('confirm_pass').value = '';
    document.getElementById('new_fullname').value = '';
    document.getElementById('new_hwid').value = window.hardwareId || localStorage.getItem('hardware_id') || '';
    document.getElementById('new_max_tax').value = '5';
    document.getElementById('new_allow_ai').checked = false;
    document.getElementById('new_start').value = '';
    document.getElementById('new_end').value = '';
}

// yyyy-mm-dd (ô chọn ngày) -> dd/MM/yyyy (Google Sheet)
function toViDate(v) {
    if (!v) return '';
    const p = v.split('-');
    return p[2] + '/' + p[1] + '/' + p[0];
}

// 4. Lắng nghe sự kiện bàn phím
document.addEventListener('DOMContentLoaded', async function () {
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

document.addEventListener('keypress', function (e) {
    if (e.key === 'Enter') {
        postThemThanhVien();
    }
});
