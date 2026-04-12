/**
 * TaxSolution v1.0 - Account Manager Logic (Updated with Custom Alert)
 * Author: Hà Trọng Thắng
 */

let tempStorage = [];

// 1. Hàm thêm tài khoản
window.addToTempList = function () {
    const mstEl = document.getElementById('mst_input');
    const companyEl = document.getElementById('company_input');
    const passEl = document.getElementById('pass_input');

    if (!mstEl || !companyEl || !passEl) return;

    const mst = mstEl.value.trim();
    const company = companyEl.value.trim();
    const pass = passEl.value.trim();

    // KIỂM TRA HỢP LỆ (Dùng showAlert thay cho alert)
    if (!mst || !pass) {
        showAlert("Thiếu thông tin", "Thắng ơi, vui lòng nhập đầy đủ Mã số thuế và Mật khẩu nhé!", "error");
        mstEl.focus();
        return;
    }

    if (tempStorage.some(item => item.mst === mst)) {
        showAlert("Trùng lặp", "Mã số thuế này đã nằm trong danh sách chờ rồi!", "error");
        return;
    }

    // Thêm vào mảng tạm
    tempStorage.push({
        mst: mst,
        company: company || "Doanh nghiệp chưa đặt tên",
        pass: pass
    });

    // Reset Form
    mstEl.value = ""; companyEl.value = ""; passEl.value = "";
    mstEl.focus();

    renderAccountList();
};

// 2. Hàm hiển thị danh sách
window.renderAccountList = function () {
    const container = document.getElementById('temp_tax_list');
    const btnSave = document.getElementById('btnSaveAll');
    const countBadge = document.getElementById('count_badge');

    if (!container) return;

    if (tempStorage.length === 0) {
        if (btnSave) btnSave.classList.add('hidden');
        container.innerHTML = `<div class="flex flex-col items-center justify-center py-[10vh] text-slate-300"><p class="font-black uppercase tracking-[0.3em]">Danh sách trống</p></div>`;
        return;
    }

    if (btnSave) btnSave.classList.remove('hidden');
    if (countBadge) countBadge.innerText = tempStorage.length;

    container.innerHTML = tempStorage.map((item, index) => `
        <div class="flex items-center justify-between p-5 bg-white hover:bg-blue-50/50 transition-all border-b border-slate-100 animate-in slide-in-from-right-5">
            <div class="flex items-center gap-5">
                <span class="text-xs font-black text-slate-300">0${index + 1}</span>
                <div>
                    <p class="font-black text-slate-800 tracking-wider" style="font-size: 1.1rem;">${item.mst}</p>
                    <p class="text-xs font-bold text-blue-600 uppercase tracking-tighter">${item.company}</p>
                </div>
            </div>
            <button onclick="removeAccount(${index})" class="text-slate-300 hover:text-red-500 p-2 text-xl transition-colors">✕</button>
        </div>
    `).reverse().join('');
};

// 3. Hàm xóa tài khoản
window.removeAccount = function (index) {
    // Thắng có thể dùng showAlert kiểu confirm nếu muốn, 
    // nhưng tạm thời xóa nhanh thì cứ giữ logic đơn giản:
    tempStorage.splice(index, 1);
    renderAccountList();
};

// 4. Hàm gửi dữ liệu sang C#
window.submitAllToCSharp = function () {
    if (tempStorage.length === 0) return;

    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage({
            action: "SAVE_TAX_ACCOUNTS",
            data: tempStorage
        });

        // DÙNG SHOWALERT CHO THÀNH CÔNG
        showAlert("Thành công", `Đã lưu ${tempStorage.length} tài khoản vào hệ thống TaxSolution!`, "success");

        tempStorage = [];
        renderAccountList();
    } else {
        showAlert("Lỗi hệ thống", "Không tìm thấy môi trường WebView2 để lưu file!", "error");
    }
};