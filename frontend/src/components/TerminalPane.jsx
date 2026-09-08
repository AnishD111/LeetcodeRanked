import React, { useEffect, useRef } from 'react';
import { Terminal, ShieldAlert, CheckCircle2, XCircle, Info } from 'lucide-react';

export default function TerminalPane({ logs, isEvaluating }) {
  const scrollRef = useRef(null);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs]);

  return (
    <div className="h-full flex flex-col bg-[#0d1117] border-t border-[#30363d] overflow-hidden">
      <div className="px-4 py-2 bg-[#161b22] border-b border-[#30363d] flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Terminal className="h-4 w-4 text-purple-400" />
          <span className="text-xs font-semibold text-[#f0f6fc]">Execution Output</span>
        </div>
        {isEvaluating && (
          <div className="flex items-center gap-1.5 text-xs text-yellow-400">
            <span className="h-2 w-2 rounded-full bg-yellow-400 animate-ping" />
            <span>Evaluating in Docker sandbox...</span>
          </div>
        )}
      </div>

      <div
        ref={scrollRef}
        className="flex-1 p-4 font-mono text-xs overflow-y-auto space-y-1.5 bg-[#0d1117]"
      >
        {logs.length === 0 ? (
          <p className="text-[#8b949e]">Console output and test results will appear here...</p>
        ) : (
          logs.map((log, index) => {
            let textColor = 'text-[#c9d1d9]';
            let Icon = Info;

            if (log.type === 'success') {
              textColor = 'text-emerald-400 font-semibold';
              Icon = CheckCircle2;
            } else if (log.type === 'error' || log.type === 'failure') {
              textColor = 'text-rose-400 font-semibold';
              Icon = XCircle;
            } else if (log.type === 'warning') {
              textColor = 'text-amber-400';
              Icon = ShieldAlert;
            }

            return (
              <div key={index} className={`flex items-start gap-2 ${textColor}`}>
                <Icon className="h-3.5 w-3.5 mt-0.5 shrink-0 opacity-80" />
                <pre className="whitespace-pre-wrap break-all font-mono leading-relaxed">{log.text}</pre>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
