import React, { useState, useEffect } from 'react';
import { Navbar, ActiveTab } from './components/Navbar';
import { LiveBoard } from './components/LiveBoard';
import { MatchDetail } from './components/MatchDetail';
import { ReplayControl } from './components/ReplayControl';
import { EvaluationDashboard } from './components/EvaluationDashboard';
import { HowItWorks } from './components/HowItWorks';
import { StrategyLab } from './components/StrategyLab';
import { MatchUpdate, Prediction } from './types';
import { API_BASE_URL } from './config';

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<ActiveTab>('live');
  const [matches, setMatches] = useState<MatchUpdate[]>([]);
  const [selectedMatchId, setSelectedMatchId] = useState<string>('');
  // Distinguishes 'no answer yet' from 'answered, and there is nothing live'.
  // Without it the board cannot tell an empty league schedule from a broken
  // backend, and previously papered over both with fabricated matches.
  const [hasLoadedMatches, setHasLoadedMatches] = useState(false);
  const [predictionsHistory, setPredictionsHistory] = useState<Prediction[]>([]);
  const [isLiveConnected, setIsLiveConnected] = useState(false);
  const [hitRatePct, setHitRatePct] = useState<number | null>(null);
  // Rolling client-side history of each match's 10m probability, so the
  // board can draw a real pressure sparkline (blueprint §5A) without the
  // backend needing to serve a separate time series.
  const [probHistory, setProbHistory] = useState<Record<string, number[]>>({});

  // Poll HTTP API for live match updates
  useEffect(() => {
    const fetchMatches = () => {
      fetch(`${API_BASE_URL}/api/matches`)
        .then((res) => res.json())
        .then((data: MatchUpdate[]) => {
          if (!Array.isArray(data)) {
            return;
          }
          // An empty array is a valid answer, not a failure: it means the
          // backend is healthy and no match is currently being played. It
          // must reach the board, or the board keeps showing stale matches.
          setMatches(data);
          setIsLiveConnected(true);
          setHasLoadedMatches(true);
          if (data.length > 0) {
            setProbHistory((prev) => {
              const next = { ...prev };
              for (const m of data) {
                const series = next[m.state.matchId] ?? [];
                next[m.state.matchId] = [...series, m.prediction.probabilities.goalNext10m * 100].slice(-40);
              }
              return next;
            });
          }
        })
        .catch(() => {
          setIsLiveConnected(false);
          setHasLoadedMatches(true);
        });
    };

    fetchMatches();
    const interval = setInterval(fetchMatches, 3000);
    return () => clearInterval(interval);
  }, []);

  // Poll the "good at guessing" overall index less often — it changes far
  // slower than live match state, no need to hammer it every 3s.
  useEffect(() => {
    const fetchHitRate = () => {
      fetch(`${API_BASE_URL}/api/metrics`)
        .then((res) => res.json())
        .then((data) => {
          // Require both a resolved outcome and enough distinct matches behind
          // it. Predictions inside one match are not independent evidence, so a
          // headline accuracy drawn from two matches would be noise wearing a
          // percentage sign.
          if (typeof data.hitRatePct === 'number' && data.resolvedCount > 0 && data.matchCount >= 10) {
            setHitRatePct(data.hitRatePct);
          } else {
            setHitRatePct(null);
          }
        })
        .catch(() => {});
    };

    fetchHitRate();
    const interval = setInterval(fetchHitRate, 30000);
    return () => clearInterval(interval);
  }, []);

  const handleSelectMatch = (matchId: string) => {
    setSelectedMatchId(matchId);
    setActiveTab('detail');

    // Fetch history for selected match
    fetch(`${API_BASE_URL}/api/matches/${matchId}`)
      .then((res) => res.json())
      .then((data) => {
        if (data.predictions) {
          setPredictionsHistory(data.predictions);
        }
      })
      .catch(() => {});
  };

  const handleReplayAction = (action: string, speed?: string) => {
    fetch(`${API_BASE_URL}/api/replay/control`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action, speed }),
    }).catch(() => {});
  };

  const selectedMatchUpdate = matches.find((m) => m.state.matchId === selectedMatchId) || matches[0];

  return (
    <div className="min-h-screen bg-background text-foreground flex flex-col font-sans">
      <Navbar activeTab={activeTab} setActiveTab={setActiveTab} isLiveConnected={isLiveConnected} hitRatePct={hitRatePct} />

      <main className="flex-1 max-w-[1400px] w-full mx-auto px-5 sm:px-8 py-8">
        {activeTab === 'live' && (
          <LiveBoard
            matches={matches}
            onSelectMatch={handleSelectMatch}
            probHistory={probHistory}
            hasLoaded={hasLoadedMatches}
            isConnected={isLiveConnected}
          />
        )}

        {activeTab === 'detail' && (
          <MatchDetail
            matchUpdate={selectedMatchUpdate}
            predictionsHistory={predictionsHistory}
            onBack={() => setActiveTab('live')}
          />
        )}

        {activeTab === 'replay' && (
          <div className="space-y-6">
            <ReplayControl onControlAction={handleReplayAction} />
            <LiveBoard
            matches={matches}
            onSelectMatch={handleSelectMatch}
            probHistory={probHistory}
            hasLoaded={hasLoadedMatches}
            isConnected={isLiveConnected}
          />
          </div>
        )}

        {activeTab === 'howitworks' && <HowItWorks />}

        {activeTab === 'analytics' && <EvaluationDashboard matches={matches} />}

        {activeTab === 'strategies' && <StrategyLab />}
      </main>

      {/* Footer */}
      <footer className="border-t border-white/[0.08] mt-8">
        <div className="max-w-[1400px] mx-auto px-5 sm:px-8 py-5 flex flex-col sm:flex-row gap-2 sm:items-center justify-between">
          <p className="text-[11px] text-slate-600">
            Golo © 2026 — Ferramenta analítica. Probabilidades são estimativas, não garantias.
          </p>
          <p className="font-mono text-[10px] uppercase tracking-wider text-slate-700">
            Pesquisa quantitativa · Auditável
          </p>
        </div>
      </footer>
    </div>
  );
};

export default App;
