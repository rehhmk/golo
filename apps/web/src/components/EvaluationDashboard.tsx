import React, { useState, useEffect, useMemo } from 'react';
import {
  ComposedChart,
  Line,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
} from 'recharts';
import { EvaluationMetrics, MatchUpdate } from '../types';
import { API_BASE_URL } from '../config';
import { PageHeader } from './PageHeader';
import { awayLabel, cn, homeLabel } from "@/lib/utils"
interface EvaluationDashboardProps {
  matches: MatchUpdate[];
}

// Deterministic, rule-based reading of the raw rolling-window stats — ranked
// by how far each deviates from a neutral baseline. This is NOT the model's
// internal coefficients (those live server-side in the model artifact) and
// is NOT AI-generated: it's a reproducible reading of the same numbers a
// human analyst would look at first.
interface Factor {
  label: string;
  detail: string;
  direction: 'up' | 'down' | 'neutral';
  magnitude: number;
}

function computeFactors(m: MatchUpdate): Factor[] {
  const stats10m = m.state.windows?.[600];
  const factors: Factor[] = [];

  if (stats10m) {
    const xgDiff = stats10m.home.xg - stats10m.away.xg;
    factors.push({
      label: 'xG acumulado (10m)',
      detail: `${homeLabel(m.state)} ${stats10m.home.xg.toFixed(2)} · ${awayLabel(m.state)} ${stats10m.away.xg.toFixed(2)}`,
      direction: xgDiff > 0.05 ? 'up' : xgDiff < -0.05 ? 'down' : 'neutral',
      magnitude: Math.abs(xgDiff) * 3,
    });

    const sotTotal = stats10m.home.shotsOnTarget + stats10m.away.shotsOnTarget;
    factors.push({
      label: 'Chutes no alvo (10m)',
      detail: `${sotTotal} no total`,
      direction: sotTotal >= 3 ? 'up' : sotTotal === 0 ? 'down' : 'neutral',
      magnitude: sotTotal * 0.4,
    });

    const dangerousTotal = stats10m.home.dangerousAttacks + stats10m.away.dangerousAttacks;
    factors.push({
      label: 'Ataques perigosos (10m)',
      detail: `${dangerousTotal} registrados`,
      direction: dangerousTotal >= 8 ? 'up' : dangerousTotal <= 2 ? 'down' : 'neutral',
      magnitude: dangerousTotal * 0.15,
    });
  }

  const redTotal = m.state.redCards.home + m.state.redCards.away;
  if (redTotal > 0) {
    factors.push({
      label: 'Cartão vermelho',
      detail: `${redTotal} expulsão(ões) em campo`,
      direction: 'down',
      magnitude: redTotal * 1.5,
    });
  }

  const scoreDiff = Math.abs(m.state.score.home - m.state.score.away);
  factors.push(
    scoreDiff > 0
      ? {
          label: 'Diferença no placar',
          detail: `${scoreDiff} gol(s) — time atrás tende a pressionar`,
          direction: 'up',
          magnitude: scoreDiff * 0.6,
        }
      : { label: 'Jogo empatado', detail: 'Sem pressão adicional por placar', direction: 'neutral', magnitude: 0.1 }
  );

  return factors.sort((a, b) => b.magnitude - a.magnitude).slice(0, 4);
}

// Below this many distinct finished matches, the aggregate metrics rest on
// too few independent outcomes to be read as a measurement.
const MIN_MATCHES_FOR_CONFIDENCE = 10;

const EMPTY_METRICS: EvaluationMetrics = {
  brierScore: 0,
  logLoss: 0,
  ece: 0,
  hitRatePct: 0,
  calibrationCurve: [],
  totalSnapshots: 0,
  resolvedCount: 0,
  matchCount: 0,
  dataQualityAvg: 0,
  staleFeedPct: 0,
  modelVersion: '—',
  featureVersion: '—',
  evaluatedPeriod: '—',
};

