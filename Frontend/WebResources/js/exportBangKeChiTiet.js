/**
 * ============================================================
 * XUẤT BẢNG KÊ CHI TIẾT HÀNG HÓA RA FILE EXCEL
 * Chuẩn 16 cột theo nghiệp vụ kế toán thuế (từng dòng hàng hóa)
 *
 * Gắn vào nút dropdown: onclick="exportBangKeChiTiet()"
 * ============================================================
 */
(function (window) {
    'use strict';

    /**
     * Lấy ngày bắt đầu/kết thúc từ bộ lọc giao diện (dd/MM/yyyy)
     */
    function getDateRange() {
        var result = { fromDisplay: '', toDisplay: '' };

        try {
            if (typeof window.getFilterParams === 'function') {
                var params = window.getFilterParams();
                result.fromDisplay = params.fromDate || '';
                result.toDisplay = params.toDate || '';

                if (!result.fromDisplay && params.rawFrom) {
                    var m = params.rawFrom.match(/^(\d{4})-(\d{2})-(\d{2})$/);
                    if (m) result.fromDisplay = m[3] + '/' + m[2] + '/' + m[1];
                }
                if (!result.toDisplay && params.rawTo) {
                    var m2 = params.rawTo.match(/^(\d{4})-(\d{2})-(\d{2})$/);
                    if (m2) result.toDisplay = m2[3] + '/' + m2[2] + '/' + m2[1];
                }
            }
        } catch (e) {
            console.warn('[ExportBKCT] Không lấy được ngày từ bộ lọc:', e);
        }

        // Fallback: lấy trực tiếp từ DOM
        if (!result.fromDisplay) {
            var fromEl = document.getElementById('date-from');
            if (fromEl && fromEl.value) {
                var m3 = fromEl.value.match(/^(\d{4})-(\d{2})-(\d{2})$/);
                if (m3) result.fromDisplay = m3[3] + '/' + m3[2] + '/' + m3[1];
            }
        }
        if (!result.toDisplay) {
            var toEl = document.getElementById('date-to');
            if (toEl && toEl.value) {
                var m4 = toEl.value.match(/^(\d{4})-(\d{2})-(\d{2})$/);
                if (m4) result.toDisplay = m4[3] + '/' + m4[2] + '/' + m4[1];
            }
        }

        return result;
    }

    /**
     * Lấy MST công ty đang chọn
     */
    function getCurrentMst() {
        return window._selectedCompanyMst
            || window._loggedInMst
            || (window.InvoiceRamManager && window.InvoiceRamManager._currentMst)
            || 'MST';
    }

    /**
     * HÀM CHÍNH: XUẤT BẢNG KÊ CHI TIẾT HÀNG HÓA
     */
    function exportBangKeChiTiet() {
        try {
            // 1. Kiểm tra WebView2
            if (!(window.chrome && window.chrome.webview)) {
                window.showAlert
                    ? window.showAlert('Lỗi', 'Không tìm thấy kết nối WebView2! Hãy chạy ứng dụng từ chương trình chính.', 'error')
                    : alert('WebView2 chưa sẵn sàng!');
                return;
            }

            // 2. Lấy dữ liệu từ RAM
            // Ưu tiên: window._parsedXmlInvoices (đã bóc tách chi tiết XML có HoaDons)
            var parsedXmls = window._parsedXmlInvoices;
            var generalInvoices = (window.getInvoiceRAM ? window.getInvoiceRAM() : null)
                || (window.InvoiceRamManager ? window.InvoiceRamManager.get() : null)
                || window._currentInvoicesRAM
                || window._invoiceCache
                || [];

            if ((!parsedXmls || parsedXmls.length === 0) && (!generalInvoices || generalInvoices.length === 0)) {
                window.showAlert
                    ? window.showAlert('Thông báo', 'Không có dữ liệu hóa đơn trong bộ nhớ để xuất!\nVui lòng tra cứu hóa đơn trước.', 'warning')
                    : alert('Không có dữ liệu hóa đơn!');
                return;
            }

            // Kiểm tra xem đã có bóc tách XML chưa
            var hasDetails = false;
            if (Array.isArray(parsedXmls) && parsedXmls.length > 0) {
                hasDetails = parsedXmls.some(function (inv) {
                    return Array.isArray(inv.HoaDons) && inv.HoaDons.length > 0;
                });
            }
            if (!hasDetails && Array.isArray(generalInvoices)) {
                hasDetails = generalInvoices.some(function (inv) {
                    return (Array.isArray(inv.HoaDons) && inv.HoaDons.length > 0)
                        || (Array.isArray(inv.hoaDons) && inv.hoaDons.length > 0);
                });
            }

            if (!hasDetails) {
                if (typeof window.showAlert === 'function') {
                    window.showAlert(
                        'Chưa tải chi tiết hóa đơn',
                        'Bảng kê chi tiết hàng hóa yêu cầu dữ liệu từng dòng hàng (file XML). Vui lòng bấm nút "Tải chi tiết" trên thanh công cụ trước khi xuất báo cáo!',
                        'warning'
                    );
                } else {
                    alert('Chưa có dữ liệu chi tiết! Vui lòng bấm nút "Tải chi tiết" trước khi xuất Bảng kê chi tiết hàng hóa.');
                }
                return;
            }

            // Chọn nguồn dữ liệu gửi xuống C#
            var invoicesToSend = (parsedXmls && parsedXmls.length > 0) ? parsedXmls : generalInvoices;

            var isMuaVao = (window._currentInvoiceType || 'ban_ra') === 'mua_vao';
            var dateRange = getDateRange();
            var mst = getCurrentMst();

            console.log('[ExportBKCT] Gửi ' + invoicesToSend.length + ' hóa đơn xuống C# để xuất Bảng kê chi tiết...');

            var requestId = 'excel_ct_' + Math.random().toString(36).slice(2);
            window._pendingWebviewRequests = window._pendingWebviewRequests || {};

            // 5. Đăng ký handler nhận phản hồi từ C#
            window._pendingWebviewRequests[requestId] = function (payload) {
                var action = payload && payload.action;

                if (action === 'EXPORT_EXCEL_PROCESSING') {
                    console.log('[ExportBKCT] Đang xử lý...');
                    return;
                }

                if (action === 'EXPORT_EXCEL_DONE') {
                    delete window._pendingWebviewRequests[requestId];
                    var filePath = payload.filePath || '';
                    console.log('[ExportBKCT] ✅ Xuất thành công: ' + filePath);
                    if (window.showAlert) {
                        window.showAlert(
                            'Xuất Bảng kê chi tiết thành công! 📊',
                            'Đã xuất chi tiết hàng hóa ra file Excel.\nFile đã được mở tự động.',
                            'success'
                        );
                    }
                    return;
                }

                if (action === 'EXPORT_EXCEL_CANCELLED') {
                    delete window._pendingWebviewRequests[requestId];
                    console.log('[ExportBKCT] Người dùng đã hủy chọn nơi lưu.');
                    return;
                }

                if (action === 'EXPORT_EXCEL_ERROR') {
                    delete window._pendingWebviewRequests[requestId];
                    var msg = payload.message || 'Lỗi không xác định';
                    console.error('[ExportBKCT] ❌ Lỗi:', msg);
                    var isCancelled = payload.is_cancelled || msg.toLowerCase().includes('hủy') || msg.includes('há»§y');
                    if (isCancelled) {
                        console.log('[ExportBKCT] Người dùng hủy xuất.');
                    } else if (window.showAlert) {
                        window.showAlert('Lỗi xuất Excel', msg, 'error');
                    }
                    return;
                }
            };

            // Timeout 60 giây
            setTimeout(function () {
                if (window._pendingWebviewRequests[requestId]) {
                    delete window._pendingWebviewRequests[requestId];
                    console.warn('[ExportBKCT] Timeout sau 60 giây');
                }
            }, 60000);

            // 6. Gửi message xuống C#
            window.chrome.webview.postMessage({
                action: 'EXPORT_EXCEL',
                requestId: requestId,
                data: {
                    reportType: 'bang_ke_chi_tiet',
                    invoices: JSON.stringify(invoicesToSend),
                    isMuaVao: isMuaVao,
                    fromDate: dateRange.fromDisplay,
                    toDate: dateRange.toDisplay,
                    mst: mst
                }
            });

        } catch (error) {
            console.error('[ExportBKCT] ❌ Lỗi:', error);
            if (window.showAlert) {
                window.showAlert('Lỗi', 'Đã xảy ra lỗi: ' + (error.message || error), 'error');
            }
        }
    }

    // Đăng ký toàn cục
    window.exportBangKeChiTiet = exportBangKeChiTiet;

})(window);
