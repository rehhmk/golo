import React from 'react';
import { Flame, Activity, Clock, ShieldAlert, Target } from 'lucide-react';
import { MatchUpdate } from '../types';

interface GameCardProps {
  matchUpdate: MatchUpdate;
  onSelect: (matchId: string) => void;
}

export const GameCard: React.FC<GameCardProps> = ({ matchUpdate, onSelect }) => {
  const { state, prediction, trackRecord } = matchUpdate;

  const minute = Math.floor(state.clockSeconds / 60);
  const prob10m = Math.round(prediction.probabilities.goalNext10m * 100);
  const prob5m = Math.round(prediction.probabilities.goalNext5m * 100);
  const probFT = Math.round(prediction.probabilities.goalBeforeFullTime * 100);

  const isHighProb = prob10m >= 75;
  const isMediumProb = prob10m >= 45 && prob10m < 75;

  const probColorClass = isHighProb
    ? 'text-emerald-400 border-emerald-500/40 bg-emerald-500/10 hot-match-glow'
    : isMediumProb
    ? 'text-amber-400 border-amber-500/30 bg-amber-500/10'
    : 'text-slate-400 border-slate-700 bg-slate-800/40';

  // Extract window 10m stats if present
  const stats10m = state.windows?.[600];
  const homeXG = stats10m ? stats10m.home.xg.toFixed(2) : '0.00';
  const awayXG = stats10m ? stats10m.away.xg.toFixed(2) : '0.00';
  const totalShotsOnTarget = stats10m ? stats10m.home.shotsOnTarget + stats10m.away.shotsOnTarget : 0;
  const totalCorners = stats10m ? stats10m.home.corners + stats10m.away.corners : 0;

  return (
    <div
      onClick={() => onSelect(state.matchId)}
      className="group bg-slate-900 border border-slate-800 hover:border-slate-700 rounded-xl p-5 transition-all shadow-lg hover:shadow-xl cursor-pointer relative overflow-hidden flex flex-col justify-between"
    >
      {/* Top Header Row */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <span className="flex items-center gap-1 text-xs font-semibold text-rose-500 bg-rose-500/10 px-2.5 py-0.5 rounded-full border border-rose-500/20 font-mono">
              <span className="w-1.5 h-1.5 rounded-full bg-rose-500 animate-ping" />
              {minute}'
            </span>
            <span className="text-xs text-slate-400 font-medium truncate max-w-[160px]">
              {state.competitionId}
            </span>
          </div>

          {isHighProb ? (
            <span className="flex items-center gap-1 text-xs font-extrabold text-emerald-400 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/30 font-mono">
              <Flame className="w-3.5 h-3.5 fill-emerald-400/20" /> HOT MATCH
            </span>
          ) : (
            <span className="text-[11px] font-mono text-slate-400 bg-slate-950 px-2 py-0.5 rounded border border-slate-800">
              Conf: {prediction.confidenceBand}
            </span>
          )}
        </div>

        {/* Score & Teams */}
        <div className="flex justify-between items-center my-3 bg-slate-950/60 p-3 rounded-lg border border-slate-800/80">
          <div className="space-y-1.5 font-semibold text-slate-100 text-sm">
            <div className="flex items-center gap-2">
              <span className="truncate">{state.homeTeamId}</span>
              {state.redCards.home > 0 && (
                <span className="w-2.5 h-3.5 bg-rose-600 rounded-sm inline-block" title="Red Card" />
              )}
            </div>
            <div className="flex items-center gap-2">
              <span className="truncate">{state.awayTeamId}</span>
              {state.redCards.away > 0 && (
                <span className="w-2.5 h-3.5 bg-rose-600 rounded-sm inline-block" title="Red Card" />
              )}
            </div>
          </div>

          <div className="font-mono text-xl font-extrabold text-slate-100 bg-slate-950 px-3.5 py-1.5 rounded-lg border border-slate-800 tracking-wider">
            {state.score.home} - {state.score.away}
          </div>
        </div>

        {/* Key Rolling Stats */}
        <div className="grid grid-cols-3 gap-2 my-3 text-center">
          <div className="bg-slate-950/40 p-2 rounded-lg border border-slate-800/50">
            <span className="text-[10px] text-slate-400 uppercase font-medium block">xG Acum (10m)</span>
            <span className="font-mono text-xs font-bold text-slate-200">{homeXG} - {awayXG}</span>
          </div>
          <div className="bg-slate-950/40 p-2 rounded-lg border border-slate-800/50">
            <span className="text-[10px] text-slate-400 uppercase font-medium block">Chutes Alvo</span>
            <span className="font-mono text-xs font-bold text-slate-200">{totalShotsOnTarget}</span>
          </div>
          <div className="bg-slate-950/40 p-2 rounded-lg border border-slate-800/50">
            <span className="text-[10px] text-slate-400 uppercase font-medium block">Escanteios</span>
            <span className="font-mono text-xs font-bold text-slate-200">{totalCorners}</span>
          </div>
        </div>
      </div>

      {/* Probability Metrics Bottom Bar */}
      <div className="mt-2 pt-3 border-t border-slate-800/80 flex items-center justify-between">
        <div className="flex items-center gap-1.5 text-xs text-slate-400">
          <Activity className="w-4 h-4 text-emerald-400" />
          <span className="font-medium">Gol Próx. 10m:</span>
        </div>

        <div className="flex items-center gap-3">
          <div className="text-right hidden sm:block">
            <span className="text-[10px] font-mono text-slate-400 block">5m: {prob5m}% | FT: {probFT}%</span>
          </div>
          <div className={`font-mono text-xl font-black px-3 py-1 rounded-lg border ${probColorClass}`}>
            {prob10m}%
          </div>
        </div>
      </div>

      {/* Track record: how often our own "gol nos próx. 10m" calls have been right so far in this match */}
      {trackRecord && trackRecord.resolvedCount > 0 && (
        <div className="mt-2 flex items-center justify-end gap-1 text-[10px] font-mono text-slate-500">
          <Target className="w-3 h-3" />
          <span>
            Acerto: {Math.round(trackRecord.accuracyPct)}% ({trackRecord.resolvedCount})
          </span>
        </div>
      )}
    </div>
  );
};
