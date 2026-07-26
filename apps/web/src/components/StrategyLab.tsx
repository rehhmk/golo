import React, { useCallback, useEffect, useState } from 'react';
import { API_BASE_URL } from '../config';

type Condition = { field: string; operator: string; value: number };
type Definition = {
  id: string; name: string; version: number; conditions: Condition[];
  competitionIds: string[]; additionalGoals: number; minimumOdds: number;
  minimumSamples: number; minimumHoldout: number; minimumModelEdge: number; enabled: boolean;
};
type Result = { matchCount: number; hitRate: number; rateLow: number; rateHigh: number; oddsHigh: number };
type StoredStrategy = {
  definition: Definition; report: { allMatches: Result; holdout: Result; qualified: boolean; failures: string[] };
  armed: boolean; createdAt: string;
};
type Performance = {
  resolved: number; wins: number; losses: number; voids: number; profitUnits: number;
  roiPct: number; longestLossStreak: number; maxDrawdownUnits: number;
};
type Settings = {
  alertEngineEnabled: boolean; telegramEnabled: boolean; minimumDataQuality: number;
  maximumFeedLag: number; maximumQuoteAge: number; postGoalCooldown: number;
  bookmaker: string; allowTwoGoalAlerts: boolean;
};
type Signal = {
  id: string; homeTeam: string; awayTeam: string; matchSecond: number; status: string;
  marketLine: number; modelProbability: number; marketProbability: number; edge: number;
  failures: string[]; quote: { over: number; bookmaker: string; updatedAt: string };
};
type Invitation = { code: string; expiresAt: string; accessUntil: string; usedAt?: string };
type Subscriber = { id: string; displayName: string; active: boolean; expiresAt: string; adultConfirmed: boolean };
type ProviderHealth = {
  odds: { provider: string; healthy: boolean; lastSuccessAt?: string; errorCount?: number; message?: string; cachedEvents?: number };
  modelAndDelivery: { oneGoalModelQualified?: boolean; twoGoalModelQualified?: boolean; alertDeploymentEnabled?: boolean; telegramDeploymentEnabled?: boolean };
};

const initialDefinition: Definition = {
  id: '', name: 'Gol provável após 70 minutos', version: 1,
  conditions: [{ field: 'minute', operator: 'gte', value: 70 }],
  competitionIds: [], additionalGoals: 1, minimumOdds: 1.5,
  minimumSamples: 500, minimumHoldout: 150, minimumModelEdge: 0.08, enabled: true,
};

const fieldLabels: Record<string, string> = {
  minute: 'Minuto', remaining_minutes: 'Minutos restantes', goals_so_far: 'Gols no placar',
  abs_score_diff: 'Diferença no placar', red_cards_total: 'Cartões vermelhos',
};
const operatorLabels: Record<string, string> = { eq: '=', gte: '≥', lte: '≤' };
const pct = (value = 0) => `${(value * 100).toFixed(1)}%`;

