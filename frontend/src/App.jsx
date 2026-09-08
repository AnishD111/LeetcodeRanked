import React, { useState, useEffect, useRef } from 'react';
import Navbar from './components/Navbar';
import AuthModal from './components/AuthModal';
import ProblemPane from './components/ProblemPane';
import MonacoEditorPane from './components/MonacoEditorPane';
import TerminalPane from './components/TerminalPane';
import EloHistoryModal from './components/EloHistoryModal';
import MatchResultModal from './components/MatchResultModal';

const API_BASE = 'http://localhost:8080';
const WS_BASE = 'ws://localhost:8080';

export default function App() {
  const [user, setUser] = useState(null);
  const [isAuthOpen, setIsAuthOpen] = useState(false);
  const [isHistoryOpen, setIsHistoryOpen] = useState(false);

  const [queueStatus, setQueueStatus] = useState('idle');
  const [currentProblem, setCurrentProblem] = useState(null);
  const [code, setCode] = useState('');
  const [defaultTemplate, setDefaultTemplate] = useState('');
  const [logs, setLogs] = useState([]);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [matchResult, setMatchResult] = useState(null);

  const socketRef = useRef(null);

  useEffect(() => {
    fetchUser();
  }, []);

  const fetchUser = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/auth/me`, { credentials: 'include' });
      if (res.ok) {
        const userData = await res.json();
        setUser(userData);
      } else {
        setUser(null);
      }
    } catch {
      setUser(null);
    }
  };

  const handleLogout = async () => {
    await fetch(`${API_BASE}/api/auth/logout`, { method: 'POST', credentials: 'include' });
    if (socketRef.current) {
      socketRef.current.close();
      socketRef.current = null;
    }
    setUser(null);
    setQueueStatus('idle');
    setCurrentProblem(null);
    setCode('');
    setLogs([]);
  };

  const handleFindMatch = () => {
    if (!user) {
      setIsAuthOpen(true);
      return;
    }

    setQueueStatus('queued');
    setLogs([{ type: 'system', text: 'Joining matchmaking queue... Searching for opponent...' }]);
    setMatchResult(null);

    const ws = new WebSocket(`${WS_BASE}/ws/join`);
    socketRef.current = ws;

    ws.onopen = () => {
      addLog('Connected to matchmaking coordinator.', 'system');
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'MATCH_FOUND') {
          setQueueStatus('in-match');
          setCurrentProblem({
            id: msg.problem_id,
            title: msg.problem_name,
            description: msg.description,
            difficulty: 'Medium',
          });

          const templateCode = msg.template || '# Write your solution here\n';
          setCode(templateCode);
          setDefaultTemplate(templateCode);
          addLog(msg.log_msg || 'Match started!', 'system');
          return;
        }
      } catch {
      }

      const text = event.data;
      if (text.includes('SUCCESS') || text.includes('YOU WIN')) {
        setIsSubmitting(false);
        addLog(text, 'success');

        const match = text.match(/Rating:\s*(\d+)\s*->\s*(\d+)\s*\(([\+\-]?\d+)\)/);
        if (match) {
          const oldElo = parseInt(match[1]);
          const newElo = parseInt(match[2]);
          const delta = parseInt(match[3]);
          setMatchResult({ isWin: true, oldElo, newElo, delta });
        } else {
          setMatchResult({ isWin: true });
        }
        fetchUser();
      } else if (text.includes('FAILURE') || text.includes('MATCH OVER') || text.includes('FAILED')) {
        setIsSubmitting(false);
        addLog(text, 'failure');

        if (text.includes('MATCH OVER') || text.includes('OPPONENT PASSED')) {
          const match = text.match(/Rating:\s*(\d+)\s*->\s*(\d+)\s*\(([\+\-]?\d+)\)/);
          if (match) {
            const oldElo = parseInt(match[1]);
            const newElo = parseInt(match[2]);
            const delta = parseInt(match[3]);
            setMatchResult({ isWin: false, oldElo, newElo, delta, reason: 'Opponent solved the problem first.' });
          } else {
            setMatchResult({ isWin: false, reason: 'Opponent solved the problem first.' });
          }
          fetchUser();
        }
      } else {
        addLog(text, 'system');
      }
    };

    ws.onclose = () => {
      addLog('Match connection closed.', 'warning');
      setQueueStatus('idle');
      fetchUser();
    };

    ws.onerror = () => {
      addLog('WebSocket encountered an error.', 'error');
    };
  };

  const handleCancelQueue = () => {
    if (socketRef.current) {
      socketRef.current.close();
      socketRef.current = null;
    }
    setQueueStatus('idle');
    addLog('Matchmaking cancelled.', 'system');
  };

  const handleSubmitCode = () => {
    if (!socketRef.current || socketRef.current.readyState !== WebSocket.OPEN) {
      addLog('Error: Not connected to an active match.', 'error');
      return;
    }

    setIsSubmitting(true);
    addLog('Sending submission to Docker sandbox...', 'system');
    socketRef.current.send(code);
  };

  const addLog = (text, type = 'system') => {
    setLogs((prev) => [...prev, { text, type, timestamp: new Date() }]);
  };

  return (
    <div className="min-h-screen bg-[#0d1117] flex flex-col">
      <Navbar
        user={user}
        onOpenAuth={() => setIsAuthOpen(true)}
        onLogout={handleLogout}
        onOpenHistory={() => setIsHistoryOpen(true)}
        queueStatus={queueStatus}
        onFindMatch={handleFindMatch}
        onCancelQueue={handleCancelQueue}
        inMatch={queueStatus === 'in-match'}
      />

      <main className="flex-1 grid grid-cols-1 lg:grid-cols-2 h-[calc(100vh-61px)] overflow-hidden">
        <div className="h-full overflow-hidden">
          <ProblemPane problem={currentProblem} />
        </div>

        <div className="h-full flex flex-col overflow-hidden">
          <div className="flex-1 h-[65%] overflow-hidden">
            <MonacoEditorPane
              code={code}
              onChange={(val) => setCode(val || '')}
              onSubmit={handleSubmitCode}
              isSubmitting={isSubmitting}
              disabled={queueStatus !== 'in-match'}
              onResetCode={() => setCode(defaultTemplate)}
            />
          </div>

          <div className="h-[35%] overflow-hidden">
            <TerminalPane logs={logs} isEvaluating={isSubmitting} />
          </div>
        </div>
      </main>

      <AuthModal
        isOpen={isAuthOpen}
        onClose={() => setIsAuthOpen(false)}
        onAuthSuccess={(u) => {
          setUser(u);
          setIsAuthOpen(false);
        }}
      />

      <EloHistoryModal
        isOpen={isHistoryOpen}
        onClose={() => setIsHistoryOpen(false)}
      />

      <MatchResultModal
        result={matchResult}
        onClose={() => setMatchResult(null)}
        onFindNewMatch={handleFindMatch}
      />
    </div>
  );
}
