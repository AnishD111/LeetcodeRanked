const API = 'http://localhost:8080';

// If already logged in, skip the auth page entirely
window.addEventListener('DOMContentLoaded', async () => {
  const res = await fetch(`${API}/api/auth/me`, { credentials: 'include' });
  if (res.ok) window.location.href = 'game.html';
});

function switchTab(tab) {
  const isLogin = tab === 'login';
  document.getElementById('login-form').classList.toggle('hidden', !isLogin);
  document.getElementById('register-form').classList.toggle('hidden', isLogin);
  document.getElementById('tab-login').classList.toggle('active', isLogin);
  document.getElementById('tab-register').classList.toggle('active', !isLogin);
  clearError();
}

async function handleLogin(e) {
  e.preventDefault();
  const res = await fetch(`${API}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({
      email:    document.getElementById('login-email').value,
      password: document.getElementById('login-password').value,
    }),
  });
  const data = await res.json();
  if (res.ok) {
    window.location.href = 'game.html';
  } else {
    showError(data.error || 'Login failed');
  }
}

async function handleRegister(e) {
  e.preventDefault();
  const res = await fetch(`${API}/api/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({
      username: document.getElementById('reg-username').value,
      email:    document.getElementById('reg-email').value,
      password: document.getElementById('reg-password').value,
    }),
  });
  const data = await res.json();
  if (res.ok) {
    window.location.href = 'game.html';
  } else {
    showError(data.error || 'Registration failed');
  }
}

function showError(msg) {
  const el = document.getElementById('auth-error');
  el.textContent = msg;
  el.classList.remove('hidden');
}

function clearError() {
  const el = document.getElementById('auth-error');
  el.textContent = '';
  el.classList.add('hidden');
}
