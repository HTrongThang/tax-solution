/**
 * TaxSolution v1.0 - Common Functions
 * Author: Hà Trọng Thắng
 */

// Đảm bảo hàm được gắn vào window để gọi từ bất cứ đâu
window.showAlert = function (title, message, type = 'error') {

    // 1. Dọn dẹp Modal cũ nếu đang hiện
    const oldModal = document.getElementById('dynamicModal');
    if (oldModal) oldModal.remove();

    // 2. Cấu hình màu sắc & Icon theo loại
    let iconColor = 'text-red-500';
    let bgColor = 'bg-red-50';
    let borderColor = 'border-red-100';
    let btnStyle = 'bg-slate-900 hover:bg-slate-800 text-white';
    let btnText = 'ĐÃ HIỂU';
    let iconSvg = '';

    if (type === 'success') {
        iconColor = 'text-emerald-500';
        bgColor = 'bg-emerald-50';
        borderColor = 'border-emerald-100';
        btnStyle = 'bg-emerald-600 hover:bg-emerald-700 text-white shadow-emerald-500/25';
        btnText = 'TUYỆT VỜI';
        iconSvg = `
            <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7"/>
            </svg>
        `;
    } else if (type === 'warning') {
        iconColor = 'text-amber-500';
        bgColor = 'bg-amber-50';
        borderColor = 'border-amber-100';
        btnStyle = 'bg-amber-500 hover:bg-amber-600 text-white shadow-amber-500/20';
        btnText = 'THỬ LẠI';
        iconSvg = `
            <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/>
            </svg>
        `;
    } else if (type === 'info') {
        iconColor = 'text-blue-500';
        bgColor = 'bg-blue-50';
        borderColor = 'border-blue-100';
        btnStyle = 'bg-slate-900 hover:bg-blue-600 text-white shadow-blue-500/20';
        btnText = 'ĐÃ HIỂU';
        iconSvg = `
            <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
            </svg>
        `;
    } else {
        // error
        iconColor = 'text-rose-500';
        bgColor = 'bg-rose-50';
        borderColor = 'border-rose-100';
        btnStyle = 'bg-slate-900 hover:bg-rose-600 text-white shadow-rose-500/20';
        btnText = 'ĐÃ HIỂU';
        iconSvg = `
            <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
            </svg>
        `;
    }

    // 3. Tạo cấu trúc HTML Modal
    const modalHTML = `
        <div id="dynamicModal" class="fixed inset-0 flex items-center justify-center z-[9999] animate-in fade-in duration-200 p-4">
            <div id="dynamicModalBackdrop" class="absolute inset-0 bg-slate-900/50 backdrop-blur-sm transition-opacity"></div>
            
            <div class="relative bg-white rounded-[2rem] p-7 w-full max-w-[360px] shadow-2xl border border-slate-100 transform animate-in zoom-in-95 duration-200">
                <div class="flex flex-col items-center text-center">
                    <div class="w-16 h-16 ${bgColor} ${iconColor} border-2 ${borderColor} rounded-2xl flex items-center justify-center mb-5 shadow-inner">
                        ${iconSvg}
                    </div>
                    
                    <h3 class="text-xl font-black text-slate-800 mb-2 tracking-tight">${title}</h3>
                    <p class="text-sm font-medium text-slate-500 mb-6 leading-relaxed">${message}</p>
                    
                    <button id="btnDismissAlert" 
                            class="w-full ${btnStyle} py-3.5 rounded-xl font-bold text-xs transition-all active:scale-95 shadow-md tracking-wider uppercase cursor-pointer">
                        ${btnText}
                    </button>
                </div>
            </div>
        </div>
    `;

    // 4. Chèn vào cuối body
    document.body.insertAdjacentHTML('beforeend', modalHTML);

    const closeAlert = () => {
        const modal = document.getElementById('dynamicModal');
        if (modal) modal.remove();
        document.removeEventListener('keydown', handleKey);
    };

    const handleKey = (e) => {
        if (e.key === 'Escape' || e.key === 'Enter') {
            closeAlert();
        }
    };
    document.addEventListener('keydown', handleKey);

    const dismissBtn = document.getElementById('btnDismissAlert');
    if (dismissBtn) dismissBtn.onclick = closeAlert;

    const backdrop = document.getElementById('dynamicModalBackdrop');
    if (backdrop) backdrop.onclick = closeAlert;

    // 5. Tự động đóng sau 3 giây nếu là thông báo thành công
    if (type === 'success') {
        setTimeout(() => {
            const modal = document.getElementById('dynamicModal');
            if (modal) {
                modal.classList.add('transition-opacity', 'duration-300', 'opacity-0');
                setTimeout(() => modal.remove(), 300);
            }
        }, 2800);
    }
};

