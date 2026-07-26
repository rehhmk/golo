#!/usr/bin/env python3
"""
Golo ML - Poisson hazard trainer.

Fits the goal-arrival intensity used by internal/predictor/hazard.go against
real historical matches, and exports models/hazard_v1.json.

Model
-----
Goals arrive as a Poisson process with intensity lambda(t, state). The
probability of at least one goal over a window of w seconds is

    P = 1 - exp(-lambda * w)

which is what makes the horizons nest correctly and converge at full time.
This script fits lambda by Poisson regression on goal counts over one-minute
slices of match time, with the slice length as exposure:

    lambda = (base / 5400) * exp(beta . x)

What is fitted, and what is not
-------------------------------
Fitted from data: the base rate and how intensity varies with match time,
scoreline and red cards.

NOT fitted: the activity terms (shots, shots on target, corners, dangerous
attacks). SportMonks stores those as cumulative fixture statistics rather than
timestamped events, so for a finished match only final totals exist and no
point-in-time value can be reconstructed. Those coefficients remain a prior.

To stop that prior from silently inflating the fitted base, the activity terms
are *centered*: each is expressed relative to a typical value, so a match with
ordinary activity gets a multiplier of 1.0 and only above-average pressure
raises the intensity. The centering constants come from measured per-10-minute
rates (ml/data/activity_baseline.json) when available, and from documented
defaults otherwise.

Validation holds out the most recent season of every league — the same
direction as real deployment, so nothing leaks backwards from the future.

Usage:
    python ml/src/train_baseline.py
"""

import hashlib
import json
import math
import pathlib
import sys

import numpy as np
import pandas as pd
from sklearn.linear_model import PoissonRegressor

REGULATION_SECONDS = 5400.0
SLICE_SECONDS = 60.0

ROOT = pathlib.Path(__file__).resolve().parents[2]
DATA = ROOT / "ml" / "data" / "fixtures.jsonl"
ACTIVITY_BASELINE = ROOT / "ml" / "data" / "activity_baseline.json"
OUT = ROOT / "models" / "hazard_v1.json"
REPORT = ROOT / "ml" / "reports" / "hazard_v1_training.md"

# Fitted covariates, in the order the artifact records them. These names must
# match the keys internal/features produces, since Go looks each one up by name.
FEATURES = ["match_time_frac", "match_time_frac_sq", "abs_score_diff", "goals_so_far"]
RED_FEATURE = "abs_red_diff"

# Activity priors. Not fitted — see the module docstring. The values are
# intensity multipliers per unit above the typical rate.
ACTIVITY_PRIORS = {
    "shots_10m_total": 0.06,
    "shots_on_target_10m_total": 0.12,
    "corners_10m_total": 0.03,
    "dangerous_attacks_10m_total": 0.008,
    "xg_10m_total": 0.9,
}

# Fallback typical per-10-minute values, used when no measured baseline exists.
# Derived from routine full-match totals (both teams combined): ~21 shots,
# ~7 on target, ~10 corners, ~65 dangerous attacks over ~95 minutes.
DEFAULT_ACTIVITY_CENTERS = {
    "shots_10m_total": 2.2,
    "shots_on_target_10m_total": 0.75,
    "corners_10m_total": 1.05,
    "dangerous_attacks_10m_total": 6.8,
    "xg_10m_total": 0.0,  # unavailable on this plan; stays at zero
}


def load_matches():
    if not DATA.exists():
        sys.exit(f"{DATA} not found — run ml/src/build_dataset.py first")
    rows = [json.loads(line) for line in DATA.read_text().splitlines() if line.strip()]
    if not rows:
        sys.exit("dataset is empty")
    return rows


