# Golo

Golo estimates, in near-real-time, the probability of one or two additional goals in a live football match. It includes an admin-only strategy laboratory and a private, invitation-only Telegram beta. There is no automated wagering, stake recommendation, or LLM in the prediction path.

The full product/technical specification lives in [`context/Golo_Blueprint_MVP.md`](context/Golo_Blueprint_MVP.md) (Portuguese); the UI/UX spec is in [`context/Golo_UI_UX_Blueprint.md`](context/Golo_UI_UX_Blueprint.md). Treat those as the source of truth for scope and architecture decisions.

## Quickstart

```bash
cp .env.example .env   # defaults to PROVIDER=mock, no external account needed
make build
make run
```

The API server starts on `http://localhost:8080`. Public research endpoints live under `/api/matches` and `/api/metrics`; protected beta controls live under `/api/admin`.

In a second terminal:

```bash
make frontend-dev
```

Opens the React/Vite dashboard on `http://localhost:5173`, polling the API above.

## Switching to a real live provider

Set `PROVIDER=sportmonks` and `SPORTMONKS_API_KEY` in `.env` (see `.env.example` for the trial signup link). The key is only ever read server-side (`internal/config`) — it's never returned by the API or fetched by the frontend.

SportMonks scopes which leagues are fetchable at the account/plan level (you pick leagues in their dashboard), not via an API parameter — a Free Plan key only returns whatever small fixed set of leagues that plan includes. Current league priority for this project: **Brasileirão Série A, Copa Libertadores**, then **MLS, Liga MX**. Once a plan with real access to those leagues exists, put their SportMonks `league_id`s in `PRIORITY_COMPETITION_IDS` (comma-separated, priority order) — `internal/providers/sportmonks` will surface matches from those leagues first without hiding others.

## Private beta and fail-closed rollout

Copy the beta settings from `.env.example`, configure a bcrypt admin hash and a 32+ character session secret, then open **Strategy Lab**. Strategy edits are immutable versions and may only be armed after all chronological evidence gates pass.

The shipped `hazard_v1.2.0` artifact was evaluated against a constant-rate baseline on newest-season holdouts. It currently does **not** beat that baseline for either one or two remaining goals, so `model_qualified` rejects every alert. This is deliberate. Rebuild timestamped activity timelines and retrain before considering delivery:

```bash
ml/.venv/bin/python ml/src/build_dataset.py
ml/.venv/bin/python ml/src/train_baseline.py
```

Use Odds-API.io only in shadow mode on a development/free plan. Before accepting payment, verify commercial usage and deep-link authorization, select adequate request capacity, then explicitly set `ALERT_ENGINE_ENABLED=true`. The “two more goals” market also requires its own model qualification and the separate dashboard switch.

Telegram enrollment is one-time-code based. A user sends `/start CODE`, confirms 18+ and the beta terms, and receives the same centrally armed strategies until their manually managed expiry. Every qualified entry is retained with its offered price, model/market probability, evidence, and gate decisions; every win, loss, or void is forwarded.

All messages include: `18+ · Ministério da Fazenda adverte: Aposta não é investimento.`

## Development

```bash
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -l . (formatting check)
npm --prefix apps/web run build
```
