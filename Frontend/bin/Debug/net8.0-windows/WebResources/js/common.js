/**
 * TaxSolution v1.0 - Common Functions
 * Author: Hà Trọng Thắng
 */

// Đảm bảo hàm được gắn vào window để gọi từ bất cứ đâu
window.showAlert = function (title, message, type = 'error') {

    // 1. Dọn dẹp Modal cũ nếu đang hiện
    const oldModal = document.getElementById('dynamicModal');
    if (oldModal) oldModal.remove();

    // 2. Cấu hình màu sắc & Icon theo loại (success/error)
    const isError = type === 'error';
    const iconColor = isError ? 'text-red-500' : 'text-emerald-500';
    const bgColor = isError ? 'bg-red-50' : 'bg-emerald-50';
    const borderColor = isError ? 'border-red-100' : 'border-emerald-100';

    // Icon SVG tương ứng
    const iconPath = isError
        ? '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/>'
        : '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>';

    // 3. Tạo cấu trúc HTML Modal
    const modalHTML = `
        <div id="dynamicModal" class="fixed inset-0 flex items-center justify-center z-[9999] animate-in fade-in duration-200">
            <div class="absolute inset-0 bg-slate-900/40 backdrop-blur-sm"></div>
            
            <div class="relative bg-white rounded-[2rem] p-8 w-[350px] shadow-2xl border border-slate-100 transform animate-in zoom-in-95 duration-200">
                <div class="flex flex-col items-center text-center">
                    <div class="w-16 h-16 ${bgColor} ${iconColor} border-2 ${borderColor} rounded-full flex items-center justify-center mb-5 shadow-inner">
                        <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            ${iconPath}
                        </svg>
                    </div>
                    
                    <h3 class="text-xl font-black text-slate-800 mb-2 tracking-tight">${title}</h3>
                    <p class="text-sm font-medium text-slate-500 mb-8 leading-relaxed">${message}</p>
                    
                    <button onclick="document.getElementById('dynamicModal').remove()" 
                            class="w-full bg-slate-800 hover:bg-slate-900 text-white py-4 rounded-2xl font-black text-sm transition-all active:scale-95 shadow-lg tracking-widest uppercase">
                        ĐÃ HIỂU
                    </button>
                </div>
            </div>
        </div>
    `;

    // 4. Chèn vào cuối body
    document.body.insertAdjacentHTML('beforeend', modalHTML);

    // 5. Tự động đóng sau 3.5 giây nếu là thông báo thành công
    if (type === 'success') {
        setTimeout(() => {
            const modal = document.getElementById('dynamicModal');
            if (modal) {
                modal.classList.add('fade-out'); // Thêm hiệu ứng biến mất nếu có CSS
                setTimeout(() => modal.remove(), 200);
            }
        }, 3500);
    }
};

/**
 * Hàm định dạng số tiền hoặc số MST (Nếu cần dùng)
 */
window.formatMST = function (mst) {
    return mst.replace(/[^0-9]/g, '');
};

window.showConfirm = function (title, message, onConfirm) {
    const oldModal = document.getElementById('confirmModal');
    if (oldModal) oldModal.remove();

    const modalHTML = `
        <div id="confirmModal" class="fixed inset-0 flex items-center justify-center z-[9999] animate-in fade-in duration-200 p-4">
            <div class="absolute inset-0 bg-slate-900/40 backdrop-blur-sm"></div>
            <div class="relative bg-white rounded-[2rem] p-4 w-full max-w-[350px] shadow-2xl border border-slate-100 transform animate-in zoom-in-95 duration-200">
                <div class="flex flex-col items-center text-center">
                    <div class="w-16 h-16 bg-rose-50 text-rose-500 border-2 border-rose-100 rounded-full flex items-center justify-center mb-5 shadow-inner">
                        <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                        </svg>
                    </div>
                    <h3 class="text-xl font-bold text-slate-800 mb-2 tracking-tight">${title}</h3>
                    <p class="text-sm text-slate-500 mb-4 leading-relaxed">${message}</p>
                    
                    <div class="flex gap-3 w-full">
                        <button onclick="document.getElementById('confirmModal').remove()" 
                                class="flex-1 bg-slate-100 hover:bg-slate-200 text-slate-600 py-3 rounded-2xl font-bold text-xs transition-all tracking-widest uppercase">
                            HỦY
                        </button>
                        <button id="btnConfirmAction" 
                                class="flex-1 bg-rose-500 hover:bg-rose-600 text-white py-3 rounded-2xl font-bold text-xs transition-all shadow-lg tracking-widest uppercase">
                            XÓA NGAY
                        </button>
                    </div>
                </div>
            </div>
        </div>
    `;

    document.body.insertAdjacentHTML('beforeend', modalHTML);

    // Gán sự kiện cho nút Xác nhận
    document.getElementById('btnConfirmAction').onclick = function () {
        document.getElementById('confirmModal').remove();
        if (typeof onConfirm === 'function') onConfirm();
    };

};