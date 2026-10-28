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
        const hasInfo = !!(item.address || item.taxDepartment);
        const st = String(item.status || '').toLowerCase();
        const statusClass = st.includes('đang hoạt động') ? 'bg-emerald-50 text-emerald-600'
            : (st.includes('ngừng') || st.includes('đóng') || st.includes('bỏ trốn')) ? 'bg-rose-50 text-rose-600'
                : 'bg-amber-50 text-amber-600';
        const rowHtml = `
<tr class="group/row hover:bg-emerald-50/40 transition-colors align-top">
    <td class="py-3 px-3 text-center">
        <span class="inline-flex w-6 h-6 items-center justify-center rounded-full bg-slate-100 text-[10px] font-bold text-slate-500">${realIndex + 1}</span>
    </td>
    <td class="py-3 px-2">
        <div class="flex items-start gap-2.5">
            <div class="w-9 h-9 flex-shrink-0 rounded-xl bg-gradient-to-br from-emerald-400 to-teal-600 text-white text-sm font-extrabold flex items-center justify-center shadow-sm">${escapeHtml((item.company || '?').trim().charAt(0).toUpperCase())}</div>
            <div class="min-w-0">
                <div class="text-xs font-bold text-slate-800 leading-snug">${escapeHtml(item.company || '---')}</div>
                <div class="flex flex-wrap items-center gap-1.5 mt-1">
                    <span class="text-[11px] font-semibold font-mono text-slate-600">MST: ${escapeHtml(item.mst)}</span>
                    ${item.orgType ? `<span class="text-[9px] font-semibold text-slate-400">${escapeHtml(item.orgType)}</span>` : ''}
                </div>
            </div>
        </div>
    </td>
    <td class="py-3 px-2">
        ${hasInfo ? `
        <div class="flex items-start gap-1 text-[11px] text-slate-700 leading-snug"><span class="text-[10px]">📍</span><span>${escapeHtml(item.address || '---')}</span></div>
        <div class="flex items-start gap-1 text-[10px] text-slate-500 leading-snug mt-1"><span class="text-[10px]">🏛️</span><span>${escapeHtml(item.taxDepartment || '---')}</span></div>`
                : `<button onclick="window.refreshAccountInfo('${escapeHtml(item.mst)}')" class="inline-flex items-center gap-1 px-2 py-1 rounded-lg bg-emerald-50 text-emerald-600 hover:bg-emerald-500 hover:text-white text-[10px] font-bold transition-all">🔍 Tải thông tin</button>`}
    </td>
    <td class="py-3 px-2">
        ${item.status ? `<span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold ${statusClass}"><span class="w-1.5 h-1.5 rounded-full bg-current"></span>${escapeHtml(item.status)}</span>` : '<span class="text-[10px] text-slate-300">---</span>'}
    </td>
    <td class="py-3 px-3">
        <div class="flex items-center justify-end gap-1.5">
            <button onclick="window.editAccount('${escapeHtml(item.mst)}')" title="Sửa"
                    class="w-7 h-7 flex items-center justify-center rounded-lg bg-blue-50 text-blue-500 hover:bg-blue-500 hover:text-white transition-all">
                <svg xmlns="http://www.w3.org/2000/svg" class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                </svg>
            </button>
            <button onclick="window.deleteAccount('${escapeHtml(item.mst)}')" title="Xóa"
                    class="w-7 h-7 flex items-center justify-center rounded-lg bg-rose-50 text-rose-500 hover:bg-rose-500 hover:text-white transition-all">
                <svg xmlns="http://www.w3.org/2000/svg" class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                </svg>
            </button>
        </div>
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

    // Giữ lại thông tin doanh nghiệp: ưu tiên kết quả vừa tra cứu, nếu không thì dùng dữ liệu cũ
    const oldItem = tempStorage.find(i => String(i.mst) === String(editingMst || mst)) || {};
    const looked = (window.currentMstInfo && String(window.currentMstInfo.taxID) === mst) ? window.currentMstInfo : null;
    const newAccount = {
        mst, company, pass,
        address: looked ? looked.address : oldItem.address,
        taxDepartment: looked ? looked.taxDepartment : oldItem.taxDepartment,
        status: looked ? looked.status : oldItem.status,
        orgType: looked ? looked.orgType : oldItem.orgType,
        date: oldItem.date || new Date().toLocaleDateString('vi-VN')
    };

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
    window.togglePassVisibility(true);

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

