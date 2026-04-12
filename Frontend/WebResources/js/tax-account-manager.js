/**
 * TaxSolution v1.0 - Account Manager Logic
 * Author: Hà Trọng Thắng
 * Mô tả: Quản lý danh sách MST tạm thời và gửi sang C#
 */

// SỬA LỖI: Dùng var và kiểm tra để không bị báo "already declared" khi nạp lại trang
var tempStorage = tempStorage || [];

// 1. Hàm thêm tài khoản vào hàng chờ
window.addToTempList = function () {
    const mstEl = document.getElementById('mst_input');
    const companyEl = document.getElementById('company_input');
    const passEl = document.getElementById('pass_input');

    if (!mstEl || !companyEl || !passEl) return;

    const mst = mstEl.value.trim();
    const company = companyEl.value.trim();
    const pass = passEl.value.trim();

    // Kiểm tra đầu vào
    if (!mst || !pass) {
        if (typeof window.showAlert === "function") {
            window.showAlert("Thiếu thông tin", "Thắng ơi, nhập đủ Mã số thuế và Mật khẩu nhé!", "error");
        } else {
            alert("Vui lòng nhập đầy đủ MST và Mật khẩu!");
        }
        mstEl.focus();
        return;
    }

    // Kiểm tra trùng MST trong danh sách hiện tại
    if (tempStorage.some(item => item.mst === mst)) {
        if (typeof window.showAlert === "function") {
            window.showAlert("Trùng lặp", "Mã số thuế này đã có trong danh sách chờ!", "error");
        }
        return;
    }

    // Thêm vào mảng (Sử dụng var tempStorage ở trên)
    tempStorage.push({
        mst: mst,
        company: company || "Doanh nghiệp chưa đặt tên",
        pass: pass
    });

    // Reset Form và focus lại ô đầu tiên để nhập tiếp cho nhanh
    mstEl.value = "";
    companyEl.value = "";
    passEl.value = "";
    mstEl.focus();

    window.renderAccountList();
};

// 2. Hàm hiển thị danh sách ra giao diện
window.renderAccountList = function () {
    const container = document.getElementById('temp_tax_list');
    const btnSave = document.getElementById('btnSaveAll');
    const countBadge = document.getElementById('count_badge');

    if (!container) return;

    if (tempStorage.length === 0) {
        if (btnSave) btnSave.classList.add('hidden');
        container.innerHTML = `
            <div class="flex flex-col items-center justify-center py-[10vh] text-slate-300">
                <p class="font-black uppercase tracking-[0.3em]" style="font-size: clamp(0.75rem, 1vw, 1.2rem);">Danh sách trống</p>
            </div>`;
        return;
    }

    if (btnSave) btnSave.classList.remove('hidden');
    if (countBadge) countBadge.innerText = tempStorage.length;

    // Render danh sách (Đảo ngược để cái mới nhất lên đầu)
    container.innerHTML = tempStorage.slice().reverse().map((item, index) => {
        // Tính lại index đúng sau khi reverse
        const realIndex = tempStorage.length - 1 - index;
        return `
            <div class="flex items-center justify-between p-5 bg-white hover:bg-blue-50/50 transition-all border-b border-slate-100 animate-in slide-in-from-right-5">
                <div class="flex items-center gap-5">
                    <span class="text-xs font-black text-slate-300">${realIndex + 1}</span>
                    <div>
                        <p class="font-black text-slate-800 tracking-wider" style="font-size: 1.1rem;">${item.mst}</p>
                        <p class="text-xs font-bold text-blue-600 uppercase tracking-tighter">${item.company}</p>
                    </div>
                </div>
                <button onclick="window.removeAccount(${realIndex})" class="w-10 h-10 flex items-center justify-center rounded-xl bg-slate-50 text-slate-300 hover:bg-red-50 hover:text-red-500 transition-all">
                    ✕
                </button>
            </div>
        `;
    }).join('');
};

// 3. Hàm xóa một tài khoản khỏi hàng chờ
window.removeAccount = function (index) {
    tempStorage.splice(index, 1);
    window.renderAccountList();
};

// 4. Hàm gửi toàn bộ dữ liệu sang WinForms (C#)
window.submitAllToCSharp = function () {
    if (tempStorage.length === 0) return;

    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage({
            action: "SAVE_TAX_ACCOUNTS",
            data: tempStorage
        });

        if (typeof window.showAlert === "function") {
            window.showAlert("Thành công", `Đã gửi ${tempStorage.length} tài khoản sang hệ thống lưu trữ!`, "success");
        } else {
            alert("Lưu thành công!");
        }

        // Xóa sạch hàng chờ sau khi đã gửi sang C# thành công
        tempStorage = [];
        window.renderAccountList();
    } else {
        if (typeof window.showAlert === "function") {
            window.showAlert("Lỗi kết nối", "Không tìm thấy môi trường WebView2!", "error");
        }
    }
};

// Tự động render lần đầu nếu có dữ liệu cũ
document.addEventListener('DOMContentLoaded', () => {
    window.renderAccountList();
});