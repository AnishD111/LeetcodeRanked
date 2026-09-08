import React, { useState } from 'react';
import { X, Lock, Mail, User, AlertCircle } from 'lucide-react';

const API_BASE = 'http://localhost:8080';

export default function AuthModal({ isOpen, onClose, onAuthSuccess }) {
  const [isLogin, setIsLogin] = useState(true);
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  if (!isOpen) return null;

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    const endpoint = isLogin ? '/api/auth/login' : '/api/auth/register';
    const payload = isLogin
      ? { email, password }
      : { username, email, password };

    try {
      const res = await fetch(`${API_BASE}${endpoint}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(payload),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || 'Authentication failed');
      }

      onAuthSuccess(data.user);
      onClose();
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleOAuth = (provider) => {
    window.location.href = `${API_BASE}/api/auth/${provider}`;
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
      <div className="bg-[#161b22] border border-[#30363d] rounded-2xl w-full max-w-md p-6 shadow-2xl relative">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 text-[#8b949e] hover:text-white p-1 rounded-lg hover:bg-[#21262d] transition-colors"
        >
          <X className="h-5 w-5" />
        </button>

        <div className="text-center mb-6">
          <h2 className="text-2xl font-bold text-white tracking-tight">
            {isLogin ? 'Welcome Back' : 'Create Account'}
          </h2>
          <p className="text-sm text-[#8b949e] mt-1">
            {isLogin ? 'Sign in to jump into ranked matches' : 'Register to start competing and tracking Elo'}
          </p>
        </div>

        <div className="grid grid-cols-2 gap-3 mb-6">
          <button
            onClick={() => handleOAuth('github')}
            className="flex items-center justify-center gap-2 bg-[#21262d] hover:bg-[#30363d] border border-[#30363d] text-white py-2.5 px-4 rounded-xl text-sm font-medium transition-all active:scale-95"
          >
            <svg className="h-4 w-4 fill-current" viewBox="0 0 24 24">
              <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
            </svg>
            <span>GitHub</span>
          </button>
          <button
            onClick={() => handleOAuth('google')}
            className="flex items-center justify-center gap-2 bg-[#21262d] hover:bg-[#30363d] border border-[#30363d] text-white py-2.5 px-4 rounded-xl text-sm font-medium transition-all active:scale-95"
          >
            <svg className="h-4 w-4" viewBox="0 0 24 24">
              <path
                fill="#EA4335"
                d="M12 5c1.6 0 3 .6 4.1 1.7l3.1-3.1C17.3 1.8 14.8 1 12 1 7.5 1 3.7 3.6 1.9 7.3l3.7 2.9C6.5 7.3 9 5 12 5z"
              />
              <path
                fill="#4285F4"
                d="M23.5 12.3c0-.8-.1-1.7-.2-2.3H12v4.6h6.5c-.3 1.5-1.1 2.8-2.4 3.7l3.7 2.9c2.2-2 3.7-5 3.7-8.9z"
              />
              <path
                fill="#FBBC05"
                d="M5.6 14.8c-.2-.7-.4-1.5-.4-2.8s.2-2.1.4-2.8L1.9 6.3C.7 8.7 0 10.3 0 12s.7 3.3 1.9 5.7l3.7-2.9z"
              />
              <path
                fill="#34A853"
                d="M12 23c3.2 0 6-1.1 8-3l-3.7-2.9c-1.1.7-2.5 1.2-4.3 1.2-3 0-5.5-2.3-6.4-5.2L1.9 16c1.8 3.7 5.6 7 10.1 7z"
              />
            </svg>
            <span>Google</span>
          </button>
        </div>

        <div className="relative flex py-2 items-center mb-6">
          <div className="flex-grow border-t border-[#30363d]"></div>
          <span className="flex-shrink mx-4 text-xs uppercase tracking-wider text-[#8b949e]">or with email</span>
          <div className="flex-grow border-t border-[#30363d]"></div>
        </div>

        {error && (
          <div className="flex items-center gap-2 p-3 bg-red-900/30 border border-red-700/50 rounded-xl text-red-300 text-sm mb-4">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          {!isLogin && (
            <div>
              <label className="block text-xs font-medium text-[#8b949e] mb-1.5">Username</label>
              <div className="relative">
                <User className="h-4 w-4 absolute left-3 top-3.5 text-[#8b949e]" />
                <input
                  type="text"
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="LeetCoder123"
                  className="w-full bg-[#0d1117] border border-[#30363d] focus:border-purple-500 focus:ring-1 focus:ring-purple-500 rounded-xl py-2.5 pl-10 pr-4 text-sm text-white placeholder-[#484f58] outline-none transition-all"
                />
              </div>
            </div>
          )}

          <div>
            <label className="block text-xs font-medium text-[#8b949e] mb-1.5">Email Address</label>
            <div className="relative">
              <Mail className="h-4 w-4 absolute left-3 top-3.5 text-[#8b949e]" />
              <input
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
                className="w-full bg-[#0d1117] border border-[#30363d] focus:border-purple-500 focus:ring-1 focus:ring-purple-500 rounded-xl py-2.5 pl-10 pr-4 text-sm text-white placeholder-[#484f58] outline-none transition-all"
              />
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-[#8b949e] mb-1.5">Password</label>
            <div className="relative">
              <Lock className="h-4 w-4 absolute left-3 top-3.5 text-[#8b949e]" />
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                className="w-full bg-[#0d1117] border border-[#30363d] focus:border-purple-500 focus:ring-1 focus:ring-purple-500 rounded-xl py-2.5 pl-10 pr-4 text-sm text-white placeholder-[#484f58] outline-none transition-all"
              />
            </div>
          </div>

          <button
            type="submit"
            disabled={loading}
            className="w-full bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-500 hover:to-indigo-500 disabled:opacity-50 text-white font-semibold py-2.5 px-4 rounded-xl text-sm transition-all shadow-lg shadow-purple-600/30 active:scale-98 cursor-pointer mt-2"
          >
            {loading ? 'Processing...' : isLogin ? 'Sign In' : 'Create Account'}
          </button>
        </form>

        <div className="text-center mt-6 text-sm text-[#8b949e]">
          {isLogin ? (
            <span>
              Don't have an account?{' '}
              <button
                type="button"
                onClick={() => { setIsLogin(false); setError(''); }}
                className="text-purple-400 hover:text-purple-300 font-semibold ml-1 cursor-pointer"
              >
                Sign Up
              </button>
            </span>
          ) : (
            <span>
              Already have an account?{' '}
              <button
                type="button"
                onClick={() => { setIsLogin(true); setError(''); }}
                className="text-purple-400 hover:text-purple-300 font-semibold ml-1 cursor-pointer"
              >
                Sign In
              </button>
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
