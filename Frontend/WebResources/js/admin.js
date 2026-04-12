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
        showAlert("Lỗi nhập liệu", "Vui lòng điền đầy đủ các thông tin nhân sự!", "error");
        return;
    }

    if (p !== cp) {
        showAlert("Mật khẩu không khớp", "Xác nhận mật khẩu không chính xác. Thắng kiểm tra lại nhé!", "error");
        return;
    }

    // Gửi JSON về C# qua WebView2
    try {
        window.chrome.webview.postMessage(JSON.stringify({
            action: 'ADMIN_THEM_USER',
            username: u,
            password: p
        }));

        // Xóa trắng form sau khi gửi
        lamMoiForm();

        showAlert("Thành công", "Đã gửi yêu cầu cấp quyền cho nhân viên mới!", "success");
    } catch (err) {
        showAlert("Lỗi hệ thống", "Không thể kết nối với ứng dụng chính.", "error");
    }
}

// 3. Hàm làm mới form nhập liệu
function lamMoiForm() {
    document.getElementById('new_user').value = '';
    document.getElementById('new_pass').value = '';
    document.getElementById('confirm_pass').value = '';
}

// 4. Lắng nghe sự kiện bàn phím
document.addEventListener('keypress', function (e) {
    if (e.key === 'Enter') {
        postThemThanhVien();
    }
});
