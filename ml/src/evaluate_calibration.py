#!/usr/bin/env python3
"""
Golo ML - Calibration & Accuracy Evaluator
Calculates Brier Score, Log Loss, and Expected Calibration Error (ECE)
over predicted probabilities vs actual binary outcomes.
"""

import math
import os
import sqlite3

# Event types that count as a goal, per internal/domain/events.go IsGoalEvent().
GOAL_EVENT_TYPES = {"GOAL", "OWN_GOAL", "PENALTY_SCORED"}

def calculate_brier_score(y_true, y_prob):
    return sum((p - y) ** 2 for y, p in zip(y_true, y_prob)) / len(y_true)

def calculate_log_loss(y_true, y_prob, eps=1e-15):
    total = 0.0
    for y, p in zip(y_true, y_prob):
        p_clamped = max(eps, min(1.0 - eps, p))
        total += y * math.log(p_clamped) + (1 - y) * math.log(1 - p_clamped)
    return -total / len(y_true)

def calculate_ece(y_true, y_prob, n_bins=10):
    bin_boundaries = [i / n_bins for i in range(n_bins + 1)]
    ece = 0.0
    n = len(y_true)

    for i in range(n_bins):
        low, high = bin_boundaries[i], bin_boundaries[i+1]
        bin_items = [(y, p) for y, p in zip(y_true, y_prob) if low <= p < high or (i == n_bins - 1 and p == high)]

        if bin_items:
            bin_size = len(bin_items)
            avg_prob = sum(p for _, p in bin_items) / bin_size
            acc = sum(y for y, _ in bin_items) / bin_size
            ece += (bin_size / n) * abs(acc - avg_prob)

    return ece

def load_real_dataset(db_path):
    """
    Loads (y_true, y_prob) for the "goal before full time" horizon directly
    from the golo SQLite database, mirroring the labeling logic in
    internal/evaluation/evaluation.go's BuildFullTimeSamples: a prediction is
    only included once the match has progressed past its as-of instant
    (endSecond > asOf), and is labeled 1 if any goal event occurs afterward.

    This duplicates that Go logic in Python deliberately — the point of this
    script is to cross-check the Go-computed /api/metrics numbers with an
    independent implementation, per the blueprint's Definition of Done
    (§27: "Python e Go geram o mesmo resultado dentro de tolerância").
    Returns (y_true, y_prob), both empty if no predictions can yet be resolved.
    """
    conn = sqlite3.connect(db_path)
    try:
        cur = conn.cursor()

        cur.execute("SELECT match_id, as_of_second, prob_ft FROM predictions")
        predictions = cur.fetchall()

        cur.execute("SELECT match_id, match_second, event_type FROM events")
        goal_seconds = {}
        end_seconds = {}
        for match_id, match_second, event_type in cur.fetchall():
            if match_second > end_seconds.get(match_id, -1):
                end_seconds[match_id] = match_second
            if event_type in GOAL_EVENT_TYPES:
                goal_seconds.setdefault(match_id, []).append(match_second)
    finally:
        conn.close()

    y_true, y_prob = [], []
    for match_id, as_of_second, prob_ft in predictions:
        end_second = end_seconds.get(match_id)
        if end_second is None or end_second <= as_of_second:
            continue  # outcome not resolved yet

        observed = 1 if any(s > as_of_second for s in goal_seconds.get(match_id, [])) else 0
        y_true.append(observed)
        y_prob.append(prob_ft)

    return y_true, y_prob


if __name__ == "__main__":
    db_path = os.environ.get("GOLO_DB_PATH", "./golo.db")
    y_true, y_prob = ([], [])
    if os.path.exists(db_path):
        y_true, y_prob = load_real_dataset(db_path)

    if y_true:
        print(f"--- Golo Evaluation Metrics (real data: {db_path}, n={len(y_true)}) ---")
    else:
        # Fall back to the synthetic sample if there's no resolvable real
        # data yet (e.g. a fresh database with no finished matches).
        print(f"--- Golo Evaluation Metrics (synthetic fallback: no resolved outcomes in {db_path}) ---")
        y_true = [0, 0, 1, 0, 1, 1, 0, 1, 0, 1]
        y_prob = [0.12, 0.25, 0.78, 0.35, 0.85, 0.62, 0.15, 0.91, 0.40, 0.70]

    bs = calculate_brier_score(y_true, y_prob)
    ll = calculate_log_loss(y_true, y_prob)
    ece = calculate_ece(y_true, y_prob)

    print(f"Brier Score: {bs:.4f}")
    print(f"Log Loss:    {ll:.4f}")
    print(f"ECE:         {ece:.4f}")
