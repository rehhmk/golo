#!/usr/bin/env python3
"""
Golo ML - historical dataset builder.

Pulls finished fixtures for the configured leagues/seasons from SportMonks and
caches the fields needed to reconstruct a match minute by minute: goal times,
card times, timestamped activity events, and the true end-of-match clock from
the period list.

Activity is only retained when the provider supplies a genuine timestamped
timeline event. Final cumulative fixture statistics are never spread backward
over a match: doing so would leak future information into the training rows.

Output: ml/data/fixtures.jsonl, one match per line. Re-running is cheap: the
raw pages are cached under ml/data/raw/ and only missing pages are fetched.

Usage:
    python ml/src/build_dataset.py [--max-pages-per-season N]
"""

import argparse
import json
import os
import pathlib
import sys
import time
import urllib.parse
import urllib.request

BASE_URL = "https://api.sportmonks.com/v3/football"

# Verified against /leagues on the current plan (2026-07-26).
LEAGUES = {
    # The four the live engine follows.
    648: "Brasileirao Serie A",
    1122: "Copa Libertadores",
    779: "Major League Soccer",
    743: "Liga MX",
    # Everything else the plan covers. These add roughly eight times the
    # volume — Champions and Europa League carry 27 seasons each — which is
    # what the coefficient stability sweep needs: at 3,287 matches, the red
    # card, corner and goals-so-far terms all change sign under an 80%
    # resample, so they are indistinguishable from noise. More matches either
    # settles them into real effects or removes them for good.
    #
    # Competition is already a stratum in the trainer, so mixing a European
    # goal rate with a South American one does not contaminate the base rate.
    2: "Champions League",
    5: "Europa League",
    271: "Superliga",
    1328: "UEFA Super Cup",
    2286: "Europa Conference League",
}

# SportMonks state_id values that mean "this match was played to a finish".
FINISHED_STATES = {5, 7, 8}

# Event type_ids, confirmed against live and historical fixtures.
TYPE_GOAL = 14
TYPE_PENALTY_SCORED = 16
TYPE_OWN_GOAL = 15
TYPE_YELLOW = 19
TYPE_RED = 20
TYPE_SECOND_YELLOW = 21

GOAL_TYPES = {TYPE_GOAL, TYPE_PENALTY_SCORED, TYPE_OWN_GOAL}
RED_TYPES = {TYPE_RED, TYPE_SECOND_YELLOW}

DATA_DIR = pathlib.Path(__file__).resolve().parents[1] / "data"
RAW_DIR = DATA_DIR / "raw"

# Bumped whenever the requested include changes. Cached pages are keyed by it
# so a schema change cannot be masked by stale files on disk: the previous
# build reused pages fetched without the timeline include and reproduced the
# same empty activity set on every re-run.
RAW_SCHEMA = "v2-timeline"

# SportMonks timeline event types, by developer_name, mapped to the feature
# the reducer counts them under. Verified live against fixture 19609643.
#
# These live in the `timeline` include, NOT in `events`. The events include
# carries only goals, cards and substitutions, which is why an earlier build
# that parsed activity out of `events` produced zero shots across all 3,282
# matches and left the model with nothing but clock and scoreline to learn
# from.
#
# A shot on target is also a shot, so it contributes to both counters —
# matching internal/reducer, where EventShotOnTarget satisfies both
# IsShotEvent() and IsOnTarget().
ACTIVITY_KINDS = {
    "SHOT_ON_TARGET": ("shots_on_target_10m_total",),
    "SHOT_OFF_TARGET": ("shots_off_target_10m_total",),
    "SHOT_BLOCKED": ("shots_off_target_10m_total",),
    "CORNER": ("corners_10m_total",),
}

# The shot counters are deliberately disjoint. Counting an on-target shot in
# both a total and an on-target column made the two columns correlate at 0.61,
# and the fit responded by splitting the effect between them with opposite
# signs: shots +0.0135 and shots_on_target -0.0031. Read literally the model
# then claimed a shot on target raises the goal rate *less* than a shot that
# missed, which is not a finding about football but an artifact of asking a
# regression to attribute one event to two overlapping variables.

