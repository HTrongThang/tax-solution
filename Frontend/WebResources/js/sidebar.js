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
        console.log("Đang nạp: " + pagePath);
        const response = await fetch(pagePath + '?v=' + Date.now());
        const html = await response.text();

        slot.innerHTML = html;

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