def build_slices(matches):
    """Expand each match into one-minute slices with the state at the start of
    the slice and the number of goals scored during it."""
    records = []
    for m in matches:
        end = float(m["end_second"])
        goals = m["goals"]
        reds = m["red_cards"]
        goal_secs = np.array([g["second"] for g in goals], dtype=float)
        goal_home = np.array([bool(g["home"]) for g in goals], dtype=bool)

        t = 0.0
        while t < end:
            slice_end = min(t + SLICE_SECONDS, end)
            exposure = slice_end - t
            if exposure <= 0:
                break

            before = goal_secs <= t
            score_home = int(np.sum(before & goal_home))
            score_away = int(np.sum(before & ~goal_home))

            red_home = sum(1 for r in reds if r["second"] <= t and r["home"])
            red_away = sum(1 for r in reds if r["second"] <= t and not r["home"])

            in_slice = int(np.sum((goal_secs > t) & (goal_secs <= slice_end)))

            records.append(
                {
                    "fixture_id": m["fixture_id"],
                    "league_id": m["league_id"],
                    "season": m["season"],
                    "t": t,
                    "end_second": end,
                    "exposure": exposure,
                    "goals_in_slice": in_slice,
                    "match_second": t,
                    # Time enters as a fraction of regulation plus its square:
                    # the observed intensity rises steeply out of a cautious
                    # opening (1.93 goals/90 in the first ten minutes) then
                    # flattens near 2.5-2.8, which a linear term cannot bend to.
                    "match_time_frac": t / REGULATION_SECONDS,
                    "match_time_frac_sq": (t / REGULATION_SECONDS) ** 2,
                    "abs_score_diff": abs(score_home - score_away),
                    "goals_so_far": score_home + score_away,
                    "abs_red_diff": abs(red_home - red_away),
                    "goal_secs_after": None,
                }
            )
            t = slice_end

    return pd.DataFrame.from_records(records)


def add_horizon_labels(df, matches):
    """Label each slice with whether a goal followed within 5m, 10m, and before
    the final whistle."""
    by_id = {m["fixture_id"]: np.array([g["second"] for g in m["goals"]], dtype=float) for m in matches}

    next5, next10, before_ft = [], [], []
    for fixture_id, t, end in zip(df["fixture_id"].values, df["t"].values, df["end_second"].values):
        secs = by_id[fixture_id]
        after = secs[secs > t]
        nxt = after[0] if after.size else math.inf
        next5.append(1 if nxt <= min(t + 300, end) else 0)
        next10.append(1 if nxt <= min(t + 600, end) else 0)
        before_ft.append(1 if nxt <= end else 0)

    df["label_5m"] = next5
    df["label_10m"] = next10
    df["label_ft"] = before_ft
    return df


def holdout_split(df):
    """Hold out the most recent season of each league."""
    latest = df.groupby("league_id")["season"].max().to_dict()
    is_test = df.apply(lambda r: r["season"] == latest[r["league_id"]], axis=1)
    return df[~is_test].copy(), df[is_test].copy(), latest


class FittedHazard:
    """Poisson intensity model in raw feature units.

    The fit is done on standardised features and the coefficients are
    transformed back afterwards. Fitting on raw units does not work here:
    match_second spans 0-5400 while the other covariates span 0-9, and the
    solver stops after a single iteration with every coefficient at exactly
    zero, silently degenerating into an intercept-only model.
    """

    def __init__(self, names, coefficients, intercept):
        self.names = list(names)
        self.coefficients = dict(zip(names, coefficients))
        self.intercept = float(intercept)

    def rate(self, X):
        """Goals per second for each row of X (columns ordered as self.names)."""
        beta = np.array([self.coefficients[n] for n in self.names], dtype=float)
        return np.exp(self.intercept + X @ beta)


def fit(df_train):
    names = FEATURES + [RED_FEATURE]
    X = df_train[names].to_numpy(dtype=float)
    y = df_train["goals_in_slice"].to_numpy(dtype=float)
    exposure = df_train["exposure"].to_numpy(dtype=float)

    mu = X.mean(axis=0)
    sd = X.std(axis=0)
    sd[sd == 0] = 1.0
    Xs = (X - mu) / sd

    # PoissonRegressor optimises a weighted mean of y/exposure, which is
    # equivalent to a log-link count model with log(exposure) as an offset.
    model = PoissonRegressor(alpha=1e-8, max_iter=5000, tol=1e-12)
    model.fit(Xs, y / exposure, sample_weight=exposure)
    if model.n_iter_ <= 1:
        raise RuntimeError(f"Poisson fit did not converge (n_iter={model.n_iter_})")

    raw_coef = model.coef_ / sd
    raw_intercept = model.intercept_ - float(np.sum(model.coef_ * mu / sd))
    return FittedHazard(names, raw_coef, raw_intercept)


def probabilities(model, df, activity_centers=None):
    """Predicted P(goal) for each horizon, capped at the time remaining."""
    X = df[model.names].to_numpy(dtype=float)
    lam = model.rate(X)  # goals per second

    remaining = np.maximum(df["end_second"].to_numpy(dtype=float) - df["t"].to_numpy(dtype=float), 0.0)
    out = {}
    for key, window in (("5m", 300.0), ("10m", 600.0), ("ft", None)):
        w = remaining if window is None else np.minimum(window, remaining)
        out[key] = 1.0 - np.exp(-lam * w)
    return out, lam