# Dangerous attacks are deliberately absent: SportMonks publishes them only as
# a cumulative fixture statistic, never as a timestamped timeline entry, so no
# point-in-time value is recoverable from history. That coefficient stays a
# documented prior.


def api_key() -> str:
    key = os.environ.get("SPORTMONKS_API_KEY")
    if key:
        return key
    env_path = pathlib.Path(__file__).resolve().parents[2] / ".env"
    if env_path.exists():
        for line in env_path.read_text().splitlines():
            if line.startswith("SPORTMONKS_API_KEY="):
                return line.split("=", 1)[1].strip()
    sys.exit("SPORTMONKS_API_KEY not set (env or .env)")


def get(path: str, params: dict, key: str, retries: int = 4) -> dict:
    """GET with backoff. A 429 means the hourly quota is gone, so wait it out
    rather than hammering — the live engine shares this quota."""
    params = dict(params, api_token=key)
    url = f"{BASE_URL}{path}?{urllib.parse.urlencode(params)}"
    # SportMonks rejects the default Python-urllib agent with a 403.
    req = urllib.request.Request(url, headers={"User-Agent": "golo-dataset-builder/1.0"})
    for attempt in range(retries):
        try:
            with urllib.request.urlopen(req, timeout=45) as resp:
                return json.loads(resp.read())
        except urllib.error.HTTPError as exc:
            if exc.code == 429:
                wait = 90 * (attempt + 1)
                print(f"    rate limited, waiting {wait}s", flush=True)
                time.sleep(wait)
                continue
            raise
        except Exception as exc:  # transient network
            if attempt == retries - 1:
                raise
            print(f"    retry after {exc}", flush=True)
            time.sleep(5 * (attempt + 1))
    raise RuntimeError(f"giving up on {path}")


def seasons_for_league(league_id: int, key: str) -> list:
    payload = get(f"/leagues/{league_id}", {"include": "seasons"}, key)
    return [(s["id"], s.get("name", "?")) for s in payload["data"].get("seasons", [])]


PERIOD_FIRST_HALF = 1
PERIOD_SECOND_HALF = 2
HALF_MINUTES = 45


def half_lengths(fixture: dict):
    """Actual played length of each half in minutes, stoppage included.

    SportMonks reports a period's `minutes` inclusive of counts_from, so a
    second half that ran to 90+5 reports 95, not 50.
    """
    first = second = None
    for period in fixture.get("periods") or []:
        minutes = period.get("minutes")
        if not minutes:
            continue
        if period.get("type_id") == PERIOD_FIRST_HALF:
            first = int(minutes)
        elif period.get("type_id") == PERIOD_SECOND_HALF:
            second = int(minutes) - HALF_MINUTES
    return first, second


def event_second(ev: dict, first_half_len: int) -> int:
    """Elapsed playing time of an event, in seconds, on a continuous axis.

    Football's minute labels are ambiguous: first-half stoppage is reported as
    minute=45 with extra_minute=1..5, while genuine second-half play carries
    minute=46..90. Naively adding the offset maps a 45+1' goal and a real 46'
    goal to the same instant, and ignoring it piles every stoppage goal onto
    minutes 45 and 90. Both distort the rate, because the exposure grid counts
    nominal minutes while those buckets absorb far more real play — the first
    attempt produced phantom goal-rate spikes of 3.58 and 4.51 goals/90 at
    40-50' and 80-90' against a ~2.4 baseline.

    Anchoring the second half to where the first actually ended removes the
    ambiguity: elapsed time becomes strictly increasing and every minute of
    the axis represents one real minute of football.
    """
    minute = int(ev.get("minute") or 0)
    extra = int(ev.get("extra_minute") or 0)

    if minute <= HALF_MINUTES:
        return min(minute + extra, first_half_len) * 60
    return (first_half_len + (minute - HALF_MINUTES) + extra) * 60


def participants(fixture: dict):
    home = away = None
    for p in fixture.get("participants") or []:
        loc = (p.get("meta") or {}).get("location")
        if loc == "home":
            home = p["id"]
        elif loc == "away":
            away = p["id"]
    return home, away


