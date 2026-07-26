# Golo

Golo estimates, in near-real-time, the probability of at least one goal occurring in the rest of a live football match, at several horizons (5min, 10min, full-time, home/away). It's a private research tool, not a betting product — no automated wagering, no LLM in the prediction path.

The full product/technical specification lives in [`context/Golo_Blueprint_MVP.md`](context/Golo_Blueprint_MVP.md) (Portuguese); the UI/UX spec is in [`context/Golo_UI_UX_Blueprint.md`](context/Golo_UI_UX_Blueprint.md). Treat those as the source of truth for scope and architecture decisions.

## Quickstart

```bash
cp .env.example .env   # defaults to PROVIDER=mock, no external account needed
make build
make run
```

The API server starts on `http://localhost:8080` (`/healthz`, `/api/matches`, `/api/matches/:id`, `/api/matches/:id/stream` SSE, `/api/metrics`, `/api/replay/control`).

In a second terminal:

```bash
make frontend-dev
```

Opens the React/Vite dashboard on `http://localhost:5173`, polling the API above.

## Switching to a real live provider

Set `PROVIDER=sportmonks` and `SPORTMONKS_API_KEY` in `.env` (see `.env.example` for the trial signup link). The key is only ever read server-side (`internal/config`) — it's never returned by the API or fetched by the frontend.

SportMonks scopes which leagues are fetchable at the account/plan level (you pick leagues in their dashboard), not via an API parameter — a Free Plan key only returns whatever small fixed set of leagues that plan includes. Current league priority for this project: **Brasileirão Série A, Copa Libertadores**, then **MLS, Liga MX**. Once a plan with real access to those leagues exists, put their SportMonks `league_id`s in `PRIORITY_COMPETITION_IDS` (comma-separated, priority order) — `internal/providers/sportmonks` will surface matches from those leagues first without hiding others.

## Status

Backend domain logic (reducer, feature engine, predictor, calibration, SQLite eventstore, real `/api/metrics` evaluator) and the frontend component set are implemented and tested (`make test`). Firebase (Realtime DB + Hosting), GCP deployment (Compute Engine/Terraform), and training a real model on real historical data are not yet done — those require human-gated steps (account creation, billing) documented in the blueprint's roadmap (§24, gates 2+).

## Development

```bash
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -l . (formatting check)
```
