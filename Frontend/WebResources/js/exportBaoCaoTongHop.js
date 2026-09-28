/**
 * ============================================================
 * XUẤT BÁO CÁO TỔNG HỢP HÓA ĐƠN RA FILE EXCEL
 * Gửi dữ liệu từ RAM xuống C# qua WebView2 để xuất bằng ClosedXML
 *
 * Gắn vào nút dropdown: onclick="exportBaoCaoTongHop()"
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
            console.warn('[ExportBCTH] Không lấy được ngày từ bộ lọc:', e);
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

    // ────────────────────────────────────────────────
    // HÀM CHÍNH: GỬI DỮ LIỆU XUỐNG C# ĐỂ XUẤT EXCEL
    // ────────────────────────────────────────────────
    function exportBaoCaoTongHop() {
        try {
            debugger;
            // 1. Kiểm tra WebView2
            if (!(window.chrome && window.chrome.webview)) {
                window.showAlert
                    ? window.showAlert('Lỗi', 'Không tìm thấy kết nối WebView2! Hãy chạy ứng dụng từ chương trình chính.', 'error')
                    : alert('WebView2 chưa sẵn sàng!');
                return;
            }

            // 2. Lấy dữ liệu từ RAM (ưu tiên getInvoiceRAM vì crawlerTable.js ghi đè setInvoiceRAM)
            var invoices = (window.getInvoiceRAM ? window.getInvoiceRAM() : null)
                || (window.InvoiceRamManager ? window.InvoiceRamManager.get() : null)
                || window._currentInvoicesRAM
                || window._invoiceCache
                || [];

            if (!invoices || invoices.length === 0) {
                window.showAlert
                    ? window.showAlert('Thông báo', 'Không có dữ liệu hóa đơn trong bộ nhớ để xuất!\nVui lòng tra cứu hóa đơn trước khi xuất báo cáo.', 'warning')
                    : alert('Không có dữ liệu hóa đơn trong bộ nhớ để xuất!');
                return;
            }

            // 3. Thu thập thông tin
            var isMuaVao = (window._currentInvoiceType || 'ban_ra') === 'mua_vao';
            var dateRange = getDateRange();
            var mst = getCurrentMst();

            console.log('[ExportBCTH] Gửi ' + invoices.length + ' hóa đơn xuống C# để xuất Excel...');

            // 4. Chuẩn hóa dữ liệu trước khi gửi (sửa lỗi lệch cột từ crawler)
            if (typeof window.sanitizeInvoiceData === 'function') {
                invoices.forEach(function (inv) { window.sanitizeInvoiceData(inv); });
            }

            // 4. Tạo requestId để nhận phản hồi
            var requestId = 'excel_' + Math.random().toString(36).slice(2);
            window._pendingWebviewRequests = window._pendingWebviewRequests || {};

            // 5. Đăng ký handler nhận phản hồi từ C#
            window._pendingWebviewRequests[requestId] = function (payload) {
                var action = payload && payload.action;

                if (action === 'EXPORT_EXCEL_PROCESSING') {
                    console.log('[ExportBCTH] Đang xử lý...');
                    return; // Giữ handler cho message tiếp theo
                }

                if (action === 'EXPORT_EXCEL_DONE') {
                    delete window._pendingWebviewRequests[requestId];
                    var filePath = payload.filePath || '';
                    console.log('[ExportBCTH] ✅ Xuất thành công: ' + filePath);
                    if (window.showAlert) {
                        window.showAlert(
                            'Xuất Excel thành công! 🎉',
                            'Đã xuất ' + invoices.length + ' hóa đơn.\nFile đã được mở tự động.',
                            'success'
                        );
                    }
                    return;
                }

                if (action === 'EXPORT_EXCEL_ERROR') {
                    delete window._pendingWebviewRequests[requestId];
                    var msg = payload.message || 'Lỗi không xác định';
                    console.error('[ExportBCTH] ❌ Lỗi:', msg);
                    // Nếu người dùng hủy → không hiện lỗi đỏ
                    if (msg.includes('hủy')) {
                        console.log('[ExportBCTH] Người dùng hủy xuất.');
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
                    console.warn('[ExportBCTH] Timeout sau 60 giây');
                }
            }, 60000);

            // 6. GỬI MESSAGE XUỐNG C#
            window.chrome.webview.postMessage({
                action: 'EXPORT_EXCEL',
                requestId: requestId,
                data: {
                    reportType: 'bao_cao_tong_hop',
                    invoices: JSON.stringify(invoices),
                    isMuaVao: isMuaVao,
                    fromDate: dateRange.fromDisplay,
                    toDate: dateRange.toDisplay,
                    mst: mst
                }
            });

        } catch (error) {
            console.error('[ExportBCTH] ❌ Lỗi:', error);
            if (window.showAlert) {
                window.showAlert('Lỗi', 'Đã xảy ra lỗi: ' + (error.message || error), 'error');
            }
        }
    }

    // Đăng ký toàn cục
    window.exportBaoCaoTongHop = exportBaoCaoTongHop;

})(window);
