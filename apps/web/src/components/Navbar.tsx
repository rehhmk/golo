import React from 'react';
import { Activity, PlayCircle, BarChart3, Radio, ShieldCheck, Target } from 'lucide-react';

interface NavbarProps {
  activeTab: 'live' | 'detail' | 'replay' | 'evaluation';
  setActiveTab: (tab: 'live' | 'detail' | 'replay' | 'evaluation') => void;
  isLiveConnected: boolean;
  hitRatePct: number | null;
}

export const Navbar: React.FC<NavbarProps> = ({ activeTab, setActiveTab, isLiveConnected, hitRatePct }) => {
  return (
    <header className="bg-slate-900 border-b border-slate-800 sticky top-0 z-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
        
        {/* Brand Logo & Title */}
        <div className="flex items-center gap-3 cursor-pointer" onClick={() => setActiveTab('live')}>
          <div className="w-10 h-10 rounded-xl bg-gradient-to-tr from-emerald-500 to-emerald-400 flex items-center justify-center shadow-lg shadow-emerald-500/20">
            <Activity className="w-6 h-6 text-slate-950 stroke-[2.5]" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <span className="font-extrabold text-xl tracking-tight text-slate-50 font-sans">GOLO</span>
              <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-mono font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                MVP v1.0
              </span>
            </div>
            <p className="text-xs text-slate-400 font-mono hidden sm:block">Real-Time Football Probability Engine</p>
          </div>
        </div>

        {/* Navigation Tabs */}
        <nav className="flex items-center gap-1 sm:gap-2">
          <button
            onClick={() => setActiveTab('live')}
            className={`flex items-center gap-2 px-3 py-2 rounded-lg text-xs sm:text-sm font-semibold transition-all ${
              activeTab === 'live'
                ? 'bg-slate-800 text-emerald-400 border border-slate-700 shadow-sm'
                : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'
            }`}
          >
            <Radio className="w-4 h-4" />
            <span>Live Board</span>
          </button>

          <button
            onClick={() => setActiveTab('replay')}
            className={`flex items-center gap-2 px-3 py-2 rounded-lg text-xs sm:text-sm font-semibold transition-all ${
              activeTab === 'replay'
                ? 'bg-slate-800 text-emerald-400 border border-slate-700 shadow-sm'
                : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'
            }`}
          >
            <PlayCircle className="w-4 h-4" />
            <span>Replay Engine</span>
          </button>

          <button
            onClick={() => setActiveTab('evaluation')}
            className={`flex items-center gap-2 px-3 py-2 rounded-lg text-xs sm:text-sm font-semibold transition-all ${
              activeTab === 'evaluation'
                ? 'bg-slate-800 text-emerald-400 border border-slate-700 shadow-sm'
                : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'
            }`}
          >
            <BarChart3 className="w-4 h-4" />
            <span>Avaliação</span>
          </button>
        </nav>

        {/* System Status Badge */}
        <div className="flex items-center gap-2">
          <div className={`w-2.5 h-2.5 rounded-full ${isLiveConnected ? 'bg-emerald-400 animate-pulse' : 'bg-amber-400'}`} />
          <span className="text-xs font-mono text-slate-400 hidden md:inline">
            {isLiveConnected ? 'SYSTEM LIVE' : 'OFFLINE / DEMO'}
          </span>
          {hitRatePct !== null && (
            <div
              className="hidden md:flex items-center gap-1 text-[11px] font-mono text-slate-400 bg-slate-950 px-2.5 py-1 rounded-lg border border-slate-800"
              title="% de previsões de gol resolvidas que acertaram o resultado"
            >
              <Target className="w-3.5 h-3.5 text-emerald-400" />
              <span>Acerto: {Math.round(hitRatePct)}%</span>
            </div>
          )}

          <div className="hidden lg:flex items-center gap-1 text-[11px] font-mono text-slate-400 bg-slate-950 px-2.5 py-1 rounded-lg border border-slate-800">
            <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />
            <span>Strict Audit</span>
          </div>
        </div>

      </div>
    </header>
  );
};
