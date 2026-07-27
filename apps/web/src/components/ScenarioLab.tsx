import React, { useState, useEffect } from 'react';
import { API_BASE_URL } from '../config';
import { PageHeader } from './PageHeader';
import { cn } from '@/lib/utils';

interface ScenarioReport {
  definition: { targetOdds: number };
  description: string;
  matchesConsidered: number;
  occurrences: number;
  wins: number;
  hitRate: number;
  rateLow: number;
  rateHigh: number;
  breakEvenOdds: number;
  requiredOdds: number;
  longestLossStreak: number;
  stakeFraction: number;
  baselineHitRate: number;
  beatsBaseline: boolean;
  worthwhile: boolean;
  sampleSufficient: boolean;
  neededSample: number;
  verdict: string;
}

interface Competition {
  id: string;
  name: string;
  matches: number;
}

interface DatasetInfo {
  matches: number;
  goals: number;
  competitions: Competition[];
}

// The builder deals in optional conditions, so every field is a string that
// may be empty — an empty field means "no condition", not "zero".
interface Draft {
  fromMinute: string;
  scoreMode: 'any' | 'level' | 'diffAtLeast';
  scoreDiffAtLeast: string;
  goalsExactly: string;
  redCards: boolean;
  horizonMinutes: string;
  targetOdds: string;
  /** Empty means every competition — the backend reads it the same way. */
  competitionIds: string[];
}

const EMPTY_DRAFT: Draft = {
  fromMinute: '70',
  scoreMode: 'diffAtLeast',
  scoreDiffAtLeast: '2',
  goalsExactly: '',
  redCards: false,
  horizonMinutes: '0',
  targetOdds: '1,50',
  competitionIds: [],
};

function numberOrUndefined(value: string): number | undefined {
  const trimmed = value.trim();
  if (trimmed === '') return undefined;
  const parsed = Number(trimmed.replace(',', '.'));
  return Number.isFinite(parsed) ? parsed : undefined;
}

function buildPayload(draft: Draft) {
  return {
    name: 'cenário',
    fromMinute: numberOrUndefined(draft.fromMinute),
    scoreDiffExactly: draft.scoreMode === 'level' ? 0 : undefined,
    scoreDiffAtLeast:
      draft.scoreMode === 'diffAtLeast' ? numberOrUndefined(draft.scoreDiffAtLeast) : undefined,
    goalsExactly: numberOrUndefined(draft.goalsExactly),
    redCardsAtLeast: draft.redCards ? 1 : undefined,
    horizonMinutes: numberOrUndefined(draft.horizonMinutes) ?? 0,
    targetOdds: numberOrUndefined(draft.targetOdds) ?? 0,
    competitionIds: draft.competitionIds.length > 0 ? draft.competitionIds : undefined,
  };
}

