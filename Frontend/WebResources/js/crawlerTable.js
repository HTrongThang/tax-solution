// Hàm vẽ danh sách công ty vào Dropdown
window.renderCompanyDropdown = function () {
    debugger;
    const selectCompany = document.getElementById('select-company');
    if (!selectCompany) return;

    if (!tempStorage || tempStorage.length === 0) {
        selectCompany.innerHTML = '<option value="">Không có dữ liệu</option>';
        return;
    }

    // Tạo HTML cho các option từ tempStorage
    // Giả sử JSON trả về có trường { "taxCode": "...", "name": "..." }
    const html = tempStorage.map(company => `
        <option value="${company.taxCode}">
            ${company.company || company.company}
        </option>
    `).join('');

    selectCompany.innerHTML = html;
    console.log("Hệ thống: Đã cập nhật danh sách công ty!");
};

// Hàm mở Popup Captcha
window.openCaptcha = function (captchaBase64) {
    debugger;
    const modal = document.getElementById('captcha-modal');
    const img = document.getElementById('img-captcha');

    // Nếu có ảnh từ API thì truyền vào, không thì dùng ảnh mặc định
    if (captchaBase64) img.src = captchaBase64;

    modal.classList.remove('hidden');
    document.getElementById('input-captcha').focus();
};

// Hàm đóng Popup
window.closeCaptcha = function () {
    document.getElementById('captcha-modal').classList.add('hidden');
    document.getElementById('input-captcha').value = ''; 
};

// Hàm xử lý khi bấm "Xác nhận" trên Popup
window.submitCaptcha = function () {
    const code = document.getElementById('input-captcha').value;
    if (!code) return alert("Vui lòng nhập mã!");

    console.log("Mã captcha bạn nhập là:", code);

    // Gửi mã này lên API Login của bạn ở đây...

    closeCaptcha(); // Xong thì đóng
};

// KẾT NỐI VỚI NÚT ĐĂNG NHẬP CHÍNH
document.getElementById('btn-login').addEventListener('click', function () {
    // Gọi hàm mở Popup
    window.openCaptcha();
});