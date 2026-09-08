const API = 'http://localhost:8080';
let socket = null;

const statusBadge  = document.getElementById('status-badge');
const codeEditor   = document.getElementById('codeEditor');
const submitBtn    = document.getElementById('submitBtn');
const findMatchBtn = document.getElementById('findMatchBtn');
const logStream    = document.getElementById('logStream');

// On load, verify session. If not authenticated, send back to login.
window.addEventListener('DOMContentLoaded', async () => {
  const ok = await fetchUserInfo();
  if (ok) {
    fetchEloHistory();
  }
});

async function fetchUserInfo() {
  try {
    const res = await fetch(`${API}/api/auth/me`, { credentials: 'include' });
    if (!res.ok) {
      window.location.href = 'index.html';
      return false;
    }
    const user = await res.json();
    document.getElementById('user-info').textContent =
      `${user.username}  ·  ${user.elo_rating} Elo  ·  ${user.rank_tier}`;
    return true;
  } catch {
    window.location.href = 'index.html';
    return false;
  }
}

async function fetchEloHistory() {
  try {
    const res = await fetch(`${API}/api/user/elo-history`, { credentials: 'include' });
    if (!res.ok) return;
    const history = await res.json();
    renderEloHistory(history);
  } catch (err) {
    console.error('Error fetching Elo history:', err);
  }
}

function renderEloHistory(history) {
  const container = document.getElementById('historyList');
  if (history.length === 0) {
    container.innerHTML = '<div class="history-placeholder">No match history yet.</div>';
    return;
  }

  container.innerHTML = history.map(entry => {
    const delta = entry.new_elo - entry.old_elo;
    const isGain = delta >= 0;
    const deltaText = isGain ? `+${delta}` : `${delta}`;
    const deltaClass = isGain ? 'gain' : 'loss';
    const dateText = new Date(entry.recorded_at).toLocaleDateString();

    return `
      <div class="history-entry">
        <div>
          <span>Rating Change</span>
          <div class="date">${dateText}</div>
        </div>
        <span class="delta ${deltaClass}">${entry.old_elo} ➔ ${entry.new_elo} (${deltaText})</span>
      </div>
    `;
  }).join('');
}


async function handleLogout() {
  await fetch(`${API}/api/auth/logout`, { method: 'POST', credentials: 'include' });
  if (socket) { socket.close(); socket = null; }
  window.location.href = 'index.html';
}

function findMatch() {
  findMatchBtn.disabled = true;
  findMatchBtn.textContent = 'Searching...';
  setStatus('queued', 'Queued');
  appendLog('Joining matchmaking queue...', 'system');

  socket = new WebSocket('ws://localhost:8080/ws/join');

  socket.onopen = () => appendLog('Connected — waiting for an opponent.', 'system');

  socket.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      if (msg.type === 'MATCH_FOUND') {
        onMatchFound(msg);
        return;
      }
    } catch {}

    const text = event.data;
    if      (text.includes('SUCCESS') || text.includes('WIN'))                    appendLog(text, 'success');
    else if (text.includes('FAILURE') || text.includes('TIMEOUT') || text.includes('OVER')) appendLog(text, 'failure');
    else                                                                           appendLog(text, 'system');
  };

  socket.onclose = () => {
    setStatus('', 'Disconnected');
    codeEditor.disabled    = true;
    submitBtn.disabled     = true;
    findMatchBtn.disabled  = false;
    findMatchBtn.textContent = 'Find Match';
    appendLog('Connection closed.', 'failure');
    fetchUserInfo();
    fetchEloHistory();
  };


  socket.onerror = () => appendLog('WebSocket error.', 'failure');
}

function onMatchFound(msg) {
  setStatus('in-match', 'In Match');
  appendLog(msg.log_msg, 'system');

  if (msg.problem_name) {
    document.getElementById('problem-title').textContent = msg.problem_name;
    // Render HTML description from the database (contains <p>, <code>, <pre> tags)
    const descEl = document.getElementById('problem-desc');
    if (msg.description) {
      descEl.innerHTML = msg.description;
    } else {
      descEl.textContent = parseProblemDesc(msg.log_msg);
    }
    document.getElementById('problem-header').classList.remove('hidden');
    document.getElementById('editor-heading').classList.add('hidden');
  }

  codeEditor.disabled = false;
  submitBtn.disabled  = false;
  if (msg.template) codeEditor.value = msg.template;
}

function submitCode() {
  if (!socket || socket.readyState !== WebSocket.OPEN) return;
  appendLog('Sending submission to sandbox...', 'system');
  socket.send(codeEditor.value);
}

function appendLog(text, type = 'system') {
  const el = document.createElement('div');
  el.className = `log-entry log-${type}`;
  el.textContent = `[${new Date().toLocaleTimeString()}] ${text}`;
  logStream.appendChild(el);
  logStream.scrollTop = logStream.scrollHeight;
}

function setStatus(cls, label) {
  statusBadge.className = 'status-badge' + (cls ? ` ${cls}` : '');
  statusBadge.textContent = label;
}

function parseProblemDesc(logMsg) {
  const lines = logMsg.split('\n').map(l => l.trim()).filter(Boolean);
  const idx = lines.findIndex(l => l.startsWith('Problem:'));
  return idx >= 0 ? lines.slice(idx + 1).join(' ').trim() : '';
}