export const EvaluationDashboard: React.FC<EvaluationDashboardProps> = ({ matches }) => {
  const [metrics, setMetrics] = useState<EvaluationMetrics | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [selectedMatchId, setSelectedMatchId] = useState<string | null>(null);

  useEffect(() => {
    fetch(`${API_BASE_URL}/api/metrics`)
      .then((res) => res.json())
      // Defensive defaults: an older backend build (or partial response) may
      // omit newer fields, and one missing number must not crash the page.
      .then((data) => {
        setMetrics({ ...EMPTY_METRICS, ...data });
        setLoadFailed(false);
      })
      .catch(() => {
        // Deliberately NOT falling back to zeroes. A dropped request used to
        // render Brier 0.000 and log loss 0.000 — the numbers a flawless model
        // would produce — as though they had been measured.
        setMetrics(null);
        setLoadFailed(true);
      });
  }, []);

  const selectedMatch = useMemo(() => {
    if (matches.length === 0) return null;
    if (selectedMatchId) {
      const found = matches.find((m) => m.state.matchId === selectedMatchId);
      if (found) return found;
    }
    return [...matches].sort(
      (a, b) => b.prediction.probabilities.goalNext10m - a.prediction.probabilities.goalNext10m
    )[0];
  }, [matches, selectedMatchId]);

  const factors = selectedMatch ? computeFactors(selectedMatch) : [];

  const chartData = useMemo(
    () =>
      (metrics?.calibrationCurve ?? []).map((row) => ({
        bin: row.bin,
        Previsto: Number((row.predicted * 100).toFixed(1)),
        Observado: Number((row.observed * 100).toFixed(1)),
        Amostras: row.count,
      })),
    [metrics]
  );

  if (!metrics) {
    if (loadFailed) {
      return (
        <div className="py-20 text-center">
          <p className="text-[13px] text-amber-300/90">Não foi possível carregar as métricas</p>
          <p className="text-[12px] text-slate-500 mt-1.5 max-w-md mx-auto leading-relaxed">
            O servidor não respondeu. Nenhum número é exibido aqui em vez de
            zeros, porque zero é indistinguível de um modelo perfeito.
          </p>
        </div>
      );
    }
    return <div className="py-20 text-center text-[13px] text-slate-500">Carregando métricas…</div>;
  }

  // Gate on what was actually measured, not on how many predictions were made.
  // Those differ by orders of magnitude: a prediction only becomes evidence
  // once its match has finished, so gating on the prediction count renders
  // zeroes for every metric as though the model had been measured and scored 0.
  const hasData = metrics.resolvedCount > 0;

  // Every prediction within one match shares that match's single outcome, so
  // the number of matches — not of samples — is the evidence actually behind
  // these figures. Below this many, the metrics are anecdote.
  const isThin = metrics.matchCount < MIN_MATCHES_FOR_CONFIDENCE;

  return (
    <div>
      <PageHeader
        label="Avaliação"
        title="Analytics"
        description="Calibração e erro das previsões, medidos contra resultados reais."
        meta={
          <div className="font-mono text-[11px] tabular-nums text-slate-500">
            <span className="text-slate-200">{metrics.resolvedCount.toLocaleString('pt-BR')}</span> resolvidas de{' '}
            <span className="text-slate-200">{metrics.matchCount}</span>{' '}
            {metrics.matchCount === 1 ? 'partida' : 'partidas'} · {metrics.modelVersion}
          </div>
        }
      />

      {/* An honest health warning beats a confident-looking number. Without it
          a reader takes "67,6% de acerto" at face value, unaware it rests on
          two match outcomes. */}
      {hasData && isThin && (
        <div className="mb-6 rounded-md border border-amber-500/25 bg-amber-500/[0.06] px-4 py-3">
          <p className="text-[13px] leading-relaxed text-amber-200/90">
            <span className="font-medium">Amostra insuficiente.</span> Estes números vêm de{' '}
            <span className="font-mono tabular-nums">{metrics.matchCount}</span>{' '}
            {metrics.matchCount === 1 ? 'partida encerrada' : 'partidas encerradas'}. Como todas as
            previsões de uma mesma partida dividem um único desfecho, o que pesa aqui é a
            quantidade de partidas — não a de previsões. Trate como indício, não como medida.
          </p>
        </div>
      )}

      {/* Metric row — no cards, just a ruled grid */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-px bg-white/[0.06] border border-white/[0.06] rounded-md overflow-hidden mb-8">
        <Metric
          label="Acerto"
          value={`${metrics.hitRatePct.toFixed(1)}%`}
          hint="Previsões de gol até o fim que acertaram"
          accent
        />
        <Metric label="Brier score" value={metrics.brierScore.toFixed(3)} hint="Erro médio ao quadrado · menor é melhor" />
        <Metric label="Log loss" value={metrics.logLoss.toFixed(3)} hint="Pune previsão confiante e errada" />
        <Metric label="Calibração (ECE)" value={`${(metrics.ece * 100).toFixed(1)}%`} hint="Quanto o previsto se afasta do observado" />
      </div>

      {/* Calibration */}
      <section className="mb-8">
        <SectionHeading
          title="Curva de calibração"
          description="Quanto mais próximas as linhas, melhor calibrado o modelo."
        />
        {hasData && chartData.length > 0 ? (
          <>
            <div className="h-64 w-full mt-4">
              <ResponsiveContainer width="100%" height="100%">
                <ComposedChart data={chartData} margin={{ top: 8, right: 8, left: -20, bottom: 0 }}>
                  <CartesianGrid stroke="rgba(255,255,255,0.05)" vertical={false} />
                  <XAxis
                    dataKey="bin"
                    tick={{ fill: '#64748B', fontSize: 11, fontFamily: 'JetBrains Mono' }}
                    axisLine={{ stroke: 'rgba(255,255,255,0.08)' }}
                    tickLine={false}
                  />
                  <YAxis
                    yAxisId="pct"
                    domain={[0, 100]}
                    unit="%"
                    tick={{ fill: '#64748B', fontSize: 11, fontFamily: 'JetBrains Mono' }}
                    axisLine={false}
                    tickLine={false}
                  />
                  <YAxis yAxisId="count" orientation="right" hide />
                  <RechartsTooltip
                    contentStyle={{
                      backgroundColor: '#0B1221',
                      border: '1px solid rgba(255,255,255,0.1)',
                      borderRadius: 6,
                      fontSize: 12,
                      fontFamily: 'JetBrains Mono',
                    }}
                    labelStyle={{ color: '#F8FAFC' }}
                    cursor={{ fill: 'rgba(255,255,255,0.03)' }}
                  />
                  <Bar yAxisId="count" dataKey="Amostras" fill="rgba(255,255,255,0.06)" radius={[2, 2, 0, 0]} barSize={32} />
                  <Line yAxisId="pct" type="monotone" dataKey="Previsto" stroke="#34D399" strokeWidth={1.5} dot={{ r: 2.5, fill: '#34D399' }} />
                  <Line yAxisId="pct" type="monotone" dataKey="Observado" stroke="#38BDF8" strokeWidth={1.5} dot={{ r: 2.5, fill: '#38BDF8' }} />
                </ComposedChart>
              </ResponsiveContainer>
            </div>

            <div className="flex items-center gap-5 mt-2 font-mono text-[11px] text-slate-500">
              <LegendDot color="#34D399" label="Previsto" />
              <LegendDot color="#38BDF8" label="Observado" />
              <LegendDot color="rgba(255,255,255,0.15)" label="Amostras" />
            </div>

            {/* Exact values */}
            <table className="w-full mt-6 text-left font-mono text-[12px] tabular-nums">
              <thead>
                <tr className="text-slate-600 border-b border-white/[0.08]">
                  <th className="font-normal py-2 pr-4">Faixa</th>
                  <th className="font-normal py-2 pr-4">Previsto</th>
                  <th className="font-normal py-2 pr-4">Observado</th>
                  <th className="font-normal py-2 pr-4">Amostras</th>
                  <th className="font-normal py-2">Desvio</th>
                </tr>
              </thead>
              <tbody>
                {metrics.calibrationCurve.map((row) => {
                  const diff = row.observed - row.predicted;
                  const calibrated = Math.abs(diff) <= 0.03;
                  return (
                    <tr key={row.bin} className="border-b border-white/[0.04]">
                      <td className="py-2 pr-4 text-slate-400">{row.bin}</td>
                      <td className="py-2 pr-4 text-slate-200">{(row.predicted * 100).toFixed(1)}%</td>
                      <td className="py-2 pr-4 text-slate-200">{(row.observed * 100).toFixed(1)}%</td>
                      <td className="py-2 pr-4 text-slate-500">{row.count}</td>
                      <td className={cn('py-2', calibrated ? 'text-slate-500' : 'text-amber-400')}>
                        {diff >= 0 ? '+' : ''}
                        {(diff * 100).toFixed(1)}pp
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </>
        ) : (
          <EmptyNote>
            Ainda não há previsões resolvidas o suficiente para medir calibração. As métricas aparecem conforme as
            partidas avançam e os horizontes se resolvem.
          </EmptyNote>
        )}
      </section>

      {/* Per-match reasoning */}
      {selectedMatch && (
        <section>
          <SectionHeading
            title="Por que esta previsão"
            description="Indicadores brutos que mais se destacam agora. Leitura direta dos números — não gerado por IA."
          />

          {matches.length > 1 && (
            <div className="flex flex-wrap gap-1 mt-4 mb-1">
              {matches.map((m) => (
                <button
                  key={m.state.matchId}
                  onClick={() => setSelectedMatchId(m.state.matchId)}
                  className={cn(
                    'px-2.5 py-1.5 rounded text-[12px] transition-colors',
                    m.state.matchId === selectedMatch.state.matchId
                      ? 'text-slate-100 bg-white/[0.06]'
                      : 'text-slate-500 hover:text-slate-300'
                  )}
                >
                  {homeLabel(m.state)} × {awayLabel(m.state)}
                </button>
              ))}
            </div>
          )}

          <ul className="mt-3 border-t border-white/[0.06]">
            {factors.map((f) => (
              <li key={f.label} className="flex items-baseline gap-3 py-2.5 border-b border-white/[0.06]">
                <span
                  className={cn(
                    'font-mono text-[11px] w-4 shrink-0',
                    f.direction === 'up' ? 'text-emerald-400' : f.direction === 'down' ? 'text-rose-400' : 'text-slate-600'
                  )}
                >
                  {f.direction === 'up' ? '↑' : f.direction === 'down' ? '↓' : '·'}
                </span>
                <span className="text-[13px] text-slate-200 w-52 shrink-0">{f.label}</span>
                <span className="text-[12.5px] text-slate-500">{f.detail}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
};

const Metric: React.FC<{ label: string; value: string; hint: string; accent?: boolean }> = ({
  label,
  value,
  hint,
  accent,
}) => (
  <div className="bg-[#0B1221] px-4 py-3.5">
    <div className="font-mono text-[10px] uppercase tracking-wider text-slate-600 mb-1.5">{label}</div>
    <div className={cn('font-mono text-[26px] leading-none tabular-nums', accent ? 'text-emerald-400' : 'text-slate-100')}>
      {value}
    </div>
    <div className="text-[11px] text-slate-600 mt-1.5">{hint}</div>
  </div>
);

const SectionHeading: React.FC<{ title: string; description?: string }> = ({ title, description }) => (
  <div>
    <h2 className="text-[15px] font-medium text-slate-100">{title}</h2>
    {description && <p className="text-[12.5px] text-slate-500 mt-1">{description}</p>}
  </div>
);

const LegendDot: React.FC<{ color: string; label: string }> = ({ color, label }) => (
  <span className="flex items-center gap-1.5">
    <span className="w-2.5 h-[2px] rounded-full" style={{ backgroundColor: color }} />
    {label}
  </span>
);

const EmptyNote: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <p className="mt-4 py-6 px-4 border-l-2 border-white/[0.08] bg-white/[0.02] text-[13px] leading-relaxed text-slate-500">
    {children}
  </p>
);
