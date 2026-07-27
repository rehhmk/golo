#!/usr/bin/env python3
"""
Does a repricing window exist after a goal?

Reads what collect_latency.py recorded and answers one question: when a goal
is scored, how long does a bookmaker take to move its price, and is the stale
price wide enough to be worth acting on?

The measurement
---------------
A goal makes another goal *arithmetically closer* for an over line — a 1-0 at
the 30th minute means "over 2.5" now needs one goal rather than two. The true
probability jumps at the instant of the goal. The price should follow.

For each goal detected, this reports:

  * detected_at   — when our own poll first saw the new score
  * moved_at      — when the bookmaker's price first changed materially
  * window        — the gap between them, which is the only thing that matters

A window is only exploitable if it is longer than the time it would take to
place a bet. A five-second window is a fact about the market and useless in
practice; a ninety-second window is a business.

Two honest caveats are enforced in the output rather than left to the reader:

  * Our detection is not the goal. It is the first poll that saw it, so every
    window reported here is an *upper bound* on the real one — the true window
    starts earlier than we can see, and is therefore shorter than we measure.
  * A price that moves is not proof the bookmaker reacted to the goal. Prices
    drift. The size of the move is reported so drift and reaction can be told
    apart.

Usage:
    python ml/src/analyze_latency.py
"""

import datetime
import json
import pathlib
import statistics
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
DATA = ROOT / "ml" / "data" / "latency"

# Below this the price moved, but not by enough to distinguish a reaction to
# the goal from ordinary drift.
MATERIAL_MOVE = 0.05

# A bet cannot realistically be placed faster than this, so anything shorter
# is a curiosity rather than an opportunity.
USABLE_WINDOW_SECONDS = 20


def parse(ts):
    return datetime.datetime.fromisoformat(str(ts).replace("Z", "+00:00"))


def load(pattern):
    rows = []
    for path in sorted(DATA.glob(pattern)):
        for line in path.read_text().splitlines():
            if line.strip():
                rows.append(json.loads(line))
    return rows


def find_goals(match_rows):
    """A goal is the first poll where a fixture's total score increased."""
    by_fixture = {}
    for row in match_rows:
        by_fixture.setdefault(row["fixture_id"], []).append(row)

    goals = []
    for fixture_id, rows in by_fixture.items():
        rows.sort(key=lambda r: r["observed_at"])
        previous = None
        for row in rows:
            if row["home_goals"] is None or row["away_goals"] is None:
                continue
            total = row["home_goals"] + row["away_goals"]
            if previous is not None and total > previous:
                goals.append({
                    "fixture_id": fixture_id,
                    "name": row["name"],
                    "detected_at": row["observed_at"],
                    "minute": row["minute"],
                    "score": f"{row['home_goals']}-{row['away_goals']}",
                    "total_after": total,
                })
            previous = total
    return goals


def match_event(goal, odds_rows):
    """Odds and match feeds name teams differently; match on a surname token."""
    home = (goal["name"] or "").split(" vs ")[0].strip().lower()
    if not home:
        return None
    tokens = [t for t in home.split() if len(t) > 3]
    for row in odds_rows:
        target = (row.get("home_team") or "").lower()
        if any(t in target for t in tokens):
            return row.get("event_id")
    return None


