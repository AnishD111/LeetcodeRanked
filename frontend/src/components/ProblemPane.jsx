import React from 'react';
import { BookOpen, Flame } from 'lucide-react';

const DIFFICULTY_STYLES = {
  Easy: 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30',
  Medium: 'bg-amber-500/10 text-amber-400 border-amber-500/30',
  Hard: 'bg-rose-500/10 text-rose-400 border-rose-500/30',
};

export default function ProblemPane({ problem }) {
  if (!problem) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 text-center text-[#8b949e]">
        <BookOpen className="h-12 w-12 text-[#30363d] mb-3" />
        <h3 className="text-lg font-semibold text-[#f0f6fc]">Waiting for Match</h3>
        <p className="text-sm max-w-sm mt-1">
          Click "Find Match" to be paired with an opponent and receive a ranked LeetCode challenge.
        </p>
      </div>
    );
  }

  const diffClass = DIFFICULTY_STYLES[problem.difficulty] || DIFFICULTY_STYLES.Medium;

  return (
    <div className="h-full flex flex-col bg-[#161b22] border-r border-[#30363d] overflow-hidden">
      <div className="p-5 border-b border-[#30363d] bg-[#161b22]">
        <div className="flex items-center gap-3 mb-2">
          <span className={`text-xs font-semibold px-2.5 py-0.5 rounded-full border ${diffClass}`}>
            {problem.difficulty || 'Medium'}
          </span>
          <span className="text-xs text-[#8b949e] flex items-center gap-1">
            <Flame className="h-3.5 w-3.5 text-orange-400" />
            1v1 Ranked Match
          </span>
        </div>
        <h1 className="text-xl font-bold text-white tracking-tight">
          {problem.title}
        </h1>
      </div>

      <div className="flex-1 overflow-y-auto p-6 space-y-4">
        {problem.description ? (
          <div
            className="problem-content text-[#c9d1d9]"
            dangerouslySetInnerHTML={{ __html: problem.description }}
          />
        ) : (
          <p className="text-sm text-[#8b949e]">No description available for this problem.</p>
        )}
      </div>
    </div>
  );
}
