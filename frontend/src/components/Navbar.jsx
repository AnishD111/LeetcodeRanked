import React from 'react';
import { Swords, Trophy, LogOut, User, History } from 'lucide-react';

const TIER_COLORS = {
  Bronze: 'from-amber-700 to-amber-900 border-amber-600 text-amber-200',
  Silver: 'from-slate-400 to-slate-600 border-slate-300 text-slate-100',
  Gold: 'from-yellow-500 to-amber-600 border-yellow-300 text-yellow-100',
  Platinum: 'from-cyan-500 to-blue-600 border-cyan-300 text-cyan-100',
  Diamond: 'from-purple-500 to-indigo-600 border-purple-300 text-purple-100',
  Master: 'from-rose-500 to-red-700 border-rose-300 text-rose-100',
};

export default function Navbar({
  user,
  onOpenAuth,
  onLogout,
  onOpenHistory,
  queueStatus,
  onFindMatch,
  onCancelQueue,
  inMatch,
}) {
  const tierClass = user ? (TIER_COLORS[user.rank_tier] || TIER_COLORS.Bronze) : TIER_COLORS.Bronze;

  return (
    <header className="border-b border-[#30363d] bg-[#161b22] px-6 py-3 flex items-center justify-between sticky top-0 z-40">
      <div className="flex items-center gap-3">
        <div className="h-10 w-10 rounded-xl bg-gradient-to-tr from-purple-600 to-indigo-500 flex items-center justify-center shadow-lg shadow-purple-500/20">
          <Swords className="h-5 w-5 text-white" />
        </div>
        <div>
          <span className="font-bold text-lg text-white tracking-tight flex items-center gap-1.5">
            LeetCode <span className="bg-gradient-to-r from-purple-400 to-indigo-400 bg-clip-text text-transparent">Ranked</span>
          </span>
          <span className="text-xs text-[#8b949e] block -mt-1">Real-Time 1v1 Arena</span>
        </div>
      </div>

      <div className="flex items-center gap-4">
        {user && !inMatch && (
          <div>
            {queueStatus === 'idle' ? (
              <button
                onClick={onFindMatch}
                className="flex items-center gap-2 bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-500 hover:to-indigo-500 text-white font-medium px-5 py-2 rounded-lg shadow-md shadow-purple-600/30 transition-all active:scale-95 cursor-pointer"
              >
                <Swords className="h-4 w-4" />
                <span>Find Match</span>
              </button>
            ) : (
              <div className="flex items-center gap-2 bg-[#21262d] border border-[#30363d] px-4 py-1.5 rounded-lg">
                <div className="h-2.5 w-2.5 rounded-full bg-yellow-400 animate-ping" />
                <span className="text-sm text-yellow-400 font-medium">Searching for opponent...</span>
                <button
                  onClick={onCancelQueue}
                  className="text-xs text-[#8b949e] hover:text-white ml-2 underline cursor-pointer"
                >
                  Cancel
                </button>
              </div>
            )}
          </div>
        )}

        {user ? (
          <div className="flex items-center gap-3 pl-3 border-l border-[#30363d]">
            <div className={`px-3 py-1 rounded-lg border bg-gradient-to-r ${tierClass} flex items-center gap-1.5 shadow-sm`}>
              <Trophy className="h-3.5 w-3.5" />
              <span className="text-xs font-bold uppercase tracking-wider">{user.rank_tier}</span>
              <span className="text-xs font-semibold opacity-90">{user.elo_rating} Elo</span>
            </div>

            <div className="flex items-center gap-2 text-sm text-[#f0f6fc]">
              {user.avatar_url ? (
                <img src={user.avatar_url} alt={user.username} className="h-8 w-8 rounded-full border border-[#30363d]" />
              ) : (
                <div className="h-8 w-8 rounded-full bg-[#21262d] border border-[#30363d] flex items-center justify-center text-xs font-bold text-purple-400">
                  {user.username.slice(0, 2).toUpperCase()}
                </div>
              )}
              <span className="font-medium hidden sm:inline">{user.username}</span>
            </div>

            <button
              onClick={onOpenHistory}
              title="Rating History"
              className="p-2 text-[#8b949e] hover:text-[#f0f6fc] hover:bg-[#21262d] rounded-lg transition-colors cursor-pointer"
            >
              <History className="h-4 w-4" />
            </button>

            <button
              onClick={onLogout}
              title="Logout"
              className="p-2 text-[#8b949e] hover:text-red-400 hover:bg-[#21262d] rounded-lg transition-colors cursor-pointer"
            >
              <LogOut className="h-4 w-4" />
            </button>
          </div>
        ) : (
          <button
            onClick={onOpenAuth}
            className="flex items-center gap-2 bg-[#21262d] hover:bg-[#30363d] border border-[#30363d] text-white font-medium px-4 py-2 rounded-lg transition-all cursor-pointer"
          >
            <User className="h-4 w-4 text-purple-400" />
            <span>Sign In</span>
          </button>
        )}
      </div>
    </header>
  );
}
