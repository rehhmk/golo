import React, { useState, useEffect } from 'react';
import { Navbar } from './components/Navbar';
import { LiveBoard } from './components/LiveBoard';
import { MatchDetail } from './components/MatchDetail';
import { ReplayControl } from './components/ReplayControl';
import { EvaluationDashboard } from './components/EvaluationDashboard';
import { MatchUpdate, Prediction } from './types';
import { API_BASE_URL } from './config';

// Fallback mock matches for standalone preview
const MOCK_MATCHES: MatchUpdate[] = [
  {
    state: {
      matchId: 'live_match_101',
      status: 'LIVE',
      period: 2,
      clockSeconds: 4080, // 68th min
      score: { home: 1, away: 1 },
      redCards: { home: 0, away: 0 },
      yellowCards: { home: 2, away: 1 },
      substitutions: { home: 1, away: 2 },
      provider: 'mock_feed',
      stateVersion: 45,
      homeTeamId: 'Arsenal',
      awayTeamId: 'Chelsea',
      competitionId: 'Premier League',
      feedLagMs: 250,
      windows: {
        600: {
          windowSeconds: 600,
          home: { shots: 5, shotsOnTarget: 3, shotsBlocked: 1, xg: 0.84, corners: 4, fouls: 2, dangerousAttacks: 12 },
          away: { shots: 1, shotsOnTarget: 0, shotsBlocked: 0, xg: 0.06, corners: 1, fouls: 4, dangerousAttacks: 3 },
        },
      },
    },
    prediction: {
      matchId: 'live_match_101',
      asOfMatchSecond: 4080,
      calculatedAt: new Date().toISOString(),
      probabilities: {
        goalNext5m: 0.48,
        goalNext10m: 0.82,
        goalBeforeFullTime: 0.94,
      },
      dataQuality: 0.96,
      confidenceBand: 'HIGH',
      status: 'OK',
      modelVersion: 'baseline_v1.0.0',
      calibratorVersion: 'platt_v1',
      featureVersion: 'v1.0.0',
      predictionSequence: 142,
    },
    trackRecord: { accuracyPct: 75, resolvedCount: 8 },
    timestamp: new Date().toISOString(),
  },
  {
    state: {
      matchId: 'live_match_102',
      status: 'LIVE',
      period: 1,
      clockSeconds: 2100, // 35th min
      score: { home: 0, away: 0 },
      redCards: { home: 0, away: 1 },
      yellowCards: { home: 1, away: 3 },
      substitutions: { home: 0, away: 0 },
      provider: 'mock_feed',
      stateVersion: 28,
      homeTeamId: 'Flamengo',
      awayTeamId: 'Palmeiras',
      competitionId: 'Brasileirão Série A',
      feedLagMs: 180,
      windows: {
        600: {
          windowSeconds: 600,
          home: { shots: 3, shotsOnTarget: 2, shotsBlocked: 1, xg: 0.42, corners: 2, fouls: 3, dangerousAttacks: 8 },
          away: { shots: 2, shotsOnTarget: 1, shotsBlocked: 0, xg: 0.18, corners: 1, fouls: 5, dangerousAttacks: 5 },
        },
      },
    },
    prediction: {
      matchId: 'live_match_102',
      asOfMatchSecond: 2100,
      calculatedAt: new Date().toISOString(),
      probabilities: {
        goalNext5m: 0.32,
        goalNext10m: 0.64,
        goalBeforeFullTime: 0.88,
      },
      dataQuality: 0.92,
      confidenceBand: 'HIGH',
      status: 'OK',
      modelVersion: 'baseline_v1.0.0',
      calibratorVersion: 'platt_v1',
      featureVersion: 'v1.0.0',
      predictionSequence: 95,
    },
    trackRecord: { accuracyPct: 60, resolvedCount: 5 },
    timestamp: new Date().toISOString(),
  },
  {
    state: {
      matchId: 'live_match_103',
      status: 'LIVE',
      period: 2,
      clockSeconds: 4440, // 74th min
      score: { home: 2, away: 1 },
      redCards: { home: 0, away: 0 },
      yellowCards: { home: 0, away: 1 },
      substitutions: { home: 2, away: 3 },
      provider: 'mock_feed',
      stateVersion: 62,
      homeTeamId: 'Real Madrid',
      awayTeamId: 'Manchester City',
      competitionId: 'UEFA Champions League',
      feedLagMs: 310,
      windows: {
        600: {
          windowSeconds: 600,
          home: { shots: 2, shotsOnTarget: 1, shotsBlocked: 0, xg: 0.15, corners: 1, fouls: 1, dangerousAttacks: 4 },
          away: { shots: 2, shotsOnTarget: 1, shotsBlocked: 0, xg: 0.22, corners: 3, fouls: 2, dangerousAttacks: 7 },
        },
      },
    },
    prediction: {
      matchId: 'live_match_103',
      asOfMatchSecond: 4440,
      calculatedAt: new Date().toISOString(),
      probabilities: {
        goalNext5m: 0.18,
        goalNext10m: 0.38,
        goalBeforeFullTime: 0.62,
      },
      dataQuality: 0.95,
      confidenceBand: 'HIGH',
      status: 'OK',
      modelVersion: 'baseline_v1.0.0',
      calibratorVersion: 'platt_v1',
      featureVersion: 'v1.0.0',
      predictionSequence: 180,
    },
    trackRecord: { accuracyPct: 0, resolvedCount: 0 },
    timestamp: new Date().toISOString(),
  },
];

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'live' | 'detail' | 'replay' | 'evaluation'>('live');
  const [matches, setMatches] = useState<MatchUpdate[]>(MOCK_MATCHES);
  const [selectedMatchId, setSelectedMatchId] = useState<string>('live_match_101');
  const [predictionsHistory, setPredictionsHistory] = useState<Prediction[]>([]);
  const [isLiveConnected, setIsLiveConnected] = useState(false);
  const [hitRatePct, setHitRatePct] = useState<number | null>(null);

  // Poll HTTP API for live match updates
  useEffect(() => {
    const fetchMatches = () => {
      fetch(`${API_BASE_URL}/api/matches`)
        .then((res) => res.json())
        .then((data: MatchUpdate[]) => {
          if (Array.isArray(data) && data.length > 0) {
            setMatches(data);
            setIsLiveConnected(true);
          }
        })
        .catch(() => {
          setIsLiveConnected(false);
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
          if (typeof data.hitRatePct === 'number' && data.totalSnapshots > 0) {
            setHitRatePct(data.hitRatePct);
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
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col font-sans">
      <Navbar activeTab={activeTab} setActiveTab={setActiveTab} isLiveConnected={isLiveConnected} hitRatePct={hitRatePct} />

      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {activeTab === 'live' && (
          <LiveBoard matches={matches} onSelectMatch={handleSelectMatch} />
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
            <LiveBoard matches={matches} onSelectMatch={handleSelectMatch} />
          </div>
        )}

        {activeTab === 'evaluation' && <EvaluationDashboard />}
      </main>

      {/* Footer */}
      <footer className="bg-slate-900 border-t border-slate-800 py-6 text-center text-xs text-slate-500 font-mono">
        <p>Golo © 2026 — Real-Time Football Probability Engine MVP. Strictly for quantitative research & auditability.</p>
      </footer>
    </div>
  );
};

export default App;
