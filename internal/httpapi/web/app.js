const el = id => document.getElementById(id);
const apiBase = (window.MC_CONFIG?.apiBaseUrl ?? '').replace(/\/$/, '');
const states = { unknown: 'Đang lấy dữ liệu', online: 'Server đang phản hồi', stale: 'Dữ liệu chưa cập nhật', unreachable: 'Không truy vấn được server' };
const errors = { timeout: 'Truy vấn quá thời gian chờ', connection_error: 'Lỗi kết nối tới server', dns_error: 'Không phân giải được địa chỉ server', invalid_response: 'Server trả dữ liệu không hợp lệ', data_expired: 'Dữ liệu đã quá thời hạn cập nhật' };
const formatTime = value => value ? new Date(value).toLocaleString('vi-VN', { timeZone: 'Asia/Ho_Chi_Minh' }) : '—';
let source, fallback, reconnect, errorTimer, ageTimer, snapshot, requestPending = false;
let streamGeneration = 0;
function connection(label, connected = false) {
 el('connection').textContent = label;
 el('connection').classList.toggle('connected', connected);
}
function render(data) {
 snapshot = data;
 el('state').textContent = states[data.state]; el('indicator').className = data.state;
 el('online').textContent = data.playersOnline ?? '—';
 el('capacity').textContent = `/ ${data.playersMax ?? '—'}`;
 el('capacity-fill').style.width = `${data.state === 'online' && data.playersMax > 0 ? Math.min(100, Math.max(0, data.playersOnline / data.playersMax * 100)) : 0}%`;
 const players = data.state === 'online' && data.playersOnline > 0 ? (data.players ?? []).filter(player => player.name?.trim()) : [];
 el('players-list').replaceChildren(...players.map(player => {
  const item = document.createElement('li');
  const avatar = document.createElement('span'); avatar.className = 'player-avatar'; avatar.setAttribute('aria-hidden', 'true'); avatar.textContent = Array.from(player.name.trim()).slice(0, 2).join('').toUpperCase();
  const name = document.createElement('span'); name.className = 'player-name'; name.textContent = player.name;
  item.append(avatar, name); return item;
 }));
 el('players-empty').hidden = players.length > 0;
 el('sample-count').textContent = data.state !== 'online' ? 'Chờ dữ liệu mới' : `${players.length} tên được cung cấp`;
 el('players-message').textContent = data.state !== 'online' ? 'Chưa có dữ liệu người chơi hiện tại' : data.playersOnline === 0 ? 'Hiện chưa có người chơi online' : players.length === 0 ? 'Server không cung cấp tên người chơi' : players.length < data.playersOnline ? `Server cung cấp ${players.length} tên trong số ${data.playersOnline} người đang online` : '';
 el('players-message').hidden = !el('players-message').textContent;
 el('version').textContent = data.version ?? '—'; el('duration').textContent = data.queryDurationMs == null ? '—' : `${Math.round(data.queryDurationMs)} ms`;
 el('updated').textContent = formatTime(data.lastSuccessAt); el('error').textContent = errors[data.errorCode] ?? '';
 el('last').hidden = data.state === 'online' || !data.lastKnown;
 if (data.lastKnown) el('last-value').textContent = `${data.lastKnown.playersOnline} người chơi · ${formatTime(data.lastKnown.observedAt)}`;
 clearTimeout(ageTimer);
 if (data.state === 'online') ageTimer = setTimeout(() => { render({ ...data, state: 'stale', playersOnline: null, playersMax: null, version: null, queryDurationMs: null, errorCode: 'data_expired' }); }, Math.max(0, data.staleAfterMs - (Date.now() - Date.parse(data.lastSuccessAt))));
}
async function fetchStatus() {
 if (requestPending) return; requestPending = true;
 const generation = streamGeneration;
 try { const response = await fetch(`${apiBase}/api/v1/status`, { cache: 'no-store', signal: AbortSignal.timeout(5000) }); if (!response.ok) throw new Error(response.status); const data = await response.json(); if (generation === streamGeneration && (!snapshot || data.sequence >= snapshot.sequence || fallback)) render(data); }
 catch { if (generation === streamGeneration) { el('error').textContent = 'Mất kết nối website'; connection('Mất kết nối'); } }
 finally { requestPending = false; }
}
function connect() {
 source?.close(); source = new EventSource(`${apiBase}/api/v1/events`);
 source.addEventListener('status', event => {
  streamGeneration++;
  connection('Kết nối trực tiếp', true);
  render(JSON.parse(event.data)); clearTimeout(errorTimer); errorTimer = null;
  clearInterval(fallback); clearInterval(reconnect); fallback = reconnect = null;
 });
 source.onerror = () => {
  connection('Đang kết nối lại');
  if (!errorTimer && !fallback) errorTimer = setTimeout(() => {
   source.close(); errorTimer = null; connection('Cập nhật mỗi 5 giây'); fetchStatus(); fallback = setInterval(fetchStatus, 5000);
   reconnect = setInterval(connect, 30000);
  }, 30000);
 };
}
async function start() { await fetchStatus(); connect(); }
window.addEventListener('pagehide', () => { source?.close(); clearInterval(fallback); clearInterval(reconnect); clearTimeout(errorTimer); clearTimeout(ageTimer); });
window.addEventListener('pageshow', event => { if (event.persisted) start(); });
el('copy-address').addEventListener('click', async () => {
 try {
  await navigator.clipboard.writeText('bora.pikamc.vn:25005');
  el('copy-feedback').textContent = 'Đã sao chép! Hẹn gặp bạn trong game.';
 } catch {
  el('copy-feedback').textContent = 'Sao chép địa chỉ: bora.pikamc.vn:25005';
 }
});
start();