def analyse_goal(goal, odds_rows):
    event_id = match_event(goal, odds_rows)
    if not event_id:
        return None

    detected = parse(goal["detected_at"])
    series = [
        r for r in odds_rows
        if r.get("event_id") == event_id and r.get("name") == "Over"
    ]
    if not series:
        return None

    # A price series is only comparable within one line. Bookmakers quote
    # several totals simultaneously — Pinnacle carried Over 2.25, 2.5 and 2.75
    # at the same instant — and they also move the line itself after a goal.
    # Grouping by bookmaker alone compares Over 2.5 before the goal against
    # Over 3.5 after it and reads the difference as a repricing delay. That is
    # what produced an apparent 143-second median window on the first run.
    by_bookmaker = {}
    for row in series:
        key = (row["bookmaker"], row.get("point"))
        by_bookmaker.setdefault(key, []).append(row)

    results = []
    for (bookmaker, point), rows in by_bookmaker.items():
        rows.sort(key=lambda r: r["observed_at"])
        before = [r for r in rows if parse(r["observed_at"]) <= detected]
        after = [r for r in rows if parse(r["observed_at"]) > detected]
        if not before or not after:
            continue

        baseline = before[-1]["price"]
        for row in after:
            # Direction matters. One more goal already scored makes an over
            # line easier to reach, so a genuine reaction lowers the price. A
            # rise is drift or an unrelated move and is not evidence of the
            # bookmaker reacting to this goal.
            if baseline - row["price"] >= MATERIAL_MOVE:
                window = (parse(row["observed_at"]) - detected).total_seconds()
                results.append({
                    "bookmaker": f"{bookmaker} @{point}",
                    "price_before": baseline,
                    "price_after": row["price"],
                    "move": round(row["price"] - baseline, 3),
                    "window_seconds": round(window, 1),
                })
                break
        else:
            results.append({
                "bookmaker": f"{bookmaker} @{point}",
                "price_before": baseline,
                "price_after": after[-1]["price"],
                "move": round(after[-1]["price"] - baseline, 3),
                "window_seconds": None,  # never moved materially while observed
            })
    return results


def main():
    match_rows = load("matches_*.jsonl")
    odds_rows = load("odds_*.jsonl")

    if not match_rows:
        sys.exit(f"no collected data in {DATA} — run collect_latency.py first")

    fixtures = {r["fixture_id"] for r in match_rows}
    print(f"dados: {len(match_rows)} leituras de placar, {len(odds_rows)} de odds, {len(fixtures)} partidas")

    goals = find_goals(match_rows)
    print(f"gols detectados: {len(goals)}")
    if not goals:
        print("\nsem gol observado — nada a medir ainda.")
        return
    if not odds_rows:
        print("\nsem leitura de odds — nada a medir.")
        return

    print()
    windows = []
    for goal in goals:
        results = analyse_goal(goal, odds_rows)
        if not results:
            print(f"  {goal['name']} {goal['score']} aos {goal['minute']}' — sem odds casadas")
            continue

        print(f"  {goal['name']}  {goal['score']}  aos {goal['minute']}'")
        for r in sorted(results, key=lambda x: (x["window_seconds"] is None, x["window_seconds"] or 0)):
            if r["window_seconds"] is None:
                print(f"    {r['bookmaker']:16s} nao moveu materialmente (delta {r['move']:+.2f})")
            else:
                windows.append(r["window_seconds"])
                print(f"    {r['bookmaker']:16s} {r['price_before']:.2f} -> {r['price_after']:.2f} "
                      f"({r['move']:+.2f}) em {r['window_seconds']:.0f}s")
        print()

    if not windows:
        print("VEREDITO: nenhuma casa moveu o preco materialmente apos um gol.")
        print("Ou a janela e menor que o intervalo de coleta, ou nao existe.")
        return

    usable = [w for w in windows if w >= USABLE_WINDOW_SECONDS]
    print("=" * 64)
    print(f"janelas medidas   : {len(windows)}")
    print(f"mediana           : {statistics.median(windows):.0f}s")
    print(f"minima / maxima   : {min(windows):.0f}s / {max(windows):.0f}s")
    print(f"acima de {USABLE_WINDOW_SECONDS}s      : {len(usable)} de {len(windows)}")
    print()
    print("LEMBRETE: estas janelas sao limite SUPERIOR. A contagem comeca quando")
    print("nossa coleta viu o gol, nao quando ele aconteceu — a janela real e menor.")
    if not usable:
        print()
        print("VEREDITO: nenhuma janela util. Velocidade nao e uma vantagem aqui.")


if __name__ == "__main__":
    main()