export const StrategyLab: React.FC = () => {
  const [token, setToken] = useState(() => sessionStorage.getItem('golo-admin-token') || '');
  const [password, setPassword] = useState('');
  const [definition, setDefinition] = useState<Definition>(initialDefinition);
  const [preview, setPreview] = useState<StoredStrategy | null>(null);
  const [strategies, setStrategies] = useState<StoredStrategy[]>([]);
  const [signals, setSignals] = useState<Signal[]>([]);
  const [performance, setPerformance] = useState<Performance | null>(null);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [invitations, setInvitations] = useState<Invitation[]>([]);
  const [subscribers, setSubscribers] = useState<Subscriber[]>([]);
  const [providerHealth, setProviderHealth] = useState<ProviderHealth | null>(null);
  const [testChatId, setTestChatId] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  const request = useCallback(async (path: string, init: RequestInit = {}) => {
    const response = await fetch(`${API_BASE_URL}/api/admin/${path}`, {
      ...init,
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}`, ...(init.headers || {}) },
    });
    if (response.status === 401) {
      sessionStorage.removeItem('golo-admin-token');
      setToken('');
      throw new Error('Sessão expirada');
    }
    const body = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(body?.error || body?.message || `Erro ${response.status}`);
    return body;
  }, [token]);

  const refresh = useCallback(async () => {
    if (!token) return;
    try {
      const [strategyRows, signalRows, perf, currentSettings, inviteRows, subscriberRows, health] = await Promise.all([
        request('strategies'), request('signals'), request('performance'), request('settings'),
        request('invitations'), request('subscribers'), request('provider-health'),
      ]);
      setStrategies(strategyRows); setSignals(signalRows); setPerformance(perf);
      setSettings(currentSettings); setInvitations(inviteRows); setSubscribers(subscriberRows);
      setProviderHealth(health);
    } catch (error) { setMessage(String(error)); }
  }, [request, token]);

  useEffect(() => { refresh(); }, [refresh]);

  const login = async (event: React.FormEvent) => {
    event.preventDefault(); setBusy(true); setMessage('');
    try {
      const response = await fetch(`${API_BASE_URL}/api/admin/login`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password }),
      });
      if (!response.ok) throw new Error('Senha inválida ou autenticação não configurada');
      const body = await response.json();
      sessionStorage.setItem('golo-admin-token', body.token);
      setToken(body.token); setPassword('');
    } catch (error) { setMessage(String(error)); } finally { setBusy(false); }
  };

  const backtest = async (persist: boolean) => {
    setBusy(true); setMessage('');
    try {
      const result = await request(persist ? 'strategies' : 'strategies/backtest', {
        method: 'POST', body: JSON.stringify(definition),
      });
      setPreview(result);
      setMessage(persist ? 'Nova versão salva e aguardando armação.' : 'Backtest concluído.');
      if (persist) refresh();
    } catch (error) { setMessage(String(error)); } finally { setBusy(false); }
  };

  const arm = async (row: StoredStrategy) => {
    setBusy(true);
    try {
      await request(`strategies/${row.definition.id}/arm`, {
        method: 'POST', body: JSON.stringify({ version: row.definition.version, armed: !row.armed }),
      });
      await refresh();
    } catch (error) { setMessage(String(error)); } finally { setBusy(false); }
  };

  const saveSettings = async () => {
    if (!settings) return;
    setBusy(true);
    try {
      await request('settings', { method: 'PUT', body: JSON.stringify(settings) });
      setMessage('Configurações salvas. A variável de ambiente continua sendo o bloqueio máximo de segurança.');
    } catch (error) { setMessage(String(error)); } finally { setBusy(false); }
  };

  const createInvitation = async () => {
    setBusy(true);
    try {
      await request('invitations', { method: 'POST', body: JSON.stringify({ inviteHours: 24, accessDays: 30 }) });
      await refresh();
    } catch (error) { setMessage(String(error)); } finally { setBusy(false); }
  };

  const updateSubscriber = async (subscriber: Subscriber, active: boolean, extendDays = 0) => {
    setBusy(true);
    try {
      const base = Math.max(Date.now(), new Date(subscriber.expiresAt).getTime());
      const expiresAt = new Date(base + extendDays * 86400000).toISOString();
      await request(`subscribers/${subscriber.id}`, {
        method: 'PUT', body: JSON.stringify({ active, expiresAt }),
      });
      await refresh();
    } catch (error) { setMessage(String(error)); } finally { setBusy(false); }
  };

  const sendTelegramTest = async () => {
    setBusy(true);
    try {
      await request('telegram/test', { method: 'POST', body: JSON.stringify({ chatId: testChatId }) });
      setMessage('Mensagem de teste enviada.');
    } catch (error) { setMessage(String(error)); } finally { setBusy(false); }
  };

  if (!token) return (
    <div className="max-w-md mx-auto mt-20 border border-white/10 bg-slate-950 rounded-xl p-6">
      <p className="font-mono text-xs uppercase tracking-wider text-emerald-400">Área privada</p>
      <h1 className="text-2xl font-semibold mt-2">Strategy Lab</h1>
      <p className="text-sm text-slate-400 mt-2">Entre com a senha única do administrador.</p>
      <form onSubmit={login} className="mt-6 space-y-3">
        <input type="password" value={password} onChange={e => setPassword(e.target.value)}
          className="w-full bg-slate-900 border border-white/10 rounded px-3 py-2" placeholder="Senha" />
        <button disabled={busy} className="w-full bg-emerald-500 text-slate-950 font-semibold rounded py-2">Entrar</button>
      </form>
      {message && <p className="text-sm text-rose-400 mt-3">{message}</p>}
    </div>
  );

  return (
    <div className="space-y-6">
      <div>
        <p className="font-mono text-xs uppercase tracking-wider text-emerald-400">Administração privada</p>
        <h1 className="text-3xl font-semibold mt-1">Strategy Lab</h1>
        <p className="text-sm text-slate-400 mt-2">Regras AND auditáveis, holdout cronológico e operação do beta no Telegram.</p>
      </div>
      {message && <div className="border border-white/10 bg-slate-900 rounded-lg px-4 py-3 text-sm">{message}</div>}

      <section className="grid lg:grid-cols-[1.4fr_1fr] gap-5">
        <div className="border border-white/10 rounded-xl p-5 bg-slate-950">
          <h2 className="font-semibold">Construtor de estratégia</h2>
          <div className="grid sm:grid-cols-2 gap-3 mt-4">
            <label className="text-xs text-slate-400">Nome<input value={definition.name}
              onChange={e => setDefinition({ ...definition, name: e.target.value })}
              className="mt-1 w-full bg-slate-900 border border-white/10 rounded px-3 py-2 text-slate-100" /></label>
            <label className="text-xs text-slate-400">Mercado<select value={definition.additionalGoals}
              onChange={e => setDefinition({ ...definition, additionalGoals: Number(e.target.value) })}
              className="mt-1 w-full bg-slate-900 border border-white/10 rounded px-3 py-2 text-slate-100">
              <option value={1}>Mais 1 gol</option><option value={2}>Mais 2 gols (laboratório)</option>
            </select></label>
          </div>
          <div className="space-y-2 mt-4">
            {definition.conditions.map((condition, index) => (
              <div key={index} className="grid grid-cols-[1fr_80px_90px_32px] gap-2">
                <select value={condition.field} onChange={e => {
                  const conditions = [...definition.conditions]; conditions[index] = { ...condition, field: e.target.value };
                  setDefinition({ ...definition, conditions });
                }} className="bg-slate-900 border border-white/10 rounded px-2 py-2 text-sm">
                  {Object.entries(fieldLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select>
                <select value={condition.operator} onChange={e => {
                  const conditions = [...definition.conditions]; conditions[index] = { ...condition, operator: e.target.value };
                  setDefinition({ ...definition, conditions });
                }} className="bg-slate-900 border border-white/10 rounded px-2 py-2">
                  {Object.entries(operatorLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select>
                <input type="number" min={0} value={condition.value} onChange={e => {
                  const conditions = [...definition.conditions]; conditions[index] = { ...condition, value: Number(e.target.value) };
                  setDefinition({ ...definition, conditions });
                }} className="bg-slate-900 border border-white/10 rounded px-2 py-2" />
                <button onClick={() => setDefinition({ ...definition, conditions: definition.conditions.filter((_, i) => i !== index) })}
                  className="text-slate-500 hover:text-rose-400">×</button>
              </div>
            ))}
            <button onClick={() => setDefinition({ ...definition, conditions: [...definition.conditions, { field: 'goals_so_far', operator: 'lte', value: 3 }] })}
              className="text-sm text-emerald-400">+ condição AND</button>
          </div>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mt-4">
            <label className="text-xs text-slate-400">Odd mínima<input type="number" step=".01" value={definition.minimumOdds}
              onChange={e => setDefinition({ ...definition, minimumOdds: Number(e.target.value) })} className="mt-1 w-full bg-slate-900 border border-white/10 rounded px-2 py-2" /></label>
            <label className="text-xs text-slate-400">Amostra total<input type="number" value={definition.minimumSamples}
              onChange={e => setDefinition({ ...definition, minimumSamples: Number(e.target.value) })} className="mt-1 w-full bg-slate-900 border border-white/10 rounded px-2 py-2" /></label>
            <label className="text-xs text-slate-400">Holdout<input type="number" value={definition.minimumHoldout}
              onChange={e => setDefinition({ ...definition, minimumHoldout: Number(e.target.value) })} className="mt-1 w-full bg-slate-900 border border-white/10 rounded px-2 py-2" /></label>
            <label className="text-xs text-slate-400">Edge mínimo<input type="number" step=".01" value={definition.minimumModelEdge}
              onChange={e => setDefinition({ ...definition, minimumModelEdge: Number(e.target.value) })} className="mt-1 w-full bg-slate-900 border border-white/10 rounded px-2 py-2" /></label>
          </div>
          <div className="flex gap-3 mt-5">
            <button disabled={busy} onClick={() => backtest(false)} className="border border-white/15 rounded px-4 py-2 text-sm">Rodar backtest</button>
            <button disabled={busy} onClick={() => backtest(true)} className="bg-emerald-500 text-slate-950 font-semibold rounded px-4 py-2 text-sm">Salvar nova versão</button>
          </div>
        </div>

        <div className="border border-white/10 rounded-xl p-5 bg-slate-950">
          <h2 className="font-semibold">Resultado cronológico</h2>
          {!preview ? <p className="text-sm text-slate-500 mt-4">Rode um backtest para ver a qualificação.</p> : <>
            <div className={`mt-4 rounded-lg p-3 ${preview.report.qualified ? 'bg-emerald-500/10 text-emerald-300' : 'bg-amber-500/10 text-amber-300'}`}>
              {preview.report.qualified ? 'Qualificada pelos gates históricos' : 'Ainda não qualificada'}
            </div>
            <div className="grid grid-cols-2 gap-3 mt-4 text-sm">
              <div><span className="text-slate-500">Total</span><p>{preview.report.allMatches.matchCount} partidas · {pct(preview.report.allMatches.hitRate)}</p></div>
              <div><span className="text-slate-500">Holdout</span><p>{preview.report.holdout.matchCount} partidas · {pct(preview.report.holdout.hitRate)}</p></div>
              <div><span className="text-slate-500">Wilson 95%</span><p>{pct(preview.report.holdout.rateLow)}–{pct(preview.report.holdout.rateHigh)}</p></div>
              <div><span className="text-slate-500">Odd conservadora</span><p>{preview.report.holdout.oddsHigh?.toFixed(2) || '—'}</p></div>
            </div>
            {preview.report.failures?.map(failure => <p key={failure} className="text-xs text-rose-300 mt-2">• {failure}</p>)}
          </>}
        </div>
      </section>

      <section className="border border-white/10 rounded-xl p-5 bg-slate-950">
        <h2 className="font-semibold">Versões e armação</h2>
        <div className="overflow-x-auto mt-3"><table className="w-full text-sm">
          <thead className="text-left text-slate-500"><tr><th className="py-2">Estratégia</th><th>Mercado</th><th>Amostra</th><th>Holdout</th><th>Estado</th><th /></tr></thead>
          <tbody>{strategies.map(row => <tr key={`${row.definition.id}-${row.definition.version}`} className="border-t border-white/5">
            <td className="py-3">{row.definition.name} <span className="text-slate-600">v{row.definition.version}</span></td>
            <td>+{row.definition.additionalGoals} gol(s)</td><td>{row.report.allMatches.matchCount}</td><td>{row.report.holdout.matchCount}</td>
            <td>{row.armed ? 'Armada' : row.report.qualified ? 'Qualificada' : 'Bloqueada'}</td>
            <td className="text-right"><button disabled={busy || (!row.report.qualified && !row.armed)} onClick={() => arm(row)}
              className="border border-white/15 disabled:opacity-30 rounded px-3 py-1">{row.armed ? 'Desarmar' : 'Armar'}</button></td>
          </tr>)}</tbody>
        </table></div>
      </section>

      {settings && <section className="grid lg:grid-cols-2 gap-5">
        <div className="border border-white/10 rounded-xl p-5 bg-slate-950">
          <h2 className="font-semibold">Operação</h2>
          {providerHealth && <div className={`mt-3 rounded p-3 text-sm ${providerHealth.odds.healthy ? 'bg-emerald-500/10 text-emerald-300' : 'bg-amber-500/10 text-amber-300'}`}>
            Odds: {providerHealth.odds.healthy ? 'saudável' : 'indisponível'} · {providerHealth.odds.cachedEvents || 0} eventos
            {providerHealth.odds.message ? ` · ${providerHealth.odds.message}` : ''}
            <p className="mt-1">Modelo +1: {providerHealth.modelAndDelivery.oneGoalModelQualified ? 'qualificado' : 'bloqueado'} · +2: {providerHealth.modelAndDelivery.twoGoalModelQualified ? 'qualificado' : 'bloqueado'}</p>
            <p>Ceilings: motor {providerHealth.modelAndDelivery.alertDeploymentEnabled ? 'on' : 'off'} · Telegram {providerHealth.modelAndDelivery.telegramDeploymentEnabled ? 'on' : 'off'}</p>
          </div>}
          <div className="space-y-3 mt-4 text-sm">
            <label className="flex justify-between"><span>Motor de alertas</span><input type="checkbox" checked={settings.alertEngineEnabled} onChange={e => setSettings({ ...settings, alertEngineEnabled: e.target.checked })} /></label>
            <label className="flex justify-between"><span>Entrega Telegram</span><input type="checkbox" checked={settings.telegramEnabled} onChange={e => setSettings({ ...settings, telegramEnabled: e.target.checked })} /></label>
            <label className="flex justify-between"><span>Permitir mercado de +2 gols</span><input type="checkbox" checked={settings.allowTwoGoalAlerts} onChange={e => setSettings({ ...settings, allowTwoGoalAlerts: e.target.checked })} /></label>
            <label className="block text-slate-400">Bookmaker<input value={settings.bookmaker} onChange={e => setSettings({ ...settings, bookmaker: e.target.value })} className="mt-1 w-full bg-slate-900 border border-white/10 rounded px-3 py-2 text-slate-100" /></label>
            <button disabled={busy} onClick={saveSettings} className="bg-emerald-500 text-slate-950 font-semibold rounded px-4 py-2">Salvar operação</button>
            <div className="flex gap-2 pt-2">
              <input value={testChatId} onChange={e => setTestChatId(e.target.value)} placeholder="Telegram chat ID"
                className="min-w-0 flex-1 bg-slate-900 border border-white/10 rounded px-3 py-2" />
              <button disabled={busy || !testChatId} onClick={sendTelegramTest} className="border border-white/15 rounded px-3">Testar</button>
            </div>
          </div>
        </div>
        <div className="border border-white/10 rounded-xl p-5 bg-slate-950">
          <h2 className="font-semibold">Desempenho · 1 unidade fixa</h2>
          {performance && <div className="grid grid-cols-2 sm:grid-cols-3 gap-4 mt-4 text-sm">
            <div><span className="text-slate-500">Resolvidos</span><p className="text-xl">{performance.resolved}</p></div>
            <div><span className="text-slate-500">Acertos / erros</span><p className="text-xl">{performance.wins} / {performance.losses}</p></div>
            <div><span className="text-slate-500">ROI teórico</span><p className="text-xl">{performance.roiPct.toFixed(1)}%</p></div>
            <div><span className="text-slate-500">Resultado</span><p>{performance.profitUnits.toFixed(2)} un.</p></div>
            <div><span className="text-slate-500">Maior sequência ruim</span><p>{performance.longestLossStreak}</p></div>
            <div><span className="text-slate-500">Drawdown máximo</span><p>{performance.maxDrawdownUnits.toFixed(2)} un.</p></div>
          </div>}
        </div>
      </section>}

      <section className="grid lg:grid-cols-2 gap-5">
        <div className="border border-white/10 rounded-xl p-5 bg-slate-950">
          <div className="flex justify-between"><h2 className="font-semibold">Convites Telegram</h2><button onClick={createInvitation} className="text-sm text-emerald-400">+ convite 30 dias</button></div>
          <div className="mt-3 space-y-2 text-sm">{invitations.slice(0, 8).map(invite =>
            <div key={invite.code} className="bg-slate-900 rounded p-3"><code>{invite.code}</code><p className="text-xs text-slate-500 mt-1">{invite.usedAt ? 'Utilizado' : `Expira ${new Date(invite.expiresAt).toLocaleString('pt-BR')}`}</p></div>)}</div>
        </div>
        <div className="border border-white/10 rounded-xl p-5 bg-slate-950">
          <h2 className="font-semibold">Assinantes</h2>
          <div className="mt-3 space-y-2 text-sm">{subscribers.map(subscriber =>
            <div key={subscriber.id} className="flex flex-wrap gap-2 justify-between border-b border-white/5 py-2">
              <span>{subscriber.displayName || subscriber.id}</span>
              <span className={subscriber.active ? 'text-emerald-400' : 'text-slate-500'}>{subscriber.active ? 'Ativo' : 'Inativo'} · até {new Date(subscriber.expiresAt).toLocaleDateString('pt-BR')}</span>
              <span className="flex gap-2">
                <button disabled={busy} onClick={() => updateSubscriber(subscriber, subscriber.active, 30)} className="text-emerald-400">+30 dias</button>
                <button disabled={busy} onClick={() => updateSubscriber(subscriber, !subscriber.active)} className="text-slate-400">{subscriber.active ? 'Pausar' : 'Ativar'}</button>
              </span>
            </div>)}</div>
        </div>
      </section>

      <section className="border border-white/10 rounded-xl p-5 bg-slate-950">
        <h2 className="font-semibold">Histórico completo de decisões</h2>
        <div className="overflow-x-auto mt-3"><table className="w-full text-sm">
          <thead className="text-left text-slate-500"><tr><th className="py-2">Jogo</th><th>Entrada</th><th>Odd</th><th>Golo / mercado</th><th>Edge</th><th>Estado ou falhas</th></tr></thead>
          <tbody>{signals.slice(0, 100).map(signal => <tr key={signal.id} className="border-t border-white/5">
            <td className="py-3">{signal.homeTeam} × {signal.awayTeam}</td><td>{Math.floor(signal.matchSecond / 60)}' · Over {signal.marketLine}</td>
            <td>{signal.quote?.over?.toFixed(2) || '—'}</td><td>{pct(signal.modelProbability)} / {pct(signal.marketProbability)}</td>
            <td>{pct(signal.edge)}</td><td>{signal.status}{signal.failures?.length ? ` · ${signal.failures.join(', ')}` : ''}</td>
          </tr>)}</tbody>
        </table></div>
      </section>
    </div>
  );
};
