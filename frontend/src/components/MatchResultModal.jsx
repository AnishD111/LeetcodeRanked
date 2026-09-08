import React from 'react';
import { Trophy, Skull, ArrowRight, RotateCcw } from 'lucide-react';

export default function MatchResultModal({ result, onClose, onFindNewMatch }) {
  if (!result) return null;

  const isWin = result.isWin;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-md p-4 animate-in fade-in zoom-in-95 duration-200">
      <div className="bg-[#161b22] border border-[#30363d] rounded-3xl w-full max-w-md p-8 shadow-2xl text-center relative overflow-hidden">
        <div
          className={`absolute -top-24 left-1/2 -translate-x-1/2 w-64 h-64 rounded-full blur-3xl opacity-20 pointer-events-none ${
            isWin ? 'bg-emerald-500' : 'bg-rose-500'
          }`}
        />

        <div
          className={`h-20 w-20 mx-auto rounded-2xl flex items-center justify-center mb-6 shadow-xl ${
            isWin
              ? 'bg-gradient-to-tr from-emerald-600 to-teal-500 text-white shadow-emerald-500/30'
              : 'bg-gradient-to-tr from-rose-600 to-red-700 text-white shadow-rose-500/30'
          }`}
        >
          {isWin ? <Trophy className="h-10 w-10" /> : <Skull className="h-10 w-10" />}
        </div>

        <h2 className="text-3xl font-black text-white tracking-tight mb-1">
          {isWin ? 'VICTORY' : 'DEFEAT'}
        </h2>
        <p className="text-sm text-[#8b949e] mb-6">
          {isWin
            ? 'All test cases passed ahead of your opponent!'
            : result.reason || 'Your opponent solved the challenge first.'}
        </p>

        {result.oldElo !== undefined && result.newElo !== undefined && (
          <div className="bg-[#0d1117] border border-[#30363d] rounded-2xl p-4 mb-6 flex items-center justify-around">
            <div>
              <span className="text-xs text-[#8b949e] block mb-1">Previous</span>
              <span className="text-lg font-bold text-white">{result.oldElo}</span>
            </div>

            <ArrowRight className="h-5 w-5 text-[#8b949e]" />

            <div>
              <span className="text-xs text-[#8b949e] block mb-1">New Rating</span>
              <span className="text-lg font-bold text-white">{result.newElo}</span>
            </div>

            <div
              className={`px-3 py-1 rounded-xl text-sm font-extrabold ${
                isWin ? 'bg-emerald-500/10 text-emerald-400' : 'bg-rose-500/10 text-rose-400'
              }`}
            >
              {result.delta >= 0 ? `+${result.delta}` : result.delta}
            </div>
          </div>
        )}

        <div className="flex items-center gap-3">
          <button
            onClick={onClose}
            className="flex-1 bg-[#21262d] hover:bg-[#30363d] border border-[#30363d] text-white py-3 rounded-xl font-semibold text-sm transition-all cursor-pointer"
          >
            Review Code
          </button>
          <button
            onClick={() => {
              onClose();
              onFindNewMatch();
            }}
            className="flex-1 flex items-center justify-center gap-2 bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-500 hover:to-indigo-500 text-white py-3 rounded-xl font-semibold text-sm shadow-lg shadow-purple-600/30 transition-all cursor-pointer"
          >
            <RotateCcw className="h-4 w-4" />
            <span>Play Again</span>
          </button>
        </div>
      </div>
    </div>
  );
}
