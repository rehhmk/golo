# Walkthrough: Golo - Real-Time Football Probability Engine (MVP)

A aplicação **Golo MVP** foi completamente construída, testada e construída para produção, conforme os blueprints de produto (`context/Golo_Blueprint_MVP.md`) e UI/UX (`context/Golo_UI_UX_Blueprint.md`).

---

## 1. O que foi construído

### Backend Modular em Go (`cmd/golo`, `internal/`)
- **[types.go](file:///Users/enzotriches/Documents/Golo/internal/domain/types.go)** e **[events.go](file:///Users/enzotriches/Documents/Golo/internal/domain/events.go)**: Modelo de domínio canônico e imutável de eventos e estado probabilístico da partida.
- **[reducer.go](file:///Users/enzotriches/Documents/Golo/internal/reducer/reducer.go)**: Reducer determinístico com janelas móveis rolling stats (60s, 3m, 5m, 10m) para chutes, $xG$, escanteios, cartões e ataques perigosos.
- **[engine.go](file:///Users/enzotriches/Documents/Golo/internal/features/engine.go)**: Extrator de snapshots de vetores de features sem risco de *leakage* de informação futura.
- **[calibrator.go](file:///Users/enzotriches/Documents/Golo/internal/calibration/calibrator.go)** e **[predictor.go](file:///Users/enzotriches/Documents/Golo/internal/predictor/predictor.go)**: Motor de inferência multi-horizonte (5m, 10m, Fim do Jogo/FT) com calibração por Platt Scaling (\(P_{cal} = \frac{1}{1 + e^{A f(x) + B}}\)).
- **[baseline_v1.json](file:///Users/enzotriches/Documents/Golo/models/baseline_v1.json)**: Artefato do modelo baseline calibrado.
- **[evaluator.go](file:///Users/enzotriches/Documents/Golo/internal/quality/evaluator.go)**: Calculadora de qualidade operacional de dados e score de confiança.
- **[sqlite.go](file:///Users/enzotriches/Documents/Golo/internal/eventstore/sqlite.go)**: Armazenamento SQLite em modo WAL (`journal_mode=WAL`) para histórico auditável de eventos, estados e previsões.
- **[replay.go](file:///Users/enzotriches/Documents/Golo/internal/providers/replay/replay.go)** e **[mock.go](file:///Users/enzotriches/Documents/Golo/internal/providers/mock/mock.go)**: Provedor de Replay para simulação a velocidade 1x, 10x e MAX, e Mock Provider para simulação ao vivo.
- **[server.go](file:///Users/enzotriches/Documents/Golo/internal/api/server.go)**: Servidor HTTP com endpoints REST e Server-Sent Events (SSE) `/api/matches`, `/api/matches/:id/stream`, `/api/replay/control` e `/api/metrics`.
- **[main.go](file:///Users/enzotriches/Documents/Golo/cmd/golo/main.go)**: Executável principal compilado em `bin/golo`.

### Scripts de ML & Avaliação (`ml/src/`)
- **[train_baseline.py](file:///Users/enzotriches/Documents/Golo/ml/src/train_baseline.py)**: Script Python para exportação do modelo baseline e parâmetros de calibração.
- **[evaluate_calibration.py](file:///Users/enzotriches/Documents/Golo/ml/src/evaluate_calibration.py)**: Script de avaliação de Brier Score, Log Loss e ECE (Expected Calibration Error).

### Frontend Web React (`apps/web/`)
- **Design System Data-Dense Dark Mode**: Canvas `slate-950` (`#020617`), Cards `slate-900` (`#0F172A`), Hot Prob `emerald-400` com efeito pulse glow, Live Badge `rose-500` e tipografia `JetBrains Mono` para métricas numéricas.
- **[LiveBoard.tsx](file:///Users/enzotriches/Documents/Golo/apps/web/src/components/LiveBoard.tsx)**: Lista de partidas ordenadas por probabilidade ou momentum com suporte a filtros e busca.
- **[GameCard.tsx](file:///Users/enzotriches/Documents/Golo/apps/web/src/components/GameCard.tsx)**: Card interativo da partida com placar, relógio ao vivo, badges e métricas dos últimos 10 minutos.
- **[MatchDetail.tsx](file:///Users/enzotriches/Documents/Golo/apps/web/src/components/MatchDetail.tsx)**: Tela detalhada da partida com gráfico de evolução de probabilidade em tempo real (Recharts AreaChart), gauges por horizonte, estatísticas comparativas e auditoria de qualidade.
- **[ReplayControl.tsx](file:///Users/enzotriches/Documents/Golo/apps/web/src/components/ReplayControl.tsx)**: Painel de controle interativo do motor de replay (Play/Pause, velocidade 1x, 10x, MAX, Reset).
- **[EvaluationDashboard.tsx](file:///Users/enzotriches/Documents/Golo/apps/web/src/components/EvaluationDashboard.tsx)**: Dashboard estatístico exibindo Brier Score, Log Loss, ECE e curva de calibração por faixa de probabilidade.

---

## 2. Resultados dos Testes Automatizados

Todos os pacotes Go e a integração de replay foram validados:

```bash
go test -v ./...
```

**Resultado dos testes:**
- `internal/reducer`: `PASS` (Validação de gols e cartões em janelas móveis)
- `internal/features`: `PASS` (Extração determinística de snapshots de features)
- `internal/calibration`: `PASS` (Calibração Platt Scaling)
- `internal/predictor`: `PASS` (Monotonicidade e probabilidades por horizonte)
- `internal/quality`: `PASS` (Degradação por lag do feed)
- `internal/eventstore`: `PASS` (Operações SQLite WAL e consultas)
- `tests/replay`: `PASS` (Simulação end-to-end de partida via replay engine)

**Compilação do Frontend React/Vite:**
- Executado `npm run build` com sucesso gerando os artefatos de produção em `apps/web/dist/`.

---

## 3. Como Executar a Aplicação Localmente

### Rodar o Backend em Go:
```bash
./bin/golo
```
O servidor HTTP estará rodando em `http://localhost:8080`.

### Rodar o Frontend em modo Desenvolvimento:
```bash
cd apps/web && npm run dev
```
Acesse a aplicação no navegador em `http://localhost:5173`.
