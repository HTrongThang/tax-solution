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
        <option value="${company.mst}">
            ${company.company || company.mst}
        </option>
    `).join('');

    selectCompany.innerHTML = html;
    console.log("Hệ thống: Đã cập nhật danh sách công ty!");
};
// Hàm 1: Trung gian - Gọi khi bấm nút
window.handleLoginClick = function (btn) {
    debugger;
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
    // Báo cho C# biết là Thắng muốn Logout
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

    // 1. Kiểm tra đầu vào bằng hàm showAlert của Thắng
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

    const baseClass = "ml-2 text-[10px] font-bold px-3 py-1 rounded-md transition-all uppercase active:scale-95 disabled:opacity-50 disabled:cursor-not-allowed ";
    const emeraldStyle = "bg-emerald-50 text-emerald-600 hover:bg-emerald-600 hover:text-white";
    const roseStyle = "bg-rose-50 text-rose-600 hover:bg-rose-600 hover:text-white";

    if (isLoggedIn) {
        btn.innerText = 'Đăng xuất';
        btn.className = baseClass + roseStyle;
        btn.dataset.state = "logged-in";
    } else {
        btn.innerText = 'Đăng nhập';
        btn.className = baseClass + emeraldStyle;
        btn.dataset.state = "logged-out";
    }
};

// Hàm trung gian điều hướng sự kiện
window.handleAuthAction = function (btn) {
    if (btn.dataset.state === "logged-in") {
        return handleLogoutClick(); // Gọi hàm Logout của bạn
    }

    btn.disabled = true;
    btn.innerText = "Đang kết nối...";

    handleLoginClick(btn); // Gửi sang C#
};
window.handleLogoutClick = function () {
    if (confirm("Thắng có chắc chắn muốn đăng xuất tài khoản thuế này không?")) {

        // 2. Gửi tin nhắn sang WinForms (C#)
        if (window.chrome && window.chrome.webview) {
            window.chrome.webview.postMessage({
                action: "REQUEST_LOGOUT"
            });
            console.log("Hệ thống: Đã gửi yêu cầu Logout sang C#");
        } else {
            console.error("Lỗi: Không tìm thấy kết nối WebView2!");
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
        const start = new Date(dateFrom.value);
        const end = new Date(dateTo.value);

        if (!dateFrom.value || !dateTo.value) return;

        // Tính số ngày chênh lệch
        const diffTime = end - start;
        const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

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

        // TRƯỜNG HỢP 2: Khoảng cách vượt quá 31 ngày (1 tháng)
        if (diffDays > 31) {
            window.showAlert('Giới hạn hệ thống', 'Hệ thống chỉ hỗ trợ tra cứu tối đa trong 1 tháng (31 ngày)!', 'info');

            if (e.target.id === 'date-from') {
                // Đổi "Từ ngày" -> Tự đẩy "Đến ngày" về đúng (Từ ngày + 30)
                const newEnd = new Date(start);
                newEnd.setDate(start.getDate() + 30);

                // Nếu ngày mới vượt quá hôm nay thì lấy ngày hôm nay
                const finalEnd = newEnd > new Date() ? new Date() : newEnd;
                dateTo.value = finalEnd.toISOString().split('T')[0];
            } else {
                // Đổi "Đến ngày" -> Tự kéo "Từ ngày" về (Đến ngày - 30)
                const newStart = new Date(end);
                newStart.setDate(end.getDate() - 30);
                dateFrom.value = newStart.toISOString().split('T')[0];
            }
        }
    }

    // Gán sự kiện thay đổi cho cả 2 ô
    dateFrom.addEventListener('change', handleDateValidation);
    dateTo.addEventListener('change', handleDateValidation);
});

// Hàm để C# bốc dữ liệu gửi sang Go
window.getFilterParams = function () {
    return {
        fromDate: document.getElementById('date-from').value,
        toDate: document.getElementById('date-to').value
    };
};
window.handleSearchClick = function () {
    // 1. Lấy dữ liệu từ hàm bạn đã viết
    debugger;
    const params = window.getFilterParams();

    // 2. Kiểm tra nhanh (Double check lần cuối cho an toàn)
    if (!params.fromDate || !params.toDate) {
        return window.showAlert('Thông báo', 'Vui lòng chọn đầy đủ khoảng thời gian!', 'warning');
    }

    const start = new Date(params.fromDate);
    const end = new Date(params.toDate);
    const diffDays = Math.ceil((end - start) / (1000 * 60 * 60 * 24));

    if (diffDays < 0) {
        return window.showAlert('Lỗi logic', 'Ngày kết thúc không được nhỏ hơn ngày bắt đầu!', 'error');
    }

    if (diffDays > 31) {
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
    } else {
        window.showAlert('Lỗi', 'Không tìm thấy kết nối ứng dụng (WebView2)!', 'error');
    }
};