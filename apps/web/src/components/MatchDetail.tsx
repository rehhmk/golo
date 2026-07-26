import React, { useState, useEffect } from 'react';
import { ArrowLeft, Activity, ShieldCheck, Clock, Flame, AlertTriangle, Layers } from 'lucide-react';
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts';
import { MatchUpdate, Prediction } from '../types';
import { awayLabel, competitionLabel, homeLabel } from "@/lib/utils"

interface MatchDetailProps {
  matchUpdate?: MatchUpdate;
  predictionsHistory?: Prediction[];
  onBack: () => void;
}

export const MatchDetail: React.FC<MatchDetailProps> = ({ matchUpdate, predictionsHistory = [], onBack }) => {
  if (!matchUpdate) {
    return (
      <div className="bg-slate-900 border border-slate-800 rounded-xl p-8 text-center">
        <p className="text-slate-400">Selecione uma partida no Live Board.</p>
        <button onClick={onBack} className="mt-4 px-4 py-2 bg-slate-800 text-slate-200 rounded-lg text-xs">
          Voltar
        </button>
      </div>
    );
  }

  const { state, prediction } = matchUpdate;
  const minute = Math.floor(state.clockSeconds / 60);

  const prob5m = Math.round(prediction.probabilities.goalNext5m * 100);
  const prob10m = Math.round(prediction.probabilities.goalNext10m * 100);
  const probFT = Math.round(prediction.probabilities.goalBeforeFullTime * 100);

  // Only real observations. This previously fell back to a fabricated +3pp
  // per minute ramp whenever history was short, drawn under the caption
  // "curva contínua de intensidade calculada pelo modelo" with nothing to
  // distinguish it from a genuine reading — and since history only arrives
  // from an async fetch, that invented line was what most readers saw first.
  const chartData = predictionsHistory.map((p) => ({
    minute: `${Math.floor(p.asOfMatchSecond / 60)}'`,
    prob10m: Math.round(p.probabilities.goalNext10m * 100),
    prob5m: Math.round(p.probabilities.goalNext5m * 100),
  }));
  const hasEnoughHistory = chartData.length >= 2;

  const stats10m = state.windows?.[600];

  return (
    <div className="space-y-6">
      {/* Back Button & Header */}
      <div className="flex items-center justify-between">
        <button
          onClick={onBack}
          className="flex items-center gap-2 px-3.5 py-2 bg-slate-900 border border-slate-800 hover:border-slate-700 text-slate-300 hover:text-slate-100 rounded-xl text-xs font-semibold transition-all"
        >
          <ArrowLeft className="w-4 h-4" />
          <span>Voltar ao Live Board</span>
        </button>

        <div className="flex items-center gap-2">
          <span className="text-xs font-mono text-slate-400 bg-slate-900 border border-slate-800 px-3 py-1.5 rounded-xl">
            Model: <strong className="text-slate-200">{prediction.modelVersion}</strong>
          </span>
        </div>
      </div>

      {/* Main Scorecard Header */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 sm:p-8 relative overflow-hidden">
        <div className="flex flex-col md:flex-row justify-between items-center gap-6">
          <div className="flex-1 text-center md:text-left">
            <div className="flex items-center justify-center md:justify-start gap-2 mb-2">
              <span className="text-xs font-semibold text-rose-500 bg-rose-500/10 px-2.5 py-0.5 rounded-full border border-rose-500/20 font-mono">
                LIVE {minute}'
              </span>
              <span className="text-xs text-slate-400 font-medium">{competitionLabel(state)}</span>
            </div>
            <h1 className="text-2xl sm:text-3xl font-black text-slate-50 tracking-tight">
              {homeLabel(state)} <span className="text-slate-500 font-normal">vs</span> {awayLabel(state)}
            </h1>
          </div>

          {/* Big Score Box */}
          <div className="bg-slate-950 border border-slate-800 rounded-2xl px-6 py-4 text-center">
            <span className="text-[10px] font-mono text-slate-500 uppercase tracking-widest block mb-1">Placar Ao Vivo</span>
            <div className="font-mono text-4xl sm:text-5xl font-black text-slate-100 tracking-widest">
              {state.score.home} - {state.score.away}
            </div>
          </div>
        </div>
      </div>

      {/* Probability Gauges (Multi-Horizon) */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">
          <div className="flex justify-between items-center mb-2">
            <span className="text-xs font-medium text-slate-400">Gol Próximos 5m</span>
            <span className="font-mono text-xs text-slate-500">Curto Prazo</span>
          </div>
          <div className="font-mono text-3xl font-extrabold text-amber-400">{prob5m}%</div>
          <div className="w-full bg-slate-950 rounded-full h-2 mt-3 overflow-hidden border border-slate-800">
            <div className="bg-amber-400 h-full rounded-full transition-all duration-500" style={{ width: `${prob5m}%` }} />
          </div>
        </div>

        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5 relative overflow-hidden hot-match-glow">
          <div className="flex justify-between items-center mb-2">
            <span className="text-xs font-bold text-slate-200 flex items-center gap-1">
              <Flame className="w-3.5 h-3.5 text-emerald-400" /> Gol Próximos 10m
            </span>
            <span className="font-mono text-xs text-emerald-400 font-bold">Horizonte Core</span>
          </div>
          <div className="font-mono text-4xl font-black text-emerald-400">{prob10m}%</div>
          <div className="w-full bg-slate-950 rounded-full h-2.5 mt-3 overflow-hidden border border-slate-800">
            <div className="bg-emerald-400 h-full rounded-full transition-all duration-500" style={{ width: `${prob10m}%` }} />
          </div>
        </div>

        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">
          <div className="flex justify-between items-center mb-2">
            <span className="text-xs font-medium text-slate-400">Gol Até o Fim (FT)</span>
            <span className="font-mono text-xs text-slate-500">Jogo Completo</span>
          </div>
          <div className="font-mono text-3xl font-extrabold text-slate-200">{probFT}%</div>
          <div className="w-full bg-slate-950 rounded-full h-2 mt-3 overflow-hidden border border-slate-800">
            <div className="bg-slate-400 h-full rounded-full transition-all duration-500" style={{ width: `${probFT}%` }} />
          </div>
        </div>
      </div>

      {/* Real-time Probability Trend Chart */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-base font-bold text-slate-100 flex items-center gap-2">
              <Activity className="w-4 h-4 text-emerald-400" /> Evolução da Probabilidade ao Longo do Jogo
            </h3>
            <p className="text-xs text-slate-400 font-mono">Curva contínua de intensidade calculada pelo modelo</p>
          </div>
        </div>

        {!hasEnoughHistory ? (
          <div className="h-64 w-full pt-4 flex flex-col items-center justify-center text-center">
            <p className="text-[13px] text-slate-400">Histórico ainda insuficiente</p>
            <p className="text-[12px] text-slate-600 mt-1.5 max-w-sm leading-relaxed">
              A curva aparece assim que houver ao menos duas leituras registradas
              desta partida.
            </p>
          </div>
        ) : (
        <div className="h-64 w-full pt-4">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData}>
              <defs>
                <linearGradient id="probGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#34D399" stopOpacity={0.4} />
                  <stop offset="95%" stopColor="#34D399" stopOpacity={0.0} />
                </linearGradient>
              </defs>
              <XAxis dataKey="minute" stroke="#475569" fontSize={11} tickLine={false} />
              <YAxis domain={[0, 100]} stroke="#475569" fontSize={11} tickFormatter={(v) => `${v}%`} tickLine={false} />
              <Tooltip
                contentStyle={{ backgroundColor: '#0F172A', borderColor: '#334155', borderRadius: '8px', color: '#F8FAFC' }}
                formatter={(val: number) => [`${val}%`, 'Prob. 10m']}
              />
              <Area type="monotone" dataKey="prob10m" stroke="#34D399" strokeWidth={3} fillOpacity={1} fill="url(#probGradient)" />
            </AreaChart>
          </ResponsiveContainer>
        </div>
        )}
      </div>

      {/* Rolling Stats & Feed Audit Row */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Rolling Window Stats (10m) */}
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">
          <h4 className="text-sm font-bold text-slate-200 mb-4 pb-2 border-b border-slate-800 flex items-center gap-2">
            <Layers className="w-4 h-4 text-emerald-400" /> Estatísticas dos Últimos 10 Minutos
          </h4>

          <div className="space-y-3 font-mono text-xs">
            <div className="flex justify-between items-center">
              <span className="text-slate-400">xG Acumulado:</span>
              <span className="text-slate-100 font-bold">{stats10m?.home.xg.toFixed(2) || '0.00'} vs {stats10m?.away.xg.toFixed(2) || '0.00'}</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Chutes Totais:</span>
              <span className="text-slate-100 font-bold">{stats10m?.home.shots || 0} vs {stats10m?.away.shots || 0}</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Chutes no Alvo:</span>
              <span className="text-slate-100 font-bold">{stats10m?.home.shotsOnTarget || 0} vs {stats10m?.away.shotsOnTarget || 0}</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Escanteios:</span>
              <span className="text-slate-100 font-bold">{stats10m?.home.corners || 0} vs {stats10m?.away.corners || 0}</span>
            </div>
          </div>
        </div>

        {/* Audit & Operational Quality */}
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">
          <h4 className="text-sm font-bold text-slate-200 mb-4 pb-2 border-b border-slate-800 flex items-center gap-2">
            <ShieldCheck className="w-4 h-4 text-emerald-400" /> Auditoria & Qualidade de Dados
          </h4>

          <div className="space-y-3 font-mono text-xs">
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Qualidade do Feed:</span>
              <span className="text-emerald-400 font-bold">{(prediction.dataQuality * 100).toFixed(0)}%</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Banda de Confiança:</span>
              <span className="text-slate-200 font-bold">{prediction.confidenceBand}</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Atraso Estimado (Lag):</span>
              <span className="text-slate-200 font-bold">{state.feedLagMs || 0} ms</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Versão de Features:</span>
              <span className="text-slate-200 font-bold">{prediction.featureVersion}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
