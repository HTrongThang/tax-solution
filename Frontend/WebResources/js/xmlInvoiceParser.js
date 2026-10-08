/**
 * Module giao tiếp giữa WebView2 (Frontend) và C# Backend
 * Phục vụ việc quét thư mục và bóc tách toàn bộ file XML Hóa đơn điện tử (NĐ123/TT78)
 */
(function (window) {
    'use strict';

    window.XmlInvoiceParser = {
        /**
         * Quét thư mục và bóc tách danh sách hóa đơn XML
         * @param {string|null} directoryPath Đường dẫn thư mục (nếu null, C# sẽ tự mở FolderBrowserDialog)
         * @param {object|Function} options Tùy chọn lọc {targetMst, isMuaVao, fromDate, toDate} hoặc callback onProgress
         * @param {Function} onProgress Callback cập nhật tiến độ: ({current, total, currentFile})
         * @param {number} timeoutMs Timeout tối đa (mặc định 10 phút)
         * @returns {Promise<object>} Kết quả bóc tách bao gồm {invoices, totalFound, totalParsed, totalSkipped, totalFailed, errors}
         */
        parseDirectory: function (directoryPath = null, options = {}, onProgress = null, timeoutMs = 10 * 60 * 1000) {
            // Hỗ trợ truyền thẳng callback vào tham số thứ 2
            if (typeof options === 'function') {
                onProgress = options;
                options = {};
            }

            return new Promise((resolve, reject) => {
                if (!window.chrome || !window.chrome.webview) {
                    reject(new Error('Môi trường WebView2 chưa sẵn sàng.'));
                    return;
                }

                const requestId = 'xml_' + Math.random().toString(36).substring(2, 10);
                window._pendingWebviewRequests = window._pendingWebviewRequests || {};

                // Timeout bảo vệ (mặc định 10 phút) để không treo promise vĩnh viễn
                const timer = setTimeout(() => {
                    try { delete window._pendingWebviewRequests[requestId]; } catch (e) { }
                    reject(new Error('Timeout — quá trình bóc tách XML quá lâu.'));
                }, timeoutMs);

                // Đăng ký handler nhận kết quả từ C#
                window._pendingWebviewRequests[requestId] = function (payload) {
                    const action = payload && payload.action;

                    // Bước 1: C# bắt đầu quét (bỏ qua, chỉ log)
                    if (action === 'PARSE_XML_START') {
                        console.log('[XmlInvoiceParser] Bắt đầu quét:', payload.directoryPath);
                        return; // Không delete handler, chờ tiếp
                    }

                    // Bước 2: Tiến độ bóc tách
                    if (action === 'PARSE_XML_PROGRESS') {
                        if (typeof onProgress === 'function') {
                            onProgress({
                                current: payload.current || 0,
                                total: payload.total || 0,
                                currentFile: payload.currentFile || ''
                            });
                        }
                        return; // Không delete handler, chờ tiếp
                    }

                    // Bước 3: Hoàn tất
                    if (action === 'PARSE_XML_DONE') {
                        clearTimeout(timer);
                        delete window._pendingWebviewRequests[requestId];
                        resolve(payload);
                        return;
                    }

                    // Bước 4: Lỗi
                    if (action === 'PARSE_XML_ERROR') {
                        clearTimeout(timer);
                        delete window._pendingWebviewRequests[requestId];
                        reject(new Error(payload.message || 'Lỗi không xác định khi đọc XML.'));
                        return;
                    }

                    // Bước 5: Người dùng hủy chọn thư mục
                    if (action === 'PARSE_XML_CANCELLED') {
                        clearTimeout(timer);
                        delete window._pendingWebviewRequests[requestId];
                        resolve({ cancelled: true, message: payload.message });
                        return;
                    }
                };

                // Gửi thông điệp tới C#
                window.chrome.webview.postMessage({
                    action: 'PARSE_XML_DIRECTORY',
                    requestId: requestId,
                    data: {
                        directoryPath: directoryPath,
                        targetMst: options.targetMst || options.mst || window._loggedInMst || window._selectedCompanyMst || '',
                        isMuaVao: typeof options.isMuaVao === 'boolean' ? options.isMuaVao : (window._currentInvoiceType === 'mua_vao'),
                        fromDate: options.fromDate || '',
                        toDate: options.toDate || ''
                    }
                });
            });
        }
    };
})(window);
