// js/login.js

function guiLenhDangNhap() {
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
document.addEventListener('keypress', function (e) {
    if (e.key === 'Enter') {
        guiLenhDangNhap();
    }
});