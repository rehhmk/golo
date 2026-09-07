import React from 'react';
import { MatchUpdate } from '../types';
import { Sparkline } from '@/components/ui/sparkline';
import { awayLabel, cn, competitionLabel, homeLabel } from "@/lib/utils"
interface GameCardProps {
  matchUpdate: MatchUpdate;
  onSelect: (matchId: string) => void;
  history?: number[];
}

type Tier = 'high' | 'mid' | 'low';

const TIER_STYLES: Record<Tier, { text: string; accent: string; rule: string }> = {
  high: { text: 'text-emerald-400', accent: 'bg-emerald-400', rule: 'text-emerald-400' },
  mid: { text: 'text-amber-400', accent: 'bg-amber-400', rule: 'text-amber-400' },
  low: { text: 'text-slate-300', accent: 'bg-slate-700', rule: 'text-slate-500' },
};

// The provider grades how good the incoming data is, not how sure the model
// is about football. Printing the raw enum invited the opposite reading.
function confidenceLabel(band: string): string {
  switch (band) {
    case 'HIGH':
      return 'bons';
    case 'MEDIUM':
      return 'medianos';
    case 'LOW':
      return 'fracos';
    default:
      return band.toLowerCase();
  }
}

export const GameCard: React.FC<GameCardProps> = ({ matchUpdate, onSelect, history = [] }) => {
  const { state, prediction, trackRecord } = matchUpdate;

  const minute = Math.floor(state.clockSeconds / 60);
  const prob10m = Math.round(prediction.probabilities.goalNext10m * 100);
  const prob5m = Math.round(prediction.probabilities.goalNext5m * 100);
  const probFT = Math.round(prediction.probabilities.goalBeforeFullTime * 100);

  const tier: Tier = prob10m >= 75 ? 'high' : prob10m >= 45 ? 'mid' : 'low';
  const styles = TIER_STYLES[tier];

  const stats10m = state.windows?.[600];
  const xgTotal = stats10m ? (stats10m.home.xg + stats10m.away.xg).toFixed(2) : '0.00';
  const shotsOnTarget = stats10m ? stats10m.home.shotsOnTarget + stats10m.away.shotsOnTarget : 0;
  const corners = stats10m ? stats10m.home.corners + stats10m.away.corners : 0;

  const hasRedCard = state.redCards.home > 0 || state.redCards.away > 0;

  return (
    <button
      onClick={() => onSelect(state.matchId)}
      className="group relative w-full text-left bg-[#0B1221] hover:bg-[#0E1729] transition-colors overflow-hidden rounded-md ring-1 ring-white/[0.06] hover:ring-white/[0.12] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400/50"
    >
      {/* Probability tier accent — the only chrome that carries meaning */}
      <span className={cn('absolute left-0 top-0 h-full w-[2px]', styles.accent)} />

      <div className="pl-4 pr-4 py-3.5">
        {/* Meta line */}
        <div className="flex items-baseline justify-between gap-3 mb-3">
          <div className="flex items-baseline gap-2 min-w-0">
            <span className="font-mono text-[11px] tabular-nums text-rose-400 shrink-0">{minute}&apos;</span>
            <span className="text-[11px] text-slate-500 truncate">{competitionLabel(state)}</span>
          </div>
          <span
            className="font-mono text-[10px] uppercase tracking-wider text-slate-600 shrink-0"
            title="Qualidade dos dados recebidos desta partida — não a confiança na previsão"
          >
            dados: {confidenceLabel(prediction.confidenceBand)}
          </span>
        </div>

        {/* Teams + score */}
        <div className="space-y-1 mb-3.5">
          <TeamRow name={homeLabel(state)} score={state.score.home} red={state.redCards.home > 0} />
          <TeamRow name={awayLabel(state)} score={state.score.away} red={state.redCards.away > 0} />
        </div>

        {/* Pressure sparkline — real rolling history of this match's own probability */}
        <div className={cn('mb-3', styles.rule)}>
          <Sparkline values={history} height={26} />
        </div>

        {/* Focal metric */}
        <div className="flex items-end justify-between gap-3">
          <div className="min-w-0">
            <div className="font-mono text-[10px] uppercase tracking-wider text-slate-500 mb-0.5">Gol · 10 min</div>
            <div className="flex items-baseline gap-2 font-mono text-[11px] tabular-nums text-slate-500">
              <span>5min {prob5m}%</span>
              <span className="text-slate-700">·</span>
              <span>até o fim {probFT}%</span>
            </div>
          </div>
          <div className={cn('font-mono text-[2rem] leading-none font-semibold tabular-nums', styles.text)}>
            {prob10m}
            <span className="text-base font-normal text-slate-600">%</span>
          </div>
        </div>

        {/* Supporting stats — quiet, inline, no boxes */}
        <div className="mt-3.5 pt-3 border-t border-white/[0.06] flex items-center justify-between gap-2 font-mono text-[11px] tabular-nums text-slate-500">
          <span title="Chutes no alvo nos últimos 10 minutos, somando os dois times">
            no alvo <span className="text-slate-300">{shotsOnTarget}</span>
          </span>
          <span title="Escanteios nos últimos 10 minutos, somando os dois times">
            escanteios <span className="text-slate-300">{corners}</span>
          </span>
          {hasRedCard && (
            <span className="text-rose-400" title="Há jogador expulso em campo">
              expulsão
            </span>
          )}
          {trackRecord && trackRecord.resolvedCount > 0 && (
            <span title={`Das nossas previsões de 10 minutos nesta partida que já se resolveram, ${Math.round(trackRecord.accuracyPct)}% acertaram (${trackRecord.resolvedCount} resolvidas)`}>
              acerto aqui <span className="text-slate-300">{Math.round(trackRecord.accuracyPct)}%</span>
            </span>
          )}
        </div>
      </div>
    </button>
  );
};

const TeamRow: React.FC<{ name: string; score: number; red: boolean }> = ({ name, score, red }) => (
  <div className="flex items-baseline justify-between gap-3">
    <div className="flex items-center gap-1.5 min-w-0">
      <span className="text-[13px] text-slate-100 truncate">{name}</span>
      {red && <span className="w-[3px] h-3 bg-rose-500 rounded-[1px] shrink-0" title="Cartão vermelho" />}
    </div>
    <span className="font-mono text-[15px] tabular-nums text-slate-100 shrink-0">{score}</span>
  </div>
);