def ece(probs, labels, bins=10):
    edges = np.linspace(0, 1, bins + 1)
    total, n = 0.0, len(probs)
    for i in range(bins):
        lo, hi = edges[i], edges[i + 1]
        mask = (probs >= lo) & (probs < hi) if i < bins - 1 else (probs >= lo) & (probs <= hi)
        if not mask.any():
            continue
        total += mask.sum() / n * abs(probs[mask].mean() - labels[mask].mean())
    return total


def metrics(probs, labels):
    p = np.clip(probs, 1e-6, 1 - 1e-6)
    return {
        "brier": float(np.mean((p - labels) ** 2)),
        "log_loss": float(-np.mean(labels * np.log(p) + (1 - labels) * np.log(1 - p))),
        "ece": float(ece(p, labels)),
        "mean_pred": float(p.mean()),
        "base_rate": float(labels.mean()),
    }


def constant_rate_baseline(df_train, df_test):
    """The honest null model: one flat league-average intensity."""
    lam = df_train["goals_in_slice"].sum() / df_train["exposure"].sum()
    remaining = np.maximum(df_test["end_second"] - df_test["t"], 0.0).to_numpy(dtype=float)
    out = {}
    for key, window in (("5m", 300.0), ("10m", 600.0), ("ft", None)):
        w = remaining if window is None else np.minimum(window, remaining)
        out[key] = 1.0 - np.exp(-lam * w)
    return out, lam


def reliability_table(probs, labels, bins=10):
    edges = np.linspace(0, 1, bins + 1)
    lines = ["| predicted | observed | n |", "|---|---|---|"]
    for i in range(bins):
        lo, hi = edges[i], edges[i + 1]
        mask = (probs >= lo) & (probs < hi) if i < bins - 1 else (probs >= lo) & (probs <= hi)
        if mask.sum() < 50:
            continue
        lines.append(f"| {probs[mask].mean():.3f} | {labels[mask].mean():.3f} | {int(mask.sum())} |")
    return "\n".join(lines)


def load_activity_centers():
    if ACTIVITY_BASELINE.exists():
        measured = json.loads(ACTIVITY_BASELINE.read_text())
        centers = dict(DEFAULT_ACTIVITY_CENTERS)
        centers.update({k: v for k, v in measured.items() if k in ACTIVITY_PRIORS})
        return centers, "measured from historical fixture statistics"
    return dict(DEFAULT_ACTIVITY_CENTERS), "documented defaults (no measured baseline found)"