export const ScenarioLab: React.FC = () => {
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [report, setReport] = useState<ScenarioReport | null>(null);
  const [dataset, setDataset] = useState<DatasetInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [running, setRunning] = useState(false);

  useEffect(() => {
    fetch(`${API_BASE_URL}/api/scenarios/dataset`)
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => data && setDataset(data))
      .catch(() => setDataset(null));
  }, []);

  const run = () => {
    setRunning(true);
    setError(null);
    fetch(`${API_BASE_URL}/api/scenarios/backtest`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(buildPayload(draft)),
    })
      .then(async (res) => {
        const body = await res.json();
        if (!res.ok) throw new Error(body.error ?? 'não foi possível medir o cenário');
        return body as ScenarioReport;
      })
      .then((data) => {
        setReport(data);
        setError(null);
      })
      .catch((err: Error) => {
        // No stale result left on screen next to a new error — that reads as
        // though the failed run produced those numbers.
        setReport(null);
        setError(err.message);
      })
      .finally(() => setRunning(false));
  };

  const pct = (v: number) => `${(v * 100).toFixed(1)}%`;

  return (
    <div>
      <PageHeader
        label="Laboratório"
        title="Testar um cenário"
        description="Você tem uma teoria sobre futebol. Descubra se ela é verdade — sem apostar para saber."
        meta={
          dataset && (
            <div className="font-mono text-[11px] tabular-nums text-slate-500">
              <span className="text-slate-200">{dataset.matches.toLocaleString('pt-BR')}</span> partidas ·{' '}
              <span className="text-slate-200">{dataset.goals.toLocaleString('pt-BR')}</span> gols
            </div>
          )
        }
      />

      <div className="grid grid-cols-1 lg:grid-cols-[280px_minmax(0,1fr)] gap-5">
        <div className="rounded-md border border-white/[0.06] bg-white/[0.02] p-4 h-fit">
          <p className="text-[12px] uppercase tracking-wider text-slate-500 mb-3">Quando</p>

          <label className="block mb-3">
            <span className="text-[12px] text-slate-400">A partir do minuto</span>
            <input
              type="number"
              min={0}
              max={90}
              value={draft.fromMinute}
              onChange={(e) => setDraft({ ...draft, fromMinute: e.target.value })}
              className="mt-1 w-full rounded bg-slate-900/60 border border-white/[0.08] px-2.5 py-1.5 text-[13px] font-mono tabular-nums text-slate-100"
            />
          </label>

          <label className="block mb-3">
            <span className="text-[12px] text-slate-400">Placar</span>
            <select
              value={draft.scoreMode}
              onChange={(e) => setDraft({ ...draft, scoreMode: e.target.value as Draft['scoreMode'] })}
              className="mt-1 w-full rounded bg-slate-900/60 border border-white/[0.08] px-2.5 py-1.5 text-[13px] text-slate-100"
            >
              <option value="any">qualquer</option>
              <option value="level">empatado</option>
              <option value="diffAtLeast">alguém vencendo por</option>
            </select>
          </label>

          {draft.scoreMode === 'diffAtLeast' && (
            <label className="block mb-3">
              <span className="text-[12px] text-slate-400">Gols de vantagem (ou mais)</span>
              <input
                type="number"
                min={1}
                value={draft.scoreDiffAtLeast}
                onChange={(e) => setDraft({ ...draft, scoreDiffAtLeast: e.target.value })}
                className="mt-1 w-full rounded bg-slate-900/60 border border-white/[0.08] px-2.5 py-1.5 text-[13px] font-mono tabular-nums text-slate-100"
              />
            </label>
          )}

          <label className="block mb-3">
            <span className="text-[12px] text-slate-400">Gols na partida (deixe vazio para qualquer)</span>
            <input
              type="number"
              min={0}
              value={draft.goalsExactly}
              onChange={(e) => setDraft({ ...draft, goalsExactly: e.target.value })}
              className="mt-1 w-full rounded bg-slate-900/60 border border-white/[0.08] px-2.5 py-1.5 text-[13px] font-mono tabular-nums text-slate-100"
            />
          </label>

          <label className="flex items-center gap-2 mb-4 cursor-pointer">
            <input
              type="checkbox"
              checked={draft.redCards}
              onChange={(e) => setDraft({ ...draft, redCards: e.target.checked })}
            />
            <span className="text-[12px] text-slate-400">Com alguém expulso</span>
          </label>

          {dataset && dataset.competitions.length > 1 && (
            <div className="mb-4 pt-3 border-t border-white/[0.06]">
              <div className="flex items-baseline justify-between mb-2">
                <span className="text-[12px] text-slate-400">Competições</span>
                {draft.competitionIds.length > 0 && (
                  <button
                    onClick={() => setDraft({ ...draft, competitionIds: [] })}
                    className="text-[11px] text-slate-500 hover:text-slate-300"
                  >
                    todas
                  </button>
                )}
              </div>
              <div className="space-y-1 max-h-44 overflow-y-auto pr-1">
                {dataset.competitions.map((c) => {
                  const selected =
                    draft.competitionIds.length === 0 || draft.competitionIds.includes(c.id);
                  return (
                    <label key={c.id} className="flex items-center gap-2 cursor-pointer group">
                      <input
                        type="checkbox"
                        checked={selected}
                        onChange={() => {
                          // An empty list means "all", so the first tick has to
                          // materialise the full list before removing one —
                          // otherwise unticking one league would read as
                          // selecting only that league.
                          const current =
                            draft.competitionIds.length === 0
                              ? dataset.competitions.map((x) => x.id)
                              : draft.competitionIds;
                          const next = current.includes(c.id)
                            ? current.filter((id) => id !== c.id)
                            : [...current, c.id];
                          setDraft({
                            ...draft,
                            competitionIds: next.length === dataset.competitions.length ? [] : next,
                          });
                        }}
                      />
                      <span className="text-[12px] text-slate-400 group-hover:text-slate-300 flex-1 truncate">
                        {c.name}
                      </span>
                      <span className="text-[11px] font-mono tabular-nums text-slate-600">
                        {c.matches.toLocaleString('pt-BR')}
                      </span>
                    </label>
                  );
                })}
              </div>
            </div>
          )}

          <p className="text-[12px] uppercase tracking-wider text-slate-500 mb-2 pt-3 border-t border-white/[0.06]">
            Espera-se
          </p>
          <label className="block mb-3">
            <select
              value={draft.horizonMinutes}
              onChange={(e) => setDraft({ ...draft, horizonMinutes: e.target.value })}
              className="w-full rounded bg-slate-900/60 border border-white/[0.08] px-2.5 py-1.5 text-[13px] text-slate-100"
            >
              <option value="0">sai mais um gol até o fim</option>
              <option value="5">sai gol nos próximos 5 min</option>
              <option value="10">sai gol nos próximos 10 min</option>
              <option value="15">sai gol nos próximos 15 min</option>
            </select>
          </label>

          <label className="block mb-4">
            <span className="text-[12px] text-slate-400">Odd que você consegue</span>
            <input
              type="text"
              value={draft.targetOdds}
              onChange={(e) => setDraft({ ...draft, targetOdds: e.target.value })}
              className="mt-1 w-full rounded bg-slate-900/60 border border-white/[0.08] px-2.5 py-1.5 text-[13px] font-mono tabular-nums text-slate-100"
            />
          </label>

          <button
            onClick={run}
            disabled={running}
            className="w-full rounded bg-emerald-500/15 border border-emerald-500/30 px-3 py-2 text-[13px] font-medium text-emerald-300 hover:bg-emerald-500/20 disabled:opacity-50"
          >
            {running ? 'Medindo…' : 'Testar cenário'}
          </button>
        </div>

        <div>
          {error && (
            <div className="rounded-md border border-amber-500/25 bg-amber-500/[0.06] px-4 py-3">
              <p className="text-[13px] text-amber-200/90">{error}</p>
            </div>
          )}

          {!error && !report && (
            <div className="rounded-md border border-white/[0.06] px-5 py-12 text-center">
              <p className="text-[13px] text-slate-400">Monte um cenário e clique em testar</p>
              <p className="text-[12px] text-slate-600 mt-1.5 max-w-md mx-auto leading-relaxed">
                O cenário pré-preenchido é o mais conhecido: a partir dos 70 minutos, com um time
                vencendo por dois ou mais.
              </p>
            </div>
          )}

          {report && <ReportView report={report} pct={pct} />}
        </div>
      </div>
    </div>
  );
};

