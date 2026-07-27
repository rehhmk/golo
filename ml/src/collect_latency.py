#!/usr/bin/env python3
"""
Measure whether a repricing window exists after a goal.

The hypothesis being tested: when a goal is scored, the true probability of a
further goal changes instantly, but a bookmaker takes some seconds to move its
price. If that gap is wide enough and lasts long enough, the stale price is
beatable — and speed, rather than prediction, would be the edge.

This records two clocks side by side and nothing else:

  * match state from SportMonks (score, minute), polled fast because that
    quota is plentiful;
  * totals odds from The Odds API, polled slower because the free tier allows
    500 requests a month.

Both rows carry the wall clock at which they were observed, plus the
bookmaker's own last_update. Those are different things and the difference
matters: last_update is when the bookmaker changed the price, observed_at is
when we could first have acted on it. The exploitable window is bounded by the
second, not the first.

Nothing is analysed here. Collection and analysis are kept apart so that the
question asked of the data cannot drift to fit what the data happens to show.

Usage:
    python ml/src/collect_latency.py --minutes 150 --odds-interval 30
"""

import argparse
import datetime
import json
import pathlib
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[2]
OUT_DIR = ROOT / "ml" / "data" / "latency"

SPORTMONKS_BASE = "https://api.sportmonks.com/v3/football"
ODDS_BASE = "https://api.the-odds-api.com/v4"

# Only competitions the live engine actually follows.
SPORT_KEYS = [
    "soccer_brazil_campeonato",
    "soccer_conmebol_copa_libertadores",
    "soccer_usa_mls",
    "soccer_mexico_ligamx",
]


def env(name: str) -> str:
    path = ROOT / ".env"
    if path.exists():
        for line in path.read_text().splitlines():
            if line.startswith(f"{name}="):
                return line.split("=", 1)[1].strip()
    sys.exit(f"{name} not found in .env")


def get(url: str, timeout: int = 25):
    req = urllib.request.Request(url, headers={"User-Agent": "golo-latency/1.0"})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        remaining = resp.headers.get("x-requests-remaining")
        return json.loads(resp.read()), remaining


def now_iso() -> str:
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def poll_matches(key: str):
    """Live score and clock. Cheap relative to its quota, so polled often."""
    params = urllib.parse.urlencode(
        {"api_token": key, "include": "participants;periods;scores;league"}
    )
    payload, _ = get(f"{SPORTMONKS_BASE}/livescores/inplay?{params}")
    rows = []
    for fixture in payload.get("data") or []:
        ticking = [p for p in (fixture.get("periods") or []) if p.get("ticking")]
        minute = ticking[0].get("minutes") if ticking else None

        score = {}
        for entry in fixture.get("scores") or []:
            if entry.get("description") == "CURRENT":
                side = (entry.get("score") or {}).get("participant")
                score[side] = (entry.get("score") or {}).get("goals")

        rows.append({
            "observed_at": now_iso(),
            "fixture_id": fixture.get("id"),
            "name": fixture.get("name"),
            "league": (fixture.get("league") or {}).get("name"),
            "state_id": fixture.get("state_id"),
            "minute": minute,
            "home_goals": score.get("home"),
            "away_goals": score.get("away"),
        })
    return rows


def poll_odds(key: str, sport_key: str):
    """Totals odds for one competition. One request per competition per call."""
    params = urllib.parse.urlencode({
        "apiKey": key,
        "regions": "eu",
        "markets": "totals",
        "oddsFormat": "decimal",
    })
    payload, remaining = get(f"{ODDS_BASE}/sports/{sport_key}/odds/?{params}")
    if isinstance(payload, dict):
        return [], remaining

    observed = now_iso()
    rows = []
    for event in payload:
        for bookmaker in event.get("bookmakers") or []:
            for market in bookmaker.get("markets") or []:
                if market.get("key") != "totals":
                    continue
                for outcome in market.get("outcomes") or []:
                    rows.append({
                        "observed_at": observed,
                        "event_id": event.get("id"),
                        "home_team": event.get("home_team"),
                        "away_team": event.get("away_team"),
                        "commence_time": event.get("commence_time"),
                        "bookmaker": bookmaker.get("title"),
                        "last_update": bookmaker.get("last_update"),
                        "name": outcome.get("name"),
                        "point": outcome.get("point"),
                        "price": outcome.get("price"),
                    })
    return rows, remaining


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--minutes", type=int, default=150, help="how long to collect")
    ap.add_argument("--odds-interval", type=int, default=30, help="seconds between odds polls")
    ap.add_argument("--match-interval", type=int, default=15, help="seconds between state polls")
    ap.add_argument("--min-quota", type=int, default=120, help="stop before the quota falls below this")
    args = ap.parse_args()

    sm_key, odds_key = env("SPORTMONKS_API_KEY"), env("ODDS_API_KEY")
    OUT_DIR.mkdir(parents=True, exist_ok=True)

    stamp = datetime.datetime.now().strftime("%Y%m%d_%H%M")
    matches_path = OUT_DIR / f"matches_{stamp}.jsonl"
    odds_path = OUT_DIR / f"odds_{stamp}.jsonl"

    deadline = time.time() + args.minutes * 60
    next_odds = 0.0
    remaining = None
    odds_polls = match_polls = 0

    print(f"collecting to {OUT_DIR}", flush=True)

    with matches_path.open("a") as mf, odds_path.open("a") as of:
        while time.time() < deadline:
            try:
                rows = poll_matches(sm_key)
                match_polls += 1
                for row in rows:
                    mf.write(json.dumps(row) + "\n")
                mf.flush()
                live = [r for r in rows if r["state_id"] in (2, 22, 3)]
            except Exception as exc:  # transient network or upstream error
                print(f"  match poll failed: {exc}", flush=True)
                live = []

            # Odds cost real quota, so they are only fetched while something is
            # actually being played — a stale price matters only during a match.
            if live and time.time() >= next_odds:
                if remaining is not None and int(remaining) <= args.min_quota:
                    print(f"stopping: quota floor reached ({remaining} left)", flush=True)
                    break

                leagues = {r["league"] for r in live if r["league"]}
                for sport_key in sport_keys_for(leagues):
                    try:
                        rows, remaining = poll_odds(odds_key, sport_key)
                        odds_polls += 1
                        for row in rows:
                            of.write(json.dumps(row) + "\n")
                        of.flush()
                    except Exception as exc:
                        print(f"  odds poll failed ({sport_key}): {exc}", flush=True)

                next_odds = time.time() + args.odds_interval
                scores = ", ".join(
                    f"{r['name']} {r['home_goals']}-{r['away_goals']} {r['minute']}'" for r in live[:3]
                )
                print(f"  [{datetime.datetime.now():%H:%M:%S}] quota={remaining} | {scores}", flush=True)

            time.sleep(args.match_interval)

    print(f"done: {match_polls} match polls, {odds_polls} odds polls, quota left {remaining}")
    print(f"  {matches_path}")
    print(f"  {odds_path}")


def sport_keys_for(league_names):
    """Map live SportMonks league names onto Odds API sport keys, so quota is
    only spent on competitions that actually have a match in progress."""
    mapping = {
        "Serie A": "soccer_brazil_campeonato",
        "Copa Libertadores": "soccer_conmebol_copa_libertadores",
        "Major League Soccer": "soccer_usa_mls",
        "Liga MX": "soccer_mexico_ligamx",
    }
    keys = {mapping[name] for name in league_names if name in mapping}
    return sorted(keys) or []


if __name__ == "__main__":
    main()
