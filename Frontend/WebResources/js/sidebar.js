// 1. Mở Modal
window.openLogoutModal = function () {
    const modal = document.getElementById('logoutModal');
    if (modal) {
        modal.classList.remove('hidden');
        modal.classList.add('flex');
    }
}


// 2. Đóng Modal
window.closeLogoutModal = function () {
    const modal = document.getElementById('logoutModal');
    if (modal) {
        modal.classList.add('hidden');
        modal.classList.remove('flex');
    }
}

// 3. Xác nhận thoát
window.confirmLogout = function () {
    if (window.chrome && window.chrome.webview) {
        // Xóa sạch LocalStorage trước khi thoát cho an toàn
        localStorage.clear();
        window.chrome.webview.postMessage("LOGOUT");
    }
}

let isFetching = false;

window.loadPage = async function (pagePath, element) {
    if (isFetching) {
        console.warn("Hệ thống đang bận nạp trang khác, vui lòng đợi!");
        return;
    }

    const slot = document.getElementById('main-content-slot');
    if (!slot) return;

    isFetching = true; // Khóa lại
    slot.style.opacity = '0.5';

    try {
        const response = await fetch(pagePath + '?v=' + Date.now());
        const html = await response.text();

        slot.innerHTML = html;

        // Nếu trang vừa nạp xong là trang quản lý tài khoản/cấu hình
        if (pagePath.includes('settings_content.html') || pagePath.includes('crawler_table.html')) {
            if (window.chrome && window.chrome.webview) {
                // Gửi tín hiệu sang C# để xin dữ liệu JSON
                window.chrome.webview.postMessage({ action: "GET_SETTINGS_DATA" });
                console.log("Hệ thống: Đã gửi yêu cầu lấy dữ liệu cho trang: " + pagePath);
            }
        }
        // ------------------------------

        if (element) {
            document.querySelectorAll('.nav-link').forEach(link => {
                link.classList.remove('bg-emerald-600/20', 'text-emerald-300', 'border-emerald-500/20');
                link.classList.add('text-slate-400');
            });
            element.classList.add('bg-emerald-600/20', 'text-emerald-300', 'border-emerald-500/20');
        }

        setTimeout(() => {
            slot.style.opacity = '1';
            isFetching = false;
        }, 300);

    } catch (err) {
        console.error(err);
        isFetching = false;
        slot.style.opacity = '1';
    }
}
function toggleSidebar() {
    const sidebarSlot = document.getElementById('sidebar-slot');
    const sidebarDiv = document.getElementById('sidebar');
    const wrapper = document.getElementById('main-wrapper');
    const logoContainer = document.getElementById('logo-container');
    const iconSvg = document.getElementById('toggle-icon-svg');
    const texts = document.querySelectorAll('.sidebar-text');

    // Kiểm tra xem có đang thu gọn hay không
    const isCollapsed = sidebarSlot.classList.contains('w-24');

    if (!isCollapsed) {
        // --- THỰC HIỆN THU GỌN ---
        sidebarSlot.style.width = '6rem'; // w-24
        sidebarDiv.style.width = '6rem';
        wrapper.style.marginLeft = '6rem';

        sidebarSlot.classList.add('w-24');
        sidebarSlot.classList.remove('w-[17rem]');

        logoContainer.classList.add('justify-center');
        texts.forEach(t => t.classList.add('hidden'));

        // Đổi sang icon mũi tên hướng sang phải (Quay lại)
        iconSvg.innerHTML = '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 5l7 7-7 7M5 5l7 7-7 7" />';

        document.querySelectorAll('.nav-link, button').forEach(el => {
            el.classList.add('justify-center');
            el.classList.remove('gap-4');
        });
    } else {
        // --- THỰC HIỆN MỞ RỘNG (QUAY LẠI) ---
        sidebarSlot.style.width = '17rem';
        sidebarDiv.style.width = '17rem';
        wrapper.style.marginLeft = '17rem';

        sidebarSlot.classList.remove('w-24');
        sidebarSlot.classList.add('w-[17rem]');

        logoContainer.classList.remove('justify-center');
        texts.forEach(t => t.classList.remove('hidden'));

        // Đổi về icon mũi tên hướng sang trái
        iconSvg.innerHTML = '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 19l-7-7 7-7m8 14l-7-7 7-7" />';

        document.querySelectorAll('.nav-link, button').forEach(el => {
            el.classList.remove('justify-center');
            el.classList.add('gap-4');
        });
    }
}