import React, { useState, useMemo } from 'react';
import { GameCard } from './GameCard';
import { MatchUpdate } from '../types';
import { Flame, ArrowUpDown, Search, Filter, ShieldCheck } from 'lucide-react';

interface LiveBoardProps {
  matches: MatchUpdate[];
  onSelectMatch: (matchId: string) => void;
}

export const LiveBoard: React.FC<LiveBoardProps> = ({ matches, onSelectMatch }) => {
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

  return (
    <div className="space-y-6">
      {/* Live Board Top Banner */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 relative overflow-hidden">
        <div className="absolute -right-10 -bottom-10 w-48 h-48 bg-emerald-500/10 rounded-full blur-3xl pointer-events-none" />
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 relative z-10">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-extrabold text-slate-50 font-sans tracking-tight">Live Probability Board</h1>
              <span className="bg-rose-500/10 text-rose-500 text-xs font-mono font-bold px-2.5 py-0.5 rounded-full border border-rose-500/20">
                {matches.length} Partidas Ao Vivo
              </span>
            </div>
            <p className="text-sm text-slate-400 mt-1">
              Partidas monitoradas organizadas por intensidade estatística de gol nos próximos 10 minutos.
            </p>
          </div>

          <div className="flex items-center gap-3">
            <button
              onClick={() => setFilterHotOnly(!filterHotOnly)}
              className={`flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-bold font-mono transition-all border ${
                filterHotOnly
                  ? 'bg-emerald-500/20 text-emerald-400 border-emerald-500/40 shadow-lg shadow-emerald-500/10'
                  : 'bg-slate-950 text-slate-300 border-slate-800 hover:border-slate-700'
              }`}
            >
              <Flame className="w-4 h-4 text-emerald-400" />
              <span>HOT MATCHES ({hotCount})</span>
            </button>
          </div>
        </div>

        {/* Filter and Sorting Controls */}
        <div className="mt-6 pt-4 border-t border-slate-800/80 flex flex-col sm:flex-row gap-3 justify-between items-center">
          {/* Search Bar */}
          <div className="relative w-full sm:w-72">
            <Search className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Buscar time ou liga..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full bg-slate-950 border border-slate-800 rounded-xl pl-9 pr-4 py-2 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-slate-700"
            />
          </div>

          {/* Sort Buttons */}
          <div className="flex items-center gap-2 w-full sm:w-auto overflow-x-auto pb-1 sm:pb-0">
            <span className="text-xs text-slate-400 font-mono flex items-center gap-1">
              <ArrowUpDown className="w-3.5 h-3.5" /> Ordenar:
            </span>
            <button
              onClick={() => setSortBy('prob')}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono border transition-all ${
                sortBy === 'prob'
                  ? 'bg-slate-800 text-emerald-400 border-slate-700'
                  : 'bg-slate-950 text-slate-400 border-slate-800 hover:text-slate-200'
              }`}
            >
              Maior Probabilidade (10m)
            </button>
            <button
              onClick={() => setSortBy('momentum')}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono border transition-all ${
                sortBy === 'momentum'
                  ? 'bg-slate-800 text-emerald-400 border-slate-700'
                  : 'bg-slate-950 text-slate-400 border-slate-800 hover:text-slate-200'
              }`}
            >
              Momentum (5m Surge)
            </button>
            <button
              onClick={() => setSortBy('league')}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono border transition-all ${
                sortBy === 'league'
                  ? 'bg-slate-800 text-emerald-400 border-slate-700'
                  : 'bg-slate-950 text-slate-400 border-slate-800 hover:text-slate-200'
              }`}
            >
              Por Competição
            </button>
          </div>
        </div>
      </div>

      {/* Grid of Game Cards */}
      {filteredAndSortedMatches.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
          {filteredAndSortedMatches.map((m) => (
            <GameCard key={m.state.matchId} matchUpdate={m} onSelect={onSelectMatch} />
          ))}
        </div>
      ) : (
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
          <Filter className="w-10 h-10 text-slate-600 mx-auto mb-3" />
          <h3 className="text-lg font-bold text-slate-300">Nenhuma partida encontrada</h3>
          <p className="text-xs text-slate-500 mt-1">
            Tente ajustar seus termos de busca ou remover os filtros aplicados.
          </p>
        </div>
      )}
    </div>
  );
};
