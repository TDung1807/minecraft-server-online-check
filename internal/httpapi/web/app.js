const el = id => document.getElementById(id);
const apiBase = (window.MC_CONFIG?.apiBaseUrl ?? '').replace(/\/$/, '');
const states = { unknown: 'Đang lấy dữ liệu', online: 'Server đang phản hồi', stale: 'Dữ liệu chưa cập nhật', unreachable: 'Không truy vấn được server' };
const errors = { timeout: 'Truy vấn quá thời gian chờ', connection_error: 'Lỗi kết nối tới server', dns_error: 'Không phân giải được địa chỉ server', invalid_response: 'Server trả dữ liệu không hợp lệ', data_expired: 'Dữ liệu đã quá thời hạn cập nhật' };
const formatTime = value => value ? new Date(value).toLocaleString('vi-VN', { timeZone: 'Asia/Ho_Chi_Minh' }) : '—';
let source, fallback, reconnect, errorTimer, ageTimer, snapshot, requestPending = false;
let streamGeneration = 0;
function render(data) {
 snapshot = data;
 el('state').textContent = states[data.state]; el('indicator').className = data.state;
 el('online').textContent = data.playersOnline ?? '—'; el('max').textContent = `/ ${data.playersMax ?? '—'}`;
 el('version').textContent = data.version ?? '—'; el('duration').textContent = data.queryDurationMs == null ? '—' : `${Math.round(data.queryDurationMs)} ms`;
 el('updated').textContent = formatTime(data.lastSuccessAt); el('error').textContent = errors[data.errorCode] ?? '';
 el('last').hidden = data.state === 'online' || !data.lastKnown;
 if (data.lastKnown) el('last-value').textContent = `${data.lastKnown.playersOnline}/${data.lastKnown.playersMax} · ${formatTime(data.lastKnown.observedAt)}`;
 clearTimeout(ageTimer);
 if (data.state === 'online') ageTimer = setTimeout(() => { render({ ...data, state: 'stale', playersOnline: null, playersMax: null, version: null, queryDurationMs: null, errorCode: 'data_expired' }); }, Math.max(0, data.staleAfterMs - (Date.now() - Date.parse(data.lastSuccessAt))));
}
async function fetchStatus() {
 if (requestPending) return; requestPending = true;
 const generation = streamGeneration;
 try { const response = await fetch(`${apiBase}/api/v1/status`, { cache: 'no-store', signal: AbortSignal.timeout(5000) }); if (!response.ok) throw new Error(response.status); const data = await response.json(); if (generation === streamGeneration && (!snapshot || data.sequence >= snapshot.sequence || fallback)) render(data); }
 catch { if (generation === streamGeneration) el('transport').textContent = 'Mất kết nối website'; }
 finally { requestPending = false; }
}
function connect() {
 source?.close(); source = new EventSource(`${apiBase}/api/v1/events`);
 source.addEventListener('status', event => {
  streamGeneration++;
  render(JSON.parse(event.data)); clearTimeout(errorTimer); errorTimer = null;
  clearInterval(fallback); clearInterval(reconnect); fallback = reconnect = null;
  el('transport').textContent = 'Kết nối trực tiếp';
 });
 source.onerror = () => {
  el('transport').textContent = 'Đang kết nối lại';
  if (!errorTimer && !fallback) errorTimer = setTimeout(() => {
   source.close(); errorTimer = null; fetchStatus(); fallback = setInterval(fetchStatus, 5000);
   reconnect = setInterval(connect, 30000); el('transport').textContent = 'Cập nhật định kỳ';
  }, 30000);
 };
}
async function start() { await fetchStatus(); connect(); }
window.addEventListener('pagehide', () => { source?.close(); clearInterval(fallback); clearInterval(reconnect); clearTimeout(errorTimer); clearTimeout(ageTimer); });
window.addEventListener('pageshow', event => { if (event.persisted) start(); });
setInterval(() => { el('clock').textContent = new Date().toLocaleTimeString('vi-VN', { timeZone: 'Asia/Ho_Chi_Minh' }); }, 1000);
start();
