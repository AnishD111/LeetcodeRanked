import React, { useEffect, useState } from 'react';
import { X, TrendingUp, TrendingDown, History, Trophy, Calendar } from 'lucide-react';

const API_BASE = 'http://localhost:8080';

export default function EloHistoryModal({ isOpen, onClose }) {
  const [history, setHistory] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!isOpen) return;
    setLoading(true);

    fetch(`${API_BASE}/api/user/elo-history`, { credentials: 'include' })
      .then((res) => (res.ok ? res.json() : []))
      .then((data) => setHistory(data))
      .catch((err) => console.error('Failed to fetch Elo history:', err))
      .finally(() => setLoading(false));
  }, [isOpen]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
      <div className="bg-[#161b22] border border-[#30363d] rounded-2xl w-full max-w-lg p-6 shadow-2xl relative max-h-[85vh] flex flex-col">
        <div className="flex items-center justify-between pb-4 border-b border-[#30363d]">
          <div className="flex items-center gap-2.5">
            <div className="p-2 rounded-xl bg-purple-500/10 text-purple-400">
              <History className="h-5 w-5" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-white tracking-tight">Rating Progression</h2>
              <p className="text-xs text-[#8b949e]">Recent ranked matches and rating adjustments</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="text-[#8b949e] hover:text-white p-1 rounded-lg hover:bg-[#21262d] transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto py-4 space-y-2.5 pr-1">
          {loading ? (
            <p className="text-center text-sm text-[#8b949e] py-8">Loading history...</p>
          ) : history.length === 0 ? (
            <div className="text-center py-10 text-[#8b949e]">
              <Trophy className="h-10 w-10 text-[#30363d] mx-auto mb-2" />
              <p className="text-sm font-medium text-[#f0f6fc]">No match history yet</p>
              <p className="text-xs mt-1">Play a match to start building your competitive record.</p>
            </div>
          ) : (
            history.map((entry) => {
              const delta = entry.new_elo - entry.old_elo;
              const isGain = delta >= 0;
              const dateText = new Date(entry.recorded_at).toLocaleDateString(undefined, {
                month: 'short',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit',
              });

              return (
                <div
                  key={entry.id}
                  className="flex items-center justify-between p-3.5 rounded-xl bg-[#0d1117] border border-[#30363d] hover:border-[#8b949e]/30 transition-colors"
                >
                  <div className="flex items-center gap-3">
                    <div
                      className={`p-2 rounded-lg ${
                        isGain ? 'bg-emerald-500/10 text-emerald-400' : 'bg-rose-500/10 text-rose-400'
                      }`}
                    >
                      {isGain ? <TrendingUp className="h-4 w-4" /> : <TrendingDown className="h-4 w-4" />}
                    </div>
                    <div>
                      <div className="text-sm font-semibold text-white">
                        {entry.old_elo} <span className="text-[#8b949e]">➔</span> {entry.new_elo}
                      </div>
                      <div className="text-xs text-[#8b949e] flex items-center gap-1 mt-0.5">
                        <Calendar className="h-3 w-3" />
                        <span>{dateText}</span>
                      </div>
                    </div>
                  </div>

                  <span
                    className={`text-sm font-bold px-2.5 py-1 rounded-lg ${
                      isGain ? 'bg-emerald-500/10 text-emerald-400' : 'bg-rose-500/10 text-rose-400'
                    }`}
                  >
                    {isGain ? `+${delta}` : delta}
                  </span>
                </div>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
}
