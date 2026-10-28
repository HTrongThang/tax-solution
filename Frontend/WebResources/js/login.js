// js/login.js

function guiLenhDangNhap() {
    debugger;
    // Thêm .trim() để tránh trường hợp Thắng chỉ gõ dấu cách
    const userVal = document.getElementById('user').value.trim();
    const passVal = document.getElementById('pass').value.trim();
    const rememberVal = document.getElementById('remember').checked;

    // 1. Kiểm tra tài khoản
    if (!userVal) {
        showAlert("Thiếu thông tin", "Bạn ơi, đừng để trống tài khoản nhé!", "error");
        return;
    }

    // 2. Kiểm tra mật khẩu
    if (!passVal) {
        showAlert("Thiếu thông tin", "Mật khẩu cũng không được để trống đâu nè.", "error");
        return;
    }

    const payload = {
        action: 'Login',
        username: userVal,
        password: passVal,
        remember: rememberVal
    };

    // Gửi tin nhắn qua WebView2 tới WinForms
    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage(JSON.stringify(payload));
    } else {
        // Nếu chạy trên trình duyệt thường để test giao diện
        console.log("Dữ liệu gửi đi:", payload);
        showAlert("Thành công", "Dữ liệu đã được log ra console vì không chạy trong App.", "success");
    }
}

// Bắt sự kiện phím Enter
function moFormDangKy() {
    const modal = document.getElementById('register-modal');
    if (!modal) return;
    modal.classList.remove('hidden');
    modal.classList.add('flex');

    const hwInput = document.getElementById('reg_hwid');
    const cached = window.hardwareId || localStorage.getItem('hardware_id') || '';
    if (hwInput && cached) hwInput.value = cached;

    try {
        if (window.chrome && window.chrome.webview) {
            window.chrome.webview.postMessage(JSON.stringify({ action: 'GET_HWID' }));
        }
    } catch (_) { }
}

function dongFormDangKy() {
    const modal = document.getElementById('register-modal');
    if (!modal) return;
    modal.classList.add('hidden');
    modal.classList.remove('flex');
    ['reg_fullname', 'reg_user', 'reg_pass', 'reg_confirm'].forEach(function (id) {
        const el = document.getElementById(id);
        if (el) el.value = '';
    });
}

function guiYeuCauCapTaiKhoan() {
    const fullName = (document.getElementById('reg_fullname')?.value || '').trim();
    const u = (document.getElementById('reg_user')?.value || '').trim();
    const p = (document.getElementById('reg_pass')?.value || '').trim();
    const cp = (document.getElementById('reg_confirm')?.value || '').trim();

    if (!u || !p || !cp) {
        showAlert("Thiếu thông tin", "Vui lòng nhập đầy đủ Tên đăng nhập và Mật khẩu!", "error");
        return;
    }
    if (p !== cp) {
        showAlert("Mật khẩu không khớp", "Mật khẩu xác nhận không trùng khớp!", "error");
        return;
    }

    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage(JSON.stringify({
            action: 'REGISTER_REQUEST',
            username: u,
            password: p,
            fullName: fullName
        }));
    } else {
        showAlert("Lỗi hệ thống", "Không thể kết nối với ứng dụng chính.", "error");
    }
}

document.addEventListener('keypress', function (e) {
    if (e.key === 'Enter') {
        const modal = document.getElementById('register-modal');
        if (modal && !modal.classList.contains('hidden')) {
            guiYeuCauCapTaiKhoan();
        } else {
            guiLenhDangNhap();
        }
    }
});