const ReportView: React.FC<{ report: ScenarioReport; pct: (v: number) => string }> = ({ report, pct }) => {
  // Break-even comes from the price the user said they can get, not from the
  // price the scenario turned out to need. Deriving it from requiredOdds put
  // the marker on top of the interval by construction, hiding the very gap the
  // chart exists to show.
  const breakEven = 1 / (report.definition.targetOdds || 1);
  const tone = !report.sampleSufficient
    ? 'neutral'
    : report.worthwhile && report.beatsBaseline
      ? 'good'
      : 'bad';

  return (
    <div className="space-y-3">
      <div
        className={cn(
          'rounded-md px-4 py-3.5 border',
          tone === 'good' && 'border-emerald-500/25 bg-emerald-500/[0.06]',
          tone === 'bad' && 'border-amber-500/25 bg-amber-500/[0.06]',
          tone === 'neutral' && 'border-white/[0.08] bg-white/[0.03]'
        )}
      >
        <p
          className={cn(
            'text-[13px] leading-relaxed',
            tone === 'good' && 'text-emerald-200/90',
            tone === 'bad' && 'text-amber-200/90',
            tone === 'neutral' && 'text-slate-300'
          )}
        >
          {report.verdict}
        </p>
      </div>

      <p className="text-[12px] text-slate-500 font-mono">
        {report.description}
        <span className="text-slate-600">
          {' '}· medido em {report.matchesConsidered.toLocaleString('pt-BR')} partidas
        </span>
      </p>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-px bg-white/[0.06] border border-white/[0.06] rounded-md overflow-hidden">
        <Cell label="Acerto" value={pct(report.hitRate)} />
        <Cell label="Odd mínima" value={report.requiredOdds.toFixed(2)} hint="pior caso do intervalo" />
        <Cell label="Partidas" value={report.occurrences.toLocaleString('pt-BR')} />
        <Cell
          label="Pior sequência"
          value={`${report.longestLossStreak}`}
          hint="perdas seguidas, observado"
        />
      </div>

      {/* The interval is the point of the whole product: a hit rate quoted
          without it invites betting at a price the evidence cannot support. */}
      <div className="rounded-md border border-white/[0.06] bg-white/[0.02] px-4 py-4">
        <p className="text-[12px] text-slate-400 mb-3">Onde a taxa real provavelmente está</p>
        <div className="relative h-8">
          <div className="absolute top-3 inset-x-0 h-1 rounded bg-slate-800" />
          <div
            className="absolute top-3 h-1 rounded bg-emerald-500/70"
            style={{ left: `${report.rateLow * 100}%`, width: `${(report.rateHigh - report.rateLow) * 100}%` }}
          />
          <div
            className="absolute top-1.5 w-0.5 h-4 bg-slate-200"
            style={{ left: `${report.hitRate * 100}%` }}
            title="taxa observada"
          />
          <div
            className="absolute top-0.5 w-0.5 h-6 bg-amber-400"
            style={{ left: `${breakEven * 100}%` }}
            title="ponto de equilíbrio da odd"
          />
        </div>
        <div className="flex justify-between text-[11px] font-mono tabular-nums text-slate-600 mt-1">
          <span>0%</span>
          <span className="text-emerald-400/80">
            {pct(report.rateLow)} – {pct(report.rateHigh)}
          </span>
          <span className="text-amber-400/80">{pct(breakEven)} = equilíbrio</span>
          <span>100%</span>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div className="rounded-md border border-white/[0.06] px-4 py-3">
          <p className="text-[12px] text-slate-500 mb-1">Contra a taxa base</p>
          <p className={cn('text-[14px]', report.beatsBaseline ? 'text-emerald-300' : 'text-amber-300')}>
            {pct(report.hitRate)} contra {pct(report.baselineHitRate)}
          </p>
          <p className="text-[11px] text-slate-600 mt-1.5 leading-relaxed">
            {report.beatsBaseline
              ? 'O cenário acerta mais que uma partida qualquer.'
              : 'Não supera uma partida qualquer — o gatilho não está informando nada.'}
          </p>
        </div>
        <div className="rounded-md border border-white/[0.06] px-4 py-3">
          <p className="text-[12px] text-slate-500 mb-1">Banca por entrada</p>
          <p className="text-[14px] text-slate-200 font-mono tabular-nums">
            {report.worthwhile ? `${(report.stakeFraction * 100).toFixed(2)}%` : '—'}
          </p>
          <p className="text-[11px] text-slate-600 mt-1.5 leading-relaxed">
            {report.worthwhile
              ? '20% de drawdown aceitável dividido pela pior sequência observada.'
              : 'Não sugerimos tamanho de entrada para um cenário que não compensa.'}
          </p>
        </div>
      </div>
    </div>
  );
};

const Cell: React.FC<{ label: string; value: string; hint?: string }> = ({ label, value, hint }) => (
  <div className="bg-[#0B1221] px-3.5 py-3">
    <p className="text-[11px] uppercase tracking-wider text-slate-500">{label}</p>
    <p className="text-[20px] font-mono tabular-nums text-slate-100 mt-0.5">{value}</p>
    {hint && <p className="text-[11px] text-slate-600 mt-0.5">{hint}</p>}
  </div>
);
