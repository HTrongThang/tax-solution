/**
 * TaxSolution v1.1 - Account Manager Logic
 * Author: Hà Trọng Thắng
 */
var tempStorage = [];
var editingMst = null; // MST đang được sửa (null = chế độ thêm mới)

window.loadInitialData = function (jsonString) {
    try {
        // Parse dữ liệu
        const data = typeof jsonString === 'string' ? JSON.parse(jsonString) : jsonString;

        if (Array.isArray(data)) {
            tempStorage = data; // Lưu vào mảng dùng chung

            if (typeof window.renderCompanyDropdown === 'function') {
                window.renderCompanyDropdown();
            }


            // Kiểm tra xem hàm vẽ đã "lộ diện" chưa
            if (typeof window.renderAccountList === 'function') {
                window.renderAccountList();
                console.log("Hệ thống: Đã nhận dữ liệu và vẽ giao diện thành công!");
            } else {
                // Nếu chạy vào đây nghĩa là Thắng chưa dùng 'window.renderAccountList' 
                // ở file component rồi!
                console.error("Lỗi: Không tìm thấy hàm window.renderAccountList trên giao diện.");
            }
        }
    } catch (e) {
        console.error("Lỗi xử lý JSON:", e);
    }
};
/**
 * Hàm vẽ danh sách MST lên giao diện
 * Được gắn vào 'window' để các file script khác (như manager) có thể gọi
 */
window.renderAccountList = function () {
    const container = document.getElementById('temp_tax_list');
    if (!container) return;

    container.innerHTML = "";

    if (!window.tempStorage || window.tempStorage.length === 0) {
        container.innerHTML = `<tr><td colspan="5" class="py-10 text-center text-slate-300 text-xs italic">Chưa có dữ liệu hàng chờ...</td></tr>`;
        return;
    }

    [...window.tempStorage].reverse().forEach((item, index) => {
        const realIndex = window.tempStorage.length - 1 - index;
        const dateAdded = item.date || new Date().toLocaleDateString('vi-VN');
        const rowHtml = `
<tr class="border-b border-slate-100 last:border-0 hover:bg-slate-50/50 transition-colors">
    <td class="py-1.5 px-3 text-[10px] text-slate-500 text-center font-mono">${realIndex + 1}</td>
    
    <td class="py-1.5 px-2 text-xs text-black font-medium font-mono">${item.mst}</td>
    
    <td class="py-1.5 px-2 text-xs text-slate-800">${item.company || '---'}</td>
    
    <td class="py-1.5 px-2 text-[10px] text-slate-500 font-mono">${dateAdded}</td>
    
    <td class="py-1.5 px-3 text-right flex items-center justify-end gap-1">
        <button onclick="window.editAccount('${item.mst}')" 
                class="text-blue-500 hover:text-blue-700 transition-all p-0.5"
                title="Sửa">
            <svg xmlns="http://www.w3.org/2000/svg" class="h-3.5 w-3.5 inline-block" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" 
                      d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
            </svg>
        </button>
        <button onclick="window.deleteAccount('${item.mst}')" 
                class="text-rose-500 hover:text-rose-700 transition-all p-0.5"
                title="Xóa">
            <svg xmlns="http://www.w3.org/2000/svg" class="h-3.5 w-3.5 inline-block" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" 
                      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
        </button>
    </td>
</tr>`;
        container.insertAdjacentHTML('beforeend', rowHtml);
    });

    const badge = document.getElementById('count_badge');
    if (badge) badge.innerText = window.tempStorage.length;
};

// --- 2. HÀM THÊM MỚI / CẬP NHẬT ---
window.addToTempList = function () {
    const mstEl = document.getElementById('mst_input');
    const companyEl = document.getElementById('company_input');
    const passEl = document.getElementById('pass_input');
    const submitBtn = document.getElementById('btn-submit-mst');

    if (!mstEl || !companyEl || !passEl) return;

    const mst = mstEl.value.trim();
    const company = companyEl.value.trim() || "Doanh nghiệp chưa đặt tên";
    const pass = passEl.value.trim();

    if (!mst || !pass) {
        window.showAlert("Lỗi", "Bạn ơi, nhập đủ Mã số thuế và Mật khẩu nhé!", "error");
        mstEl.focus();
        return;
    }

    // Kiểm tra trùng (chỉ khi thêm mới, không phải sửa)
    if (!editingMst && tempStorage.some(item => item.mst === mst)) {
        window.showAlert("Trùng lặp", "Mã số thuế này đã tồn tại trong hệ thống!", "error");
        return;
    }

    // Disable nút tránh click đúp
    if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.innerHTML = '<span>⏳ Đang xử lý...</span>';
    }

    const newAccount = { mst, company, pass };

    if (window.chrome && window.chrome.webview) {
        // Nếu đang sửa → xóa bản cũ khỏi tempStorage
        if (editingMst) {
            const oldIndex = tempStorage.findIndex(item => item.mst === editingMst);
            if (oldIndex !== -1) tempStorage.splice(oldIndex, 1);
        }

        window.chrome.webview.postMessage({
            action: "ADD_AND_SAVE_MST",
            data: newAccount
        });

        tempStorage.push(newAccount);
        window.renderAccountList();

        const actionLabel = editingMst ? 'Cập nhật' : 'Thêm';
        window.showAlert('Thành công', `${actionLabel} MST ${mst} thành công!`, 'success');

        // Reset form + thoát chế độ sửa
        window.cancelEdit();
    }

    // Re-enable nút
    setTimeout(() => {
        if (submitBtn) {
            submitBtn.disabled = false;
            submitBtn.innerHTML = '<span>THÊM VÀO HÀNG CHỜ</span> <span class="transition-transform group-hover:translate-x-1">🚀</span>';
        }
    }, 500);
};

