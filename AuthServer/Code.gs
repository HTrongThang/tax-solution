/**
 * TaxSolution - Auth server (Google Apps Script Web App)
 *
 * Script Properties cần có (Cài đặt dự án > Thuộc tính tập lệnh):
 *   API_KEY      = chuỗi bí mật, trùng AUTH_API_KEY trong Backend/.env
 *   FOUNDER_HASH = SHA-256 (hex thường) mật khẩu Founder
 *
 * Triển khai: Deploy > Web app > Execute as: Me, Who has access: Anyone.
 * Sau khi sửa code, phải Deploy > Manage deployments > Edit > Version: New version.
 *
 * Sheet "Users" tự tạo ở lần gọi đầu. Có thể sửa trực tiếp các cột:
 *   Status (ACTIVE / PENDING / LOCKED), HardwareId, MaxTaxAccounts, AllowAI,
 *   StartDate, EndDate (định dạng dd/MM/yyyy).
 *
 * Khóa (LockService): chỉ các thao tác GHI mới lấy khóa (register, adminCreate, gắn Mã máy lần đầu),
 * và chỉ chờ tối đa LOCK_WAIT_MS. login/check chỉ đọc nên KHÔNG bị chặn bởi một lượt ghi bị kẹt.
 */

var SHEET_NAME = 'Users';
var HEADERS = ['Username', 'PasswordHash', 'Salt', 'FullName', 'HardwareId', 'Status',
  'MaxTaxAccounts', 'AllowAI', 'StartDate', 'EndDate', 'CreatedAt', 'Source', 'Note'];
var COL = {};
HEADERS.forEach(function (h, i) { COL[h] = i; });

var DEFAULT_MAX_TAX = 5;
var TZ = 'Asia/Ho_Chi_Minh';
var LOCK_WAIT_MS = 5000;

function doGet() {
  return out({ result: 'OK', message: 'TaxSolution auth server' });
}

function doPost(e) {
  try {
    var req = JSON.parse(e.postData.contents);
    var props = PropertiesService.getScriptProperties();

    var apiKey = props.getProperty('API_KEY');
    if (!apiKey || !req || req.apiKey !== apiKey) {
      return out({ result: 'UNAUTHORIZED' });
    }

    switch (req.action) {
      // Chỉ đọc: không khóa
      case 'login': return out(login(req));
      case 'check': return out(check(req));
      // Ghi: có khóa
      case 'register': return out(withLock(function () { return register(req); }));
      case 'adminCreate': return out(withLock(function () { return adminCreate(req, props); }));
      default: return out({ result: 'BAD_REQUEST' });
    }
  } catch (err) {
    return out({ result: 'ERROR', message: String(err) });
  }
}

/** Chạy fn khi có khóa; chờ tối đa LOCK_WAIT_MS, hết thời gian thì báo bận (không chờ vô hạn). */
function withLock(fn) {
  var lock = LockService.getScriptLock();
  if (!lock.tryLock(LOCK_WAIT_MS)) {
    return { result: 'ERROR', message: 'Hệ thống đang bận, vui lòng thử lại.' };
  }
  try {
    return fn();
  } finally {
    lock.releaseLock();
  }
}

// ---------------------------------------------------------------- actions

/** Đăng nhập: Username + passHash (SHA-256 của mật khẩu) + hwid. */
function login(req) {
  var found = findUser(req.username);
  if (!found) return { result: 'WRONG_CREDENTIALS' };
  var row = found.row;

  if (saltedHash(row[COL.Salt], req.passHash) !== String(row[COL.PasswordHash])) {
    return { result: 'WRONG_CREDENTIALS' };
  }
  return evaluate(found, req.hwid, true);
}

/** Kiểm tra lại khi đang dùng app (không cần mật khẩu, chỉ Username + hwid). */
function check(req) {
  var found = findUser(req.username);
  if (!found) return { result: 'WRONG_CREDENTIALS' };
  return evaluate(found, req.hwid, false);
}

/** Khách gửi yêu cầu cấp tài khoản: ghi dòng PENDING. */
function register(req) {
  var username = clean(req.username);
  if (!username || !req.passHash || !req.hwid) return { result: 'BAD_REQUEST' };
  if (username.toLowerCase() === 'founder' || findUser(username)) return { result: 'EXISTS' };

  var salt = Utilities.getUuid();
  sheet().appendRow([
    username, saltedHash(salt, req.passHash), salt, clean(req.fullName), clean(req.hwid), 'PENDING',
    DEFAULT_MAX_TAX, false, '', '', nowText(), 'request', ''
  ]);
  return { result: 'OK' };
}

/** Founder cấp tài khoản: ghi dòng ACTIVE. adminToken = SHA-256 mật khẩu Founder. */
function adminCreate(req, props) {
  var founderHash = String(props.getProperty('FOUNDER_HASH') || '').toLowerCase();
  if (!founderHash || String(req.adminToken || '').toLowerCase() !== founderHash) {
    return { result: 'UNAUTHORIZED' };
  }
  var username = clean(req.username);
  if (!username || !req.passHash) return { result: 'BAD_REQUEST' };
  if (username.toLowerCase() === 'founder' || findUser(username)) return { result: 'EXISTS' };

  var salt = Utilities.getUuid();
  sheet().appendRow([
    username, saltedHash(salt, req.passHash), salt, clean(req.fullName), clean(req.hwid), 'ACTIVE',
    toInt(req.maxTaxAccounts, DEFAULT_MAX_TAX), req.allowAI === true, clean(req.startDate), clean(req.endDate),
    nowText(), 'founder', ''
  ]);
  return { result: 'OK' };
}