def main():
    matches = load_matches()
    print(f"loaded {len(matches)} matches")

    df = build_slices(matches)
    df = add_horizon_labels(df, matches)
    print(f"built {len(df)} one-minute slices")

    df_train, df_test, latest = holdout_split(df)
    print(f"train {len(df_train)} slices / test {len(df_test)} slices")
    print(f"held out seasons: {latest}")

    model = fit(df_train)
    coefs = model.coefficients
    base_goals_per_90 = math.exp(model.intercept) * REGULATION_SECONDS

    print()
    print(f"fitted base rate: {base_goals_per_90:.3f} goals/90min at reference state")
    for name, value in coefs.items():
        print(f"  {name:18s} {value:+.8f}")

    fitted_probs, lam = probabilities(model, df_test)
    const_probs, const_lam = constant_rate_baseline(df_train, df_test)

    report = ["# Hazard model v1 — training report", ""]
    report.append(f"- Matches: {len(matches)} ({df['fixture_id'].nunique()} distinct)")
    report.append(f"- Slices: {len(df)} ({len(df_train)} train / {len(df_test)} test)")
    report.append(f"- Held-out seasons: {latest}")
    report.append(f"- Observed goals per match: {sum(len(m['goals']) for m in matches)/len(matches):.3f}")
    report.append(f"- Constant-rate baseline: {const_lam*REGULATION_SECONDS:.3f} goals/90min")
    report.append(f"- Fitted base rate at reference state: {base_goals_per_90:.3f} goals/90min")
    report.append("")
    report.append("## Fitted coefficients")
    report.append("")
    report.append("| term | coefficient |")
    report.append("|---|---|")
    for name, value in coefs.items():
        report.append(f"| {name} | {value:+.8f} |")
    report.append("")
    report.append("## Held-out performance")
    report.append("")
    report.append("| horizon | model | Brier | LogLoss | ECE | mean pred | base rate |")
    report.append("|---|---|---|---|---|---|---|")

    print()
    print(f"{'horizon':8s} {'model':10s} {'Brier':>8s} {'LogLoss':>9s} {'ECE':>8s} {'pred':>7s} {'actual':>7s}")
    summary = {}
    for key, label_col in (("5m", "label_5m"), ("10m", "label_10m"), ("ft", "label_ft")):
        labels = df_test[label_col].to_numpy(dtype=float)
        m_fit = metrics(fitted_probs[key], labels)
        m_const = metrics(const_probs[key], labels)
        summary[key] = {"fitted": m_fit, "constant": m_const}
        for name, m in (("fitted", m_fit), ("constant", m_const)):
            print(f"{key:8s} {name:10s} {m['brier']:8.5f} {m['log_loss']:9.5f} {m['ece']:8.5f} {m['mean_pred']:7.4f} {m['base_rate']:7.4f}")
            report.append(
                f"| {key} | {name} | {m['brier']:.5f} | {m['log_loss']:.5f} | {m['ece']:.5f} | {m['mean_pred']:.4f} | {m['base_rate']:.4f} |"
            )

    report.append("")
    report.append("## Reliability — goal before full time (held out)")
    report.append("")
    report.append(reliability_table(fitted_probs["ft"], df_test["label_ft"].to_numpy(dtype=float)))

    # Validation above is what the reported metrics describe. The artifact
    # itself is refitted on every season including the held-out one: holding
    # data back is how you measure honestly, not how you ship. Scoring rates
    # drift upward across these seasons (Brasileirao: 2.36 -> 2.53 -> 2.65
    # goals/90), so shipping a model blind to the most recent one would bake
    # in a known-stale base rate.
    final_model = fit(df)
    final_coefs = final_model.coefficients
    final_base = math.exp(final_model.intercept) * REGULATION_SECONDS
    print()
    print(f"refit on all {len(df)} slices: base {final_base:.3f} goals/90min")

    report.append("")
    report.append("## Shipped artifact")
    report.append("")
    report.append(
        f"Validation used a held-out season; the exported artifact is refitted on all "
        f"{len(df)} slices from {len(matches)} matches. Base rate {final_base:.4f} goals/90 "
        f"at the reference state (kickoff, level, no red cards)."
    )
    report.append("")
    report.append("| term | coefficient |")
    report.append("|---|---|")
    for name, value in final_coefs.items():
        report.append(f"| {name} | {value:+.8f} |")

    centers, centers_source = load_activity_centers()

    artifact = {
        "modelVersion": "hazard_v1.1.0",
        "featureVersion": "v1.1.0",
        "modelType": "poisson_hazard",
        "baseGoalsPer90": round(final_base, 6),
        "coefficients": {name: round(float(v), 8) for name, v in final_coefs.items() if name != RED_FEATURE},
        "redCardCoefficient": round(float(final_coefs[RED_FEATURE]), 8),
        "activityCoefficients": {k: v for k, v in ACTIVITY_PRIORS.items()},
        "activityCenters": {k: round(float(v), 6) for k, v in centers.items()},
        "minMultiplier": 0.25,
        "maxMultiplier": 4.0,
        "trainedUntil": max(m["starting_at"] or "" for m in matches)[:10],
        "trainingMatches": len(matches),
        "notes": (
            "Base rate and the match_second / abs_score_diff / goals_so_far / red-card terms are "
            "fitted by Poisson regression on {n} historical matches, validated on a held-out most-recent "
            "season per league. The activityCoefficients are NOT fitted: SportMonks stores shots, corners "
            "and attacks as cumulative fixture statistics rather than timestamped events, so no "
            "point-in-time value is recoverable from history. They are applied relative to activityCenters "
            "so ordinary activity leaves the fitted intensity unchanged. xG is unavailable on this plan and "
            "stays at zero."
        ).format(n=len(matches)),
    }

    serialized = json.dumps(
        {k: artifact[k] for k in ("baseGoalsPer90", "coefficients", "redCardCoefficient", "activityCoefficients", "activityCenters")},
        sort_keys=True,
    ).encode()
    artifact["sha256"] = hashlib.sha256(serialized).hexdigest()

    OUT.write_text(json.dumps(artifact, indent=2) + "\n")
    REPORT.parent.mkdir(parents=True, exist_ok=True)
    REPORT.write_text("\n".join(report) + "\n")

    print()
    print(f"wrote {OUT}")
    print(f"wrote {REPORT}")
    print(f"activity centering: {centers_source}")


if __name__ == "__main__":
    main()