// --- HÀM SỬA TÀI KHOẢN ---
window.editAccount = function (mst) {
    const item = tempStorage.find(i => String(i.mst) === String(mst));
    if (!item) return;

    // Đổ dữ liệu vào form
    const mstEl = document.getElementById('mst_input');
    const companyEl = document.getElementById('company_input');
    const passEl = document.getElementById('pass_input');
    const submitBtn = document.getElementById('btn-submit-mst');

    if (mstEl) mstEl.value = item.mst;
    if (companyEl) companyEl.value = item.company || '';
    if (passEl) passEl.value = item.pass || '';

    // Đánh dấu đang sửa
    editingMst = mst;

    // Đổi giao diện nút thành "CẬP NHẬT"
    if (submitBtn) {
        submitBtn.innerHTML = '<span>CẬP NHẬT</span> <span>✏️</span>';
        submitBtn.classList.remove('bg-blue-600', 'hover:bg-blue-700', 'shadow-blue-500/20', 'hover:shadow-blue-500/40');
        submitBtn.classList.add('bg-amber-500', 'hover:bg-amber-600', 'shadow-amber-500/20', 'hover:shadow-amber-500/40');
    }

    // Hiện nút Hủy sửa
    const cancelBtn = document.getElementById('btn-cancel-edit');
    if (cancelBtn) cancelBtn.classList.remove('hidden');

    // Focus vào ô tên công ty
    if (companyEl) companyEl.focus();
};

// --- HÀM HỦY SỬA ---
window.cancelEdit = function () {
    editingMst = null;

    // Xóa form
    const mstEl = document.getElementById('mst_input');
    const companyEl = document.getElementById('company_input');
    const passEl = document.getElementById('pass_input');
    const submitBtn = document.getElementById('btn-submit-mst');
    const cancelBtn = document.getElementById('btn-cancel-edit');

    if (mstEl) mstEl.value = '';
    if (companyEl) companyEl.value = '';
    if (passEl) passEl.value = '';

    // Reset nút về "THÊM"
    if (submitBtn) {
        submitBtn.disabled = false;
        submitBtn.innerHTML = '<span>THÊM VÀO HÀNG CHỜ</span> <span class="transition-transform group-hover:translate-x-1">🚀</span>';
        submitBtn.classList.remove('bg-amber-500', 'hover:bg-amber-600', 'shadow-amber-500/20', 'hover:shadow-amber-500/40');
        submitBtn.classList.add('bg-blue-600', 'hover:bg-blue-700', 'shadow-blue-500/20', 'hover:shadow-blue-500/40');
    }

    // Ẩn nút Hủy
    if (cancelBtn) cancelBtn.classList.add('hidden');

    if (mstEl) mstEl.focus();
};

/**
 * HÀM XÓA TÀI KHOẢN
 * Áp dụng logic tìm theo MST để đảm bảo xóa đúng dữ liệu
 */
window.deleteAccount = function (mst) {
    const storage = window.tempStorage || [];
    const realIndex = storage.findIndex(item => String(item.mst) === String(mst));

    if (realIndex === -1) return;

    // Thay confirm bằng showConfirm xịn xò
    window.showConfirm(
        "Xác nhận xóa",
        `Bạn có chắc muốn loại bỏ MST ${mst} khỏi danh sách không?`,
        function () {
            // Tất cả logic xóa đặt vào trong callback này
            storage.splice(realIndex, 1);

            if (window.chrome && window.chrome.webview) {
                window.chrome.webview.postMessage({
                    action: "DELETE_AND_SAVE_MST",
                    mst: String(mst)
                });
            }

            window.renderAccountList();

            // Hiện thêm một cái alert nhỏ báo thành công cho chuyên nghiệp
            window.showAlert("Đã xóa", `Đã loại bỏ MST ${mst} thành công.`, "success");
        }
    );
};