def distill(fixture: dict, league_id: int, season_name: str):
    """Reduce a raw fixture to the timeline the trainer needs."""
    home, away = participants(fixture)
    if home is None or away is None:
        return None

    first_len, second_len = half_lengths(fixture)
    # Both halves must be present and plausible; anything else is an
    # abandoned match or a truncated feed, whose timeline isn't comparable.
    if first_len is None or second_len is None:
        return None
    if not (40 <= first_len <= 60) or not (40 <= second_len <= 60):
        return None

    end = (first_len + second_len) * 60

    goals, reds, activity = [], [], []
    for ev in fixture.get("events") or []:
        tid = ev.get("type_id")
        sec = event_second(ev, first_len)
        if sec > end:
            continue
        if tid in GOAL_TYPES:
            team = ev.get("participant_id")
            # An own goal credits the opposing side.
            scoring_home = (team == home)
            if tid == TYPE_OWN_GOAL:
                scoring_home = not scoring_home
            goals.append({"second": sec, "home": bool(scoring_home)})
        elif tid in RED_TYPES:
            reds.append({"second": sec, "home": ev.get("participant_id") == home})

    for entry in fixture.get("timeline") or []:
        sec = event_second(entry, first_len)
        if sec > end:
            continue
        type_obj = entry.get("type") or {}
        developer_name = type_obj.get("developer_name") if isinstance(type_obj, dict) else None
        for kind in ACTIVITY_KINDS.get(str(developer_name or "").strip().upper(), ()):
            activity.append({"second": sec, "kind": kind})

    goals.sort(key=lambda g: g["second"])
    reds.sort(key=lambda r: r["second"])
    activity.sort(key=lambda e: e["second"])

    return {
        "fixture_id": fixture["id"],
        "league_id": league_id,
        "season": season_name,
        "name": fixture.get("name"),
        "starting_at": fixture.get("starting_at"),
        "end_second": end,
        "goals": goals,
        "red_cards": reds,
        "activity_events": activity,
        "activity_timeline_available": bool(activity),
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--max-pages-per-season", type=int, default=40)
    args = ap.parse_args()

    key = api_key()
    RAW_DIR.mkdir(parents=True, exist_ok=True)

    out_path = DATA_DIR / "fixtures.jsonl"
    matches, skipped = [], 0

    for league_id, league_name in LEAGUES.items():
        for season_id, season_name in seasons_for_league(league_id, key):
            print(f"{league_name} {season_name} (season {season_id})", flush=True)
            for page in range(1, args.max_pages_per_season + 1):
                cache = RAW_DIR / f"{RAW_SCHEMA}_s{season_id}_p{page}.json"
                if cache.exists():
                    payload = json.loads(cache.read_text())
                else:
                    payload = get(
                        "/fixtures",
                        {
                            "filters": f"fixtureSeasons:{season_id}",
                            "include": "events.type;participants;periods;timeline.type",
                            "per_page": 50,
                            "page": page,
                        },
                        key,
                    )
                    cache.write_text(json.dumps(payload))
                    time.sleep(0.3)

                rows = payload.get("data") or []
                if not rows:
                    break

                for fixture in rows:
                    if fixture.get("state_id") not in FINISHED_STATES:
                        skipped += 1
                        continue
                    rec = distill(fixture, league_id, season_name)
                    if rec is None:
                        skipped += 1
                        continue
                    matches.append(rec)

                print(f"  page {page}: +{len(rows)} (kept {len(matches)})", flush=True)
                if not (payload.get("pagination") or {}).get("has_more"):
                    break

    with out_path.open("w") as fh:
        for rec in matches:
            fh.write(json.dumps(rec) + "\n")

    total_goals = sum(len(m["goals"]) for m in matches)
    print()
    print(f"wrote {len(matches)} matches to {out_path} (skipped {skipped})")
    if matches:
        print(f"total goals {total_goals}, mean {total_goals/len(matches):.2f} per match")


if __name__ == "__main__":
    main()