/**
 * Hàm định dạng số tiền hoặc số MST (Nếu cần dùng)
 */
window.formatMST = function (mst) {
    return mst.replace(/[^0-9]/g, '');
};

window.showConfirm = function (optsOrTitle, message, onConfirmLegacy) {
    const oldModal = document.getElementById('confirmModal');
    if (oldModal) oldModal.remove();

    let opts = {};
    if (typeof optsOrTitle === 'object' && optsOrTitle !== null) {
        opts = optsOrTitle;
    } else {
        opts = {
            title: optsOrTitle || "Xác nhận",
            message: message || "",
            onConfirm: onConfirmLegacy,
            type: "delete"
        };
    }

    const title = opts.title || "Xác nhận";
    const msg = opts.message || "Bạn có chắc chắn muốn tiếp tục thao tác này?";
    const type = opts.type || "warning";
    const cancelText = opts.cancelText || "HỦY";
    const onConfirm = opts.onConfirm;
    const onCancel = opts.onCancel;

    // Cấu hình Icon và Style theo loại
    let iconSvg = '';
    let iconBg = 'bg-rose-50 text-rose-600 border-rose-100';
    let btnStyle = 'bg-rose-600 hover:bg-rose-700 shadow-rose-500/20';
    let defaultConfirmText = "XÁC NHẬN";

    if (type === 'logout') {
        iconBg = 'bg-rose-50 text-rose-600 border-rose-100';
        btnStyle = 'bg-rose-600 hover:bg-rose-700 text-white shadow-rose-500/25';
        defaultConfirmText = "ĐĂNG XUẤT";
        iconSvg = `
            <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"/>
            </svg>
        `;
    } else if (type === 'delete') {
        iconBg = 'bg-rose-50 text-rose-500 border-rose-100';
        btnStyle = 'bg-rose-500 hover:bg-rose-600 text-white shadow-rose-500/20';
        defaultConfirmText = "XÓA NGAY";
        iconSvg = `
            <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
            </svg>
        `;
    } else {
        iconBg = 'bg-amber-50 text-amber-600 border-amber-100';
        btnStyle = 'bg-amber-500 hover:bg-amber-600 text-white shadow-amber-500/20';
        defaultConfirmText = "XÁC NHẬN";
        iconSvg = `
            <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/>
            </svg>
        `;
    }

    const confirmText = opts.confirmText || defaultConfirmText;

    const modalHTML = `
        <div id="confirmModal" class="fixed inset-0 flex items-center justify-center z-[9999] animate-in fade-in duration-200 p-4">
            <div id="confirmModalBackdrop" class="absolute inset-0 bg-slate-900/50 backdrop-blur-sm transition-opacity"></div>
            <div class="relative bg-white rounded-[2rem] p-7 w-full max-w-[380px] shadow-2xl border border-slate-100 transform animate-in zoom-in-95 duration-200">
                <div class="flex flex-col items-center text-center">
                    <div class="w-16 h-16 ${iconBg} border-2 rounded-2xl flex items-center justify-center mb-5 shadow-inner">
                        ${iconSvg}
                    </div>
                    <h3 class="text-xl font-black text-slate-800 mb-2 tracking-tight">${title}</h3>
                    <p class="text-sm font-medium text-slate-500 mb-6 leading-relaxed">${msg}</p>
                    
                    <div class="flex gap-3 w-full">
                        <button id="btnCancelAction" 
                                class="flex-1 bg-slate-100 hover:bg-slate-200 text-slate-700 py-3.5 rounded-xl font-bold text-xs transition-all active:scale-95 tracking-wider uppercase cursor-pointer">
                            ${cancelText}
                        </button>
                        <button id="btnConfirmAction" 
                                class="flex-1 ${btnStyle} py-3.5 rounded-xl font-bold text-xs transition-all active:scale-95 shadow-md tracking-wider uppercase cursor-pointer">
                            ${confirmText}
                        </button>
                    </div>
                </div>
            </div>
        </div>
    `;

    document.body.insertAdjacentHTML('beforeend', modalHTML);

    const closeModal = () => {
        const modal = document.getElementById('confirmModal');
        if (modal) modal.remove();
        document.removeEventListener('keydown', handleKey);
    };

    const handleKey = (e) => {
        if (e.key === 'Escape') {
            closeModal();
            if (typeof onCancel === 'function') onCancel();
        }
    };
    document.addEventListener('keydown', handleKey);

    document.getElementById('confirmModalBackdrop').onclick = () => {
        closeModal();
        if (typeof onCancel === 'function') onCancel();
    };

    document.getElementById('btnCancelAction').onclick = () => {
        closeModal();
        if (typeof onCancel === 'function') onCancel();
    };

    document.getElementById('btnConfirmAction').onclick = () => {
        closeModal();
        if (typeof onConfirm === 'function') onConfirm();
    };
};