// ===== Công tắc bật/tắt giải Captcha bằng AI =====
window.setCaptchaAiToggle = function (enabled) {
    const toggle = document.getElementById('captcha-ai-toggle');
    const status = document.getElementById('captcha-ai-status');
    if (toggle) {
        toggle.checked = !!enabled;
        toggle.disabled = false;
    }
    if (status) {
        status.innerText = enabled
            ? 'Đang BẬT: Tự nhận diện và điền mã captcha'
            : 'Đang TẮT: bạn tự nhập mã captcha';
    }
};

window.toggleCaptchaAi = function (checkbox) {
    checkbox.disabled = true; // chờ Backend xác nhận rồi setCaptchaAiToggle sẽ mở lại
    if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage({
            action: 'SET_CAPTCHA_AI',
            data: { enabled: checkbox.checked }
        });
    }
};
// ===== Tra cứu thông tin MST (xinvoice) và hiển thị popup =====
var lastLookupMst = null;

function escapeHtml(v) {
    return String(v == null ? '' : v).replace(/[&<>"']/g, function (c) {
        return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
}

window.closeMstPopup = function () {
    const el = document.getElementById('mst-info-popup');
    if (el) el.remove();
};

window.applyMstInfo = function (name) {
    const companyEl = document.getElementById('company_input');
    if (companyEl) companyEl.value = name || '';
    window.closeMstPopup();
    const passEl = document.getElementById('pass_input');
    if (passEl) passEl.focus();
};

window.showMstPopup = function (info, error) {
    window.closeMstPopup();
    const rows = error ? '' : [
        ['Mã số thuế', info.taxID],
        ['Tên', info.name],
        ['Loại hình', info.orgType],
        ['Địa chỉ', info.address],
        ['Cơ quan thuế', info.taxDepartment],
        ['Trạng thái', info.status],
        ['Cập nhật lúc', info.updatedAt ? new Date(info.updatedAt).toLocaleString('vi-VN') : '']
    ].map(function (r) {
        return '<div class="flex gap-3 py-2 border-b border-slate-100 last:border-0">' +
            '<span class="w-28 flex-shrink-0 text-[10px] font-bold uppercase text-slate-400 tracking-wider pt-0.5">' + r[0] + '</span>' +
            '<span class="text-xs font-semibold text-slate-800 break-words">' + escapeHtml(r[1] || '---') + '</span></div>';
    }).join('');

    const body = error
        ? '<p class="text-sm text-rose-600 font-medium py-4">' + escapeHtml(error) + '</p>'
        : rows;
    const useBtn = error ? '' :
        '<button id="mst-popup-apply" class="px-4 py-2 bg-emerald-600 hover:bg-emerald-700 text-white rounded-xl text-[11px] font-bold uppercase tracking-wider">Sử dụng thông tin</button>';

    const html =
        '<div id="mst-info-popup" class="fixed inset-0 z-[9999] flex items-center justify-center bg-slate-900/50 backdrop-blur-sm">' +
        '<div class="bg-white rounded-2xl shadow-2xl w-[480px] max-w-[92vw] overflow-hidden">' +
        '<div class="px-5 py-3 bg-slate-50 border-b border-slate-100 flex justify-between items-center">' +
        '<span class="text-[11px] font-extrabold text-slate-700 uppercase tracking-widest">🏛️ Thông tin người nộp thuế</span>' +
        '<button id="mst-popup-x" class="text-slate-400 hover:text-rose-500 text-lg leading-none">✕</button></div>' +
        '<div class="px-5 py-2">' + body + '</div>' +
        '<div class="px-5 py-3 bg-slate-50 border-t border-slate-100 flex justify-end gap-2">' +
        '<button id="mst-popup-close" class="px-4 py-2 bg-white border border-slate-200 text-slate-600 rounded-xl text-[11px] font-bold uppercase hover:bg-slate-100">Đóng</button>' +
        useBtn + '</div></div></div>';

    document.body.insertAdjacentHTML('beforeend', html);
    document.getElementById('mst-popup-x').onclick = window.closeMstPopup;
    document.getElementById('mst-popup-close').onclick = window.closeMstPopup;
    const popup = document.getElementById('mst-info-popup');
    popup.addEventListener('click', function (e) { if (e.target === popup) window.closeMstPopup(); });
    const apply = document.getElementById('mst-popup-apply');
    if (apply) apply.onclick = function () { window.applyMstInfo(info.name); };
};

window.setMstLoading = function (loading) {
    const icon = document.getElementById('lookup-icon');
    const spin = document.getElementById('lookup-spinner');
    const hint = document.getElementById('lookup-hint');
    const btn = document.getElementById('btn-lookup-mst');
    if (icon) icon.classList.toggle('hidden', loading);
    if (spin) spin.classList.toggle('hidden', !loading);
    if (hint) hint.classList.toggle('hidden', !loading);
    if (btn) btn.disabled = loading;
};

window.lookupMst = function (value, force) {
    const mst = String(value || '').trim();
    if (!/^\d{10}(-?\d{3})?$/.test(mst)) {
        if (force) window.showMstPopup(null, 'Vui lòng nhập MST hợp lệ (10 hoặc 13 chữ số).');
        return;
    }
    if (!force && mst === lastLookupMst) return;
    lastLookupMst = mst;
    window.setMstLoading(true);

    fetch('https://api.xinvoice.vn/gdt-api/tax-payer/' + encodeURIComponent(mst))
        .then(function (res) {
            if (!res.ok) throw new Error(res.status === 429 ? 'Tra cứu quá nhanh, vui lòng thử lại sau ít giây.' : 'Không tìm thấy thông tin MST ' + mst);
            return res.json();
        })
        .then(function (info) {
            if (!info || !info.taxID) throw new Error('Không tìm thấy thông tin MST ' + mst);
            window.currentMstInfo = info;
            window.showMstPopup(info);
        })
        .catch(function (err) {
            lastLookupMst = null;
            window.showMstPopup(null, err.message || 'Lỗi tra cứu MST');
        })
        .finally(function () { window.setMstLoading(false); });
};

// Tải thông tin doanh nghiệp cho các MST đã lưu trước đây (chưa có địa chỉ...) và lưu lại
window.refreshAccountInfo = function (mst) {
    const item = tempStorage.find(i => String(i.mst) === String(mst));
    if (!item) return;
    fetch('https://api.xinvoice.vn/gdt-api/tax-payer/' + encodeURIComponent(mst))
        .then(function (res) {
            if (!res.ok) throw new Error(res.status === 429 ? 'Tra cứu quá nhanh, thử lại sau ít giây.' : 'Không tìm thấy thông tin MST ' + mst);
            return res.json();
        })
        .then(function (info) {
            if (!info || !info.taxID) throw new Error('Không tìm thấy thông tin MST ' + mst);
            item.address = info.address;
            item.taxDepartment = info.taxDepartment;
            item.status = info.status;
            item.orgType = info.orgType;
            if (window.chrome && window.chrome.webview) {
                window.chrome.webview.postMessage({ action: 'ADD_AND_SAVE_MST', data: item });
            }
            window.renderAccountList();
        })
        .catch(function (err) {
            window.showAlert('Lỗi', err.message || 'Lỗi tra cứu MST', 'error');
        });
};

// Bật/tắt hiển thị mật khẩu trong ô nhập
window.togglePassVisibility = function (forceHide) {
    const input = document.getElementById('pass_input');
    const icon = document.getElementById('pass-eye-icon');
    if (!input) return;
    const show = forceHide ? false : input.type === 'password';
    input.type = show ? 'text' : 'password';
    if (icon) icon.innerText = show ? '🙈' : '👁️';
};