// ---------------------------------------------------------------- helpers

/** Trạng thái, thiết bị, ngày hiệu lực. Ngày tính theo giờ server Google (không tin giờ máy khách). */
function evaluate(found, hwid, bindIfEmpty) {
  var row = found.row;
  var status = String(row[COL.Status]).toUpperCase();
  if (status === 'PENDING') return { result: 'PENDING' };
  if (status !== 'ACTIVE') return { result: 'LOCKED' };

  var boundHwid = String(row[COL.HardwareId] || '').trim();
  if (!boundHwid) {
    // Chưa gắn máy: gắn máy của lần đăng nhập đầu tiên (thao tác ghi => có khóa)
    if (!bindIfEmpty || !hwid) return { result: 'WRONG_DEVICE' };
    var bound = bindHwid(found.index, String(hwid));
    if (bound === null) return { result: 'ERROR', message: 'Hệ thống đang bận, vui lòng thử lại.' };
    if (bound !== String(hwid)) return { result: 'WRONG_DEVICE' };
  } else if (boundHwid !== String(hwid || '')) {
    return { result: 'WRONG_DEVICE' };
  }

  var today = todayText();
  var start = dateText(row[COL.StartDate]);
  var end = dateText(row[COL.EndDate]);
  if (start && today < start) return { result: 'NOT_STARTED', startDate: toDisplay(start) };
  if (end && today > end) return { result: 'EXPIRED', endDate: toDisplay(end) };

  return {
    result: 'OK',
    fullName: String(row[COL.FullName] || ''),
    maxTaxAccounts: toInt(row[COL.MaxTaxAccounts], DEFAULT_MAX_TAX),
    allowAI: row[COL.AllowAI] === true || String(row[COL.AllowAI]).toUpperCase() === 'TRUE',
    endDate: end ? toDisplay(end) : ''
  };
}

/**
 * Gắn Mã máy vào dòng nếu còn trống. Đọc lại ô trong khóa để tránh hai máy cùng gắn.
 * Trả về Mã máy đang được gắn sau cùng, hoặc null nếu không lấy được khóa.
 */
function bindHwid(rowIndex, hwid) {
  var lock = LockService.getScriptLock();
  if (!lock.tryLock(LOCK_WAIT_MS)) return null;
  try {
    var cell = sheet().getRange(rowIndex, COL.HardwareId + 1);
    var current = String(cell.getValue() || '').trim();
    if (!current) {
      cell.setValue(hwid);
      return hwid;
    }
    return current;
  } finally {
    lock.releaseLock();
  }
}

function sheet() {
  var ss = SpreadsheetApp.getActiveSpreadsheet();
  var sh = ss.getSheetByName(SHEET_NAME);
  if (!sh) {
    sh = ss.insertSheet(SHEET_NAME);
    sh.appendRow(HEADERS);
    sh.setFrozenRows(1);
  }
  return sh;
}

function findUser(username) {
  var name = clean(username).toLowerCase();
  if (!name) return null;
  var data = sheet().getDataRange().getValues();
  for (var i = 1; i < data.length; i++) {
    if (String(data[i][COL.Username]).toLowerCase() === name) {
      return { row: data[i], index: i + 1 };
    }
  }
  return null;
}

/** SHA-256(salt + passHash) dạng hex thường. */
function saltedHash(salt, passHash) {
  var bytes = Utilities.computeDigest(Utilities.DigestAlgorithm.SHA_256, String(salt) + String(passHash).toLowerCase());
  return bytes.map(function (b) { return ('0' + (b & 0xff).toString(16)).slice(-2); }).join('');
}

function clean(v) { return v == null ? '' : String(v).trim(); }
function toInt(v, def) { var n = parseInt(v, 10); return isNaN(n) || n < 0 ? def : n; }
function nowText() { return Utilities.formatDate(new Date(), TZ, 'dd/MM/yyyy HH:mm'); }
function todayText() { return Utilities.formatDate(new Date(), TZ, 'yyyy-MM-dd'); }

/** Nhận Date của Sheet hoặc chuỗi dd/MM/yyyy, trả về yyyy-MM-dd để so sánh. */
function dateText(v) {
  if (v === '' || v == null) return '';
  if (Object.prototype.toString.call(v) === '[object Date]') return Utilities.formatDate(v, TZ, 'yyyy-MM-dd');
  var m = String(v).trim().match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})$/);
  if (!m) return '';
  return m[3] + '-' + ('0' + m[2]).slice(-2) + '-' + ('0' + m[1]).slice(-2);
}
function toDisplay(ymd) { var p = ymd.split('-'); return p[2] + '/' + p[1] + '/' + p[0]; }

function out(obj) {
  return ContentService.createTextOutput(JSON.stringify(obj)).setMimeType(ContentService.MimeType.JSON);
}
