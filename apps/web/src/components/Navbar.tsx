import React from 'react';
import { cn } from '@/lib/utils';

export type ActiveTab = 'live' | 'detail' | 'analytics' | 'howitworks' | 'strategies' | 'lab';

interface NavbarProps {
  activeTab: ActiveTab;
  setActiveTab: (tab: ActiveTab) => void;
  isLiveConnected: boolean;
  hitRatePct: number | null;
}

const NAV_ITEMS: { key: ActiveTab; label: string }[] = [
  { key: 'live', label: 'Ao vivo' },
  { key: 'lab', label: 'Testar cenário' },
  { key: 'analytics', label: 'Analytics' },
  { key: 'strategies', label: 'Strategy Lab' },
  { key: 'howitworks', label: 'Como funciona' },
];

export const Navbar: React.FC<NavbarProps> = ({ activeTab, setActiveTab, isLiveConnected, hitRatePct }) => {
  return (
    <header className="sticky top-0 z-50 bg-[#020617]/90 backdrop-blur-md border-b border-white/[0.08]">
      <div className="max-w-[1400px] mx-auto px-5 sm:px-8 h-14 flex items-center gap-8">

        {/* Wordmark */}
        <button onClick={() => setActiveTab('live')} className="flex items-baseline gap-2 shrink-0 group">
          <span className="text-[17px] font-semibold tracking-[-0.02em] text-slate-50">Golo</span>
          <span className="font-mono text-[10px] uppercase tracking-wider text-slate-600 group-hover:text-slate-500 transition-colors hidden sm:inline">
            Probability Engine
          </span>
        </button>

        {/* Primary nav */}
        <nav className="flex items-center gap-0.5 flex-1 min-w-0 overflow-x-auto">
          {NAV_ITEMS.map(({ key, label }) => {
            const isActive = activeTab === key || (key === 'live' && activeTab === 'detail');
            return (
              <button
                key={key}
                onClick={() => setActiveTab(key)}
                className={cn(
                  'relative px-3 py-2 text-[13px] whitespace-nowrap transition-colors',
                  isActive ? 'text-slate-50' : 'text-slate-500 hover:text-slate-300'
                )}
              >
                {label}
                {isActive && <span className="absolute inset-x-3 -bottom-px h-px bg-emerald-400" />}
              </button>
            );
          })}
        </nav>

        {/* Status cluster */}
        <div className="flex items-center gap-4 shrink-0 font-mono text-[11px] tabular-nums">
          {hitRatePct !== null && (
            <span className="hidden md:flex items-baseline gap-1.5 text-slate-600" title="Acerto histórico das previsões resolvidas">
              <span className="uppercase tracking-wider">Acerto</span>
              <span className="text-slate-300">{Math.round(hitRatePct)}%</span>
            </span>
          )}
          <span className="flex items-center gap-1.5" title={isLiveConnected ? 'Conectado ao motor ao vivo' : 'Sem conexão com o servidor — o que estiver na tela pode estar desatualizado'}>
            <span
              className={cn(
                'w-1.5 h-1.5 rounded-full',
                isLiveConnected ? 'bg-emerald-400 animate-pulse' : 'bg-amber-400'
              )}
            />
            <span className={cn('uppercase tracking-wider', isLiveConnected ? 'text-slate-500 hidden sm:inline' : 'text-amber-400/80')}>
              {isLiveConnected ? 'Ao vivo' : 'Sem conexão'}
            </span>
          </span>
        </div>

      </div>
    </header>
  );
};
