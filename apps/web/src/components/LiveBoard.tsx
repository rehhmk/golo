import React, { useState, useMemo } from 'react';
import { GameCard } from './GameCard';
import { MatchUpdate } from '../types';
import { Search } from 'lucide-react';
import { cn } from '@/lib/utils';
import { PageHeader } from './PageHeader';

interface LiveBoardProps {
  matches: MatchUpdate[];
  onSelectMatch: (matchId: string) => void;
  probHistory?: Record<string, number[]>;
}

export const LiveBoard: React.FC<LiveBoardProps> = ({ matches, onSelectMatch, probHistory = {} }) => {
  const [sortBy, setSortBy] = useState<'prob' | 'momentum' | 'league'>('prob');
  const [searchQuery, setSearchQuery] = useState('');
  const [filterHotOnly, setFilterHotOnly] = useState(false);

  const filteredAndSortedMatches = useMemo(() => {
    return matches
      .filter((m) => {
        const query = searchQuery.toLowerCase();
        const matchesQuery =
          m.state.homeTeamId.toLowerCase().includes(query) ||
          m.state.awayTeamId.toLowerCase().includes(query) ||
          m.state.competitionId.toLowerCase().includes(query);

        const isHot = m.prediction.probabilities.goalNext10m >= 0.75;
        return matchesQuery && (!filterHotOnly || isHot);
      })
      .sort((a, b) => {
        if (sortBy === 'prob') {
          return b.prediction.probabilities.goalNext10m - a.prediction.probabilities.goalNext10m;
        }
        if (sortBy === 'momentum') {
          return b.prediction.probabilities.goalNext5m - a.prediction.probabilities.goalNext5m;
        }
        return a.state.competitionId.localeCompare(b.state.competitionId);
      });
  }, [matches, sortBy, searchQuery, filterHotOnly]);

  const hotCount = matches.filter((m) => m.prediction.probabilities.goalNext10m >= 0.75).length;

  const sortOptions: { key: typeof sortBy; label: string }[] = [
    { key: 'prob', label: 'Probabilidade' },
    { key: 'momentum', label: 'Momentum' },
    { key: 'league', label: 'Competição' },
  ];

  return (
    <div>
      <PageHeader
        label="Ao vivo"
        title="Probabilidade de gol"
        description="Partidas ordenadas por intensidade estatística nos próximos 10 minutos."
        meta={
          <div className="flex items-baseline gap-4 font-mono text-[11px] tabular-nums text-slate-500">
            <span>
              <span className="text-slate-200">{matches.length}</span> partidas
            </span>
            <span>
              <span className={hotCount > 0 ? 'text-emerald-400' : 'text-slate-200'}>{hotCount}</span> em alta
            </span>
          </div>
        }
      />

      {/* Controls — a single quiet toolbar row, not a card */}
      <div className="flex flex-col sm:flex-row sm:items-center gap-3 justify-between mb-5">
        <div className="relative w-full sm:w-64">
          <Search className="w-3.5 h-3.5 text-slate-600 absolute left-0 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            placeholder="Buscar time ou liga"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full bg-transparent border-0 border-b border-white/[0.08] focus:border-emerald-400/50 pl-6 pr-2 py-1.5 text-[13px] text-slate-200 placeholder:text-slate-600 focus:outline-none transition-colors"
          />
        </div>

        <div className="flex items-center gap-1">
          <button
            onClick={() => setFilterHotOnly(!filterHotOnly)}
            className={cn(
              'font-mono text-[11px] uppercase tracking-wider px-2.5 py-1.5 rounded transition-colors',
              filterHotOnly ? 'text-emerald-400 bg-emerald-400/10' : 'text-slate-500 hover:text-slate-300'
            )}
          >
            Em alta
          </button>
          <span className="w-px h-3.5 bg-white/[0.08] mx-1.5" />
          {sortOptions.map((opt) => (
            <button
              key={opt.key}
              onClick={() => setSortBy(opt.key)}
              className={cn(
                'font-mono text-[11px] uppercase tracking-wider px-2.5 py-1.5 rounded transition-colors',
                sortBy === opt.key ? 'text-slate-100 bg-white/[0.06]' : 'text-slate-500 hover:text-slate-300'
              )}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      {filteredAndSortedMatches.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
          {filteredAndSortedMatches.map((m) => (
            <GameCard
              key={m.state.matchId}
              matchUpdate={m}
              onSelect={onSelectMatch}
              history={probHistory[m.state.matchId]}
            />
          ))}
        </div>
      ) : (
        <div className="py-20 text-center">
          <p className="text-[13px] text-slate-400">Nenhuma partida encontrada</p>
          <p className="text-[12px] text-slate-600 mt-1">Ajuste a busca ou remova os filtros.</p>
        </div>
      )}
    </div>
  );
};
