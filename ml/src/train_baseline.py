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

Activity terms are fitted only when timestamped timeline events are available
for enough training matches. Legacy datasets without that timeline retain the
documented centered priors and are explicitly described as such in the
artifact. Final cumulative statistics are never used as point-in-time inputs.

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
    "shots_off_target_10m_total": 0.04,
    "shots_on_target_10m_total": 0.12,
    "corners_10m_total": 0.03,
    "dangerous_attacks_10m_total": 0.008,
    "xg_10m_total": 0.9,
}
MIN_ACTIVITY_MATCHES = 500

# Fallback typical per-10-minute values, used when no measured baseline exists.
# Derived from routine full-match totals (both teams combined): ~21 shots,
# ~7 on target, ~10 corners, ~65 dangerous attacks over ~95 minutes.
DEFAULT_ACTIVITY_CENTERS = {
    "shots_off_target_10m_total": 1.45,
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
        activity = m.get("activity_events") or []

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

            record = {
                    "fixture_id": m["fixture_id"],
                    "league_id": m["league_id"],
                    "season": m["season"],
                    "starting_at": m.get("starting_at") or "",
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
            for feature in ACTIVITY_PRIORS:
                record[feature] = sum(
                    1
                    for event in activity
                    if event.get("kind") == feature and max(0.0, t - 600.0) < float(event["second"]) <= t
                )
            records.append(record)
            t = slice_end

    return pd.DataFrame.from_records(records)


def add_horizon_labels(df, matches):
    """Label each slice with whether a goal followed within 5m, 10m, and before
    the final whistle."""
    by_id = {m["fixture_id"]: np.array([g["second"] for g in m["goals"]], dtype=float) for m in matches}

    next5, next10, before_ft, two_before_ft = [], [], [], []
    for fixture_id, t, end in zip(df["fixture_id"].values, df["t"].values, df["end_second"].values):
        secs = by_id[fixture_id]
        after = secs[secs > t]
        nxt = after[0] if after.size else math.inf
        next5.append(1 if nxt <= min(t + 300, end) else 0)
        next10.append(1 if nxt <= min(t + 600, end) else 0)
        before_ft.append(1 if nxt <= end else 0)
        two_before_ft.append(1 if int(np.sum((secs > t) & (secs <= end))) >= 2 else 0)

    df["label_5m"] = next5
    df["label_10m"] = next10
    df["label_ft"] = before_ft
    df["label_two_ft"] = two_before_ft
    return df


def training_validation_split(df):
    """Use the season with the latest actual kickoff in each league as
    Validation. Season labels are not chronologically comparable across
    competitions (e.g. "2026" versus "2026/2027"). Leagues with only one
    season cannot support an honest chronological comparison and are
    excluded from qualification."""
    season_dates = (
        df.groupby(["league_id", "season"])["starting_at"].max().reset_index()
    )
    season_counts = season_dates.groupby("league_id")["season"].nunique()
    eligible = set(season_counts[season_counts >= 2].index)
    excluded = sorted(set(season_counts.index) - eligible)
    season_dates = season_dates[season_dates["league_id"].isin(eligible)]
    newest_rows = season_dates.sort_values(
        ["league_id", "starting_at", "season"]
    ).groupby("league_id").tail(1)
    latest = dict(zip(newest_rows["league_id"], newest_rows["season"]))
    eligible_rows = df[df["league_id"].isin(eligible)].copy()
    is_validation = eligible_rows.apply(
        lambda r: r["season"] == latest[r["league_id"]], axis=1
    )
    return (
        eligible_rows[~is_validation].copy(),
        eligible_rows[is_validation].copy(),
        latest,
        excluded,
    )


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


def fit(df_train, activity_features=None, quiet=False):
    requested = FEATURES + [RED_FEATURE] + list(activity_features or [])

    # Drop covariates that never vary. A column of constant zeros carries no
    # information but does make the design matrix rank-deficient, and with
    # almost no regularisation the solver is then free to put an arbitrarily
    # large coefficient in the null space. That is not hypothetical: including
    # dangerous_attacks_10m_total — which SportMonks publishes only as a
    # cumulative statistic and never on the timeline, so it is always zero
    # here — produced a coefficient of -8e11 and drove the fitted base rate to
    # zero, while the model still reported itself as alert-qualified.
    #
    # Dropped features keep their documented prior rather than a fitted value.
    # Constancy is tested with the peak-to-peak range rather than the standard
    # deviation. Summing 250k identical values accumulates floating-point
    # error, so a genuinely constant column can report a standard deviation
    # around 1e-16 — which passes a "> 0" test and lets the degenerate column
    # straight through. max minus min is exact.
    names, dropped = [], []
    for name in requested:
        column = df_train[name].to_numpy(dtype=float)
        if float(np.ptp(column)) > 0:
            names.append(name)
        else:
            dropped.append(name)
    if dropped and not quiet:
        print(f"  dropped constant features (kept at prior): {', '.join(dropped)}")
    if not names:
        raise RuntimeError("every covariate is constant; nothing to fit")

    X = df_train[names].to_numpy(dtype=float)
    y = df_train["goals_in_slice"].to_numpy(dtype=float)
    exposure = df_train["exposure"].to_numpy(dtype=float)

    mu = X.mean(axis=0)
    sd = X.std(axis=0)
    Xs = (X - mu) / sd

    # PoissonRegressor optimises a weighted mean of y/exposure, which is
    # equivalent to a log-link count model with log(exposure) as an offset.
    model = PoissonRegressor(alpha=1e-8, max_iter=5000, tol=1e-12)
    model.fit(Xs, y / exposure, sample_weight=exposure)
    if model.n_iter_ <= 1:
        raise RuntimeError(f"Poisson fit did not converge (n_iter={model.n_iter_})")

    raw_coef = model.coef_ / sd
    raw_intercept = model.intercept_ - float(np.sum(model.coef_ * mu / sd))

    # Guard against a numerically degenerate solution being shipped as a
    # result. These check the transformed values, which are what the artifact
    # actually carries — checking the standardised intercept instead would
    # miss exactly the blow-up this exists to catch. A real football base rate
    # is 2-3 goals per 90 minutes.
    base_per_90 = math.exp(raw_intercept) * REGULATION_SECONDS
    if not (0.5 < base_per_90 < 10.0):
        raise RuntimeError(
            f"degenerate fit: base rate {base_per_90:.4f} goals/90min is outside any plausible range"
        )
    if np.max(np.abs(raw_coef)) > 50:
        worst = names[int(np.argmax(np.abs(raw_coef)))]
        raise RuntimeError(
            f"degenerate fit: coefficient for {worst} is {np.max(np.abs(raw_coef)):.2e}, "
            f"which indicates a rank-deficient design"
        )

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
    mu_ft = lam * remaining
    out["two_ft"] = 1.0 - np.exp(-mu_ft) * (1.0 + mu_ft)
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


def coefficient_stability(df, activity_features, draws=8, seed=11):
    """Refit on random subsets and report how much each coefficient moves.

    A coefficient estimating a real effect stays put when the sample changes a
    little. One that reorders itself is following noise, and reporting it as a
    finding would dress up randomness as football.

    This is not hypothetical here: with overlapping shot counters removed, the
    training fit put a shot on target at twice the weight of one that missed
    (+0.0256 against +0.0126), while refitting on the full set reversed the
    order (+0.0134 against +0.0156). Adding a fifth more data should not change
    which of two effects is larger.
    """
    rng = np.random.default_rng(seed)
    names = FEATURES + [RED_FEATURE] + list(activity_features or [])
    fixtures = df["fixture_id"].unique()

    samples = {name: [] for name in names}
    for _ in range(draws):
        chosen = rng.choice(fixtures, size=int(len(fixtures) * 0.8), replace=False)
        subset = df[df["fixture_id"].isin(chosen)]
        try:
            model = fit(subset, activity_features, quiet=True)
        except RuntimeError:
            continue
        for name, value in model.coefficients.items():
            samples[name].append(value)

    report = {}
    for name, values in samples.items():
        if len(values) < 2:
            continue
        low, high = min(values), max(values)
        report[name] = {
            "min": float(low),
            "max": float(high),
            "sign_stable": bool((low > 0) == (high > 0)),
        }
    return report


def brier_advantage(fitted_probs, constant_probs, labels, fixture_ids, draws=2000, seed=7):
    """Is the model's Brier advantage over the constant rate real, or noise?

    Returns (mean_delta, low, high) where a negative delta means the model is
    better. Negative across the whole interval is the only result that counts
    as evidence.

    The comparison is bootstrapped over *matches*, not over slices. Every
    one-minute slice within a match shares that match's outcome, so treating
    slices as independent would shrink the interval by more than an order of
    magnitude and make a coin-flip difference look decisive. That matters
    concretely here: comparing the point estimates alone declared the model
    qualified on a Brier difference of 0.00006, which this test shows is
    indistinguishable from zero.
    """
    rng = np.random.default_rng(seed)
    per_slice = (fitted_probs - labels) ** 2 - (constant_probs - labels) ** 2

    unique_ids = np.unique(fixture_ids)
    per_match = np.array([per_slice[fixture_ids == fid].mean() for fid in unique_ids])
    if len(per_match) < 2:
        return float(per_match.mean()) if len(per_match) else 0.0, -math.inf, math.inf

    means = np.array([
        rng.choice(per_match, len(per_match), replace=True).mean() for _ in range(draws)
    ])
    low, high = np.percentile(means, [2.5, 97.5])
    return float(per_match.mean()), float(low), float(high)


def constant_rate_baseline(df_train, df_test):
    """The honest null model: one flat league-average intensity."""
    lam = df_train["goals_in_slice"].sum() / df_train["exposure"].sum()
    remaining = np.maximum(df_test["end_second"] - df_test["t"], 0.0).to_numpy(dtype=float)
    out = {}
    for key, window in (("5m", 300.0), ("10m", 600.0), ("ft", None)):
        w = remaining if window is None else np.minimum(window, remaining)
        out[key] = 1.0 - np.exp(-lam * w)
    mu_ft = lam * remaining
    out["two_ft"] = 1.0 - np.exp(-mu_ft) * (1.0 + mu_ft)
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

    activity_match_count = sum(1 for m in matches if m.get("activity_timeline_available"))
    fitted_activity = activity_match_count >= MIN_ACTIVITY_MATCHES
    modeling_matches = (
        [m for m in matches if m.get("activity_timeline_available")]
        if fitted_activity
        else matches
    )
    centers, centers_source = load_activity_centers()

    df = build_slices(modeling_matches)
    df = add_horizon_labels(df, modeling_matches)
    activity_features = list(ACTIVITY_PRIORS) if fitted_activity else []
    if fitted_activity:
        for feature in activity_features:
            df[feature] = df[feature] - centers[feature]
    print(f"built {len(df)} one-minute slices")

    df_train, df_validation, latest, excluded = training_validation_split(df)
    print(f"train {len(df_train)} slices / validation {len(df_validation)} slices")
    print(f"validation seasons: {latest}")
    print(f"excluded single-season leagues: {excluded}")

    model = fit(df_train, activity_features)
    coefs = model.coefficients
    base_goals_per_90 = math.exp(model.intercept) * REGULATION_SECONDS

    print()
    print(f"fitted base rate: {base_goals_per_90:.3f} goals/90min at reference state")
    for name, value in coefs.items():
        print(f"  {name:18s} {value:+.8f}")

    fitted_probs, lam = probabilities(model, df_validation)
    const_probs, const_lam = constant_rate_baseline(df_train, df_validation)

    report = ["# Hazard model v1 — training report", ""]
    report.append(f"- Matches: {len(matches)} ({df['fixture_id'].nunique()} distinct)")
    report.append(f"- Slices: {len(df)} ({len(df_train)} training / {len(df_validation)} validation)")
    report.append(f"- Validation seasons: {latest}")
    report.append(f"- Excluded single-season leagues: {excluded}")
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
    report.append("## Validation performance")
    report.append("")
    report.append("| horizon | model | Brier | LogLoss | ECE | mean pred | base rate |")
    report.append("|---|---|---|---|---|---|---|")

    print()
    print(f"{'horizon':8s} {'model':10s} {'Brier':>8s} {'LogLoss':>9s} {'ECE':>8s} {'pred':>7s} {'actual':>7s}")
    summary = {}
    for key, label_col in (
        ("5m", "label_5m"),
        ("10m", "label_10m"),
        ("ft", "label_ft"),
        ("two_ft", "label_two_ft"),
    ):
        labels = df_validation[label_col].to_numpy(dtype=float)
        m_fit = metrics(fitted_probs[key], labels)
        m_const = metrics(const_probs[key], labels)
        summary[key] = {"fitted": m_fit, "constant": m_const}
        for name, m in (("fitted", m_fit), ("constant", m_const)):
            print(f"{key:8s} {name:10s} {m['brier']:8.5f} {m['log_loss']:9.5f} {m['ece']:8.5f} {m['mean_pred']:7.4f} {m['base_rate']:7.4f}")
            report.append(
                f"| {key} | {name} | {m['brier']:.5f} | {m['log_loss']:.5f} | {m['ece']:.5f} | {m['mean_pred']:.4f} | {m['base_rate']:.4f} |"
            )

    report.append("")
    report.append("## Reliability — goal before full time (validation)")
    report.append("")
    report.append(reliability_table(fitted_probs["ft"], df_validation["label_ft"].to_numpy(dtype=float)))

    # Validation above is what the reported metrics describe. The artifact
    # itself is refitted on every season including the held-out one: holding
    # data back is how you measure honestly, not how you ship. Scoring rates
    # drift upward across these seasons (Brasileirao: 2.36 -> 2.53 -> 2.65
    # goals/90), so shipping a model blind to the most recent one would bake
    # in a known-stale base rate.
    final_model = fit(df, activity_features)
    final_coefs = final_model.coefficients
    final_base = math.exp(final_model.intercept) * REGULATION_SECONDS
    print()
    print(f"refit on all {len(df)} slices: base {final_base:.3f} goals/90min")

    report.append("")
    report.append("## Shipped artifact")
    report.append("")
    report.append(
        f"Validation used the newest already-inspected season; the exported artifact is refitted on all "
        f"{len(df)} slices from {len(matches)} matches. Base rate {final_base:.4f} goals/90 "
        f"at the reference state (kickoff, level, no red cards)."
    )
    report.append("")
    report.append("| term | coefficient |")
    report.append("|---|---|")
    for name, value in final_coefs.items():
        report.append(f"| {name} | {value:+.8f} |")

    validation_matches = int(df_validation["fixture_id"].nunique())
    # Qualification requires the advantage to be real, not merely numerically
    # smaller. A bare point comparison qualified this model on a Brier
    # difference of 0.00006 whose 95% interval ran from -0.0011 to +0.0008 —
    # noise, presented as an edge, one gate away from arming live alerts.
    fixture_ids = df_validation["fixture_id"].to_numpy()
    advantage = {}
    for key, label_col in (("ft", "label_ft"), ("two_ft", "label_two_ft")):
        labels = df_validation[label_col].to_numpy(dtype=float)
        advantage[key] = brier_advantage(
            fitted_probs[key], const_probs[key], labels, fixture_ids
        )

    one_qualified = advantage["ft"][2] < 0
    two_qualified = advantage["two_ft"][2] < 0

    stability = coefficient_stability(df_train, activity_features)
    unstable = [n for n, s in stability.items() if not s["sign_stable"]]
    print()
    print("estabilidade dos coeficientes (refit em 8 subconjuntos de 80%):")
    for name, s in stability.items():
        flag = "" if s["sign_stable"] else "  <- TROCA DE SINAL"
        print(f"  {name:28s} [{s['min']:+.5f}, {s['max']:+.5f}]{flag}")
    if unstable:
        print(f"  {len(unstable)} coeficiente(s) trocam de sinal — o efeito nao e distinguivel de ruido")

    print()
    print("vantagem sobre a taxa constante (bootstrap por partida, negativo = melhor):")
    for key in ("ft", "two_ft"):
        mean, low, high = advantage[key]
        verdict = "real" if high < 0 else "indistinguivel de ruido"
        print(f"  {key:7s} {mean:+.6f}  IC95 [{low:+.6f}, {high:+.6f}]  -> {verdict}")
    competition_rates = {}
    for league_id, rows in df.groupby("league_id"):
        exposure = float(rows["exposure"].sum())
        competition_rates[str(league_id)] = round(
            float(rows["goals_in_slice"].sum()) / exposure * REGULATION_SECONDS,
            6,
        )

    if fitted_activity:
        # A feature the fit dropped for having no variance keeps its documented
        # prior — reporting it as fitted would claim evidence that does not
        # exist. In practice this is xg_10m_total (unavailable on this plan)
        # and dangerous_attacks_10m_total (published only as a cumulative
        # statistic, never on the timeline).
        shipped_activity = {}
        held_at_prior = []
        for name in activity_features:
            if name in final_coefs:
                shipped_activity[name] = round(float(final_coefs[name]), 8)
            else:
                shipped_activity[name] = ACTIVITY_PRIORS[name]
                held_at_prior.append(name)
        activity_note = (
            f"Activity coefficients were fitted from timestamped timeline events in "
            f"{activity_match_count} matches."
        )
        if held_at_prior:
            activity_note += (
                f" These remain documented priors because they are constant in the "
                f"training data and cannot be fitted: {', '.join(sorted(held_at_prior))}."
            )
    else:
        shipped_activity = dict(ACTIVITY_PRIORS)
        activity_note = (
            f"Only {activity_match_count} matches contained a usable timestamped activity "
            f"timeline (minimum {MIN_ACTIVITY_MATCHES}); centered documented priors remain active."
        )

    artifact = {
        "modelVersion": "hazard_v1.3.0",
        "featureVersion": "v1.3.0",
        "modelType": "poisson_hazard",
        "baseGoalsPer90": round(final_base, 6),
        "competitionBaseGoalsPer90": competition_rates,
        "coefficients": {
            name: round(float(v), 8)
            for name, v in final_coefs.items()
            if name != RED_FEATURE and name not in activity_features
        },
        "redCardCoefficient": round(float(final_coefs[RED_FEATURE]), 8),
        "activityCoefficients": shipped_activity,
        "activityCenters": {k: round(float(v), 6) for k, v in centers.items()},
        "minMultiplier": 0.25,
        "maxMultiplier": 4.0,
        "trainedUntil": max(m["starting_at"] or "" for m in matches)[:10],
        "trainingMatches": len(modeling_matches),
        "validation": {
            "validationMatches": validation_matches,
            "holdoutMatches": validation_matches,
            "baselineGoalsPer90": round(float(const_lam) * REGULATION_SECONDS, 8),
            "oneGoalBrier": round(summary["ft"]["fitted"]["brier"], 8),
            "oneGoalBaselineBrier": round(summary["ft"]["constant"]["brier"], 8),
            "twoGoalBrier": round(summary["two_ft"]["fitted"]["brier"], 8),
            "twoGoalBaselineBrier": round(summary["two_ft"]["constant"]["brier"], 8),
            "oneGoalQualified": bool(one_qualified),
            "twoGoalQualified": bool(two_qualified),
            "coefficientStability": {
                name: {
                    "min": round(s["min"], 8),
                    "max": round(s["max"], 8),
                    # numpy returns its own bool type from a comparison, which the
            # json module refuses to encode.
            "signStable": bool(s["sign_stable"]),
                }
                for name, s in stability.items()
            },
            "brierAdvantage": {
                key: {
                    "meanDelta": round(advantage[key][0], 8),
                    "ci95Low": round(advantage[key][1], 8),
                    "ci95High": round(advantage[key][2], 8),
                    "clusteredByMatch": True,
                }
                for key in advantage
            },
        },
        "notes": (
            "Base rate and the match_second / abs_score_diff / goals_so_far / red-card terms are "
            "fitted by Poisson regression on {n} historical matches, validated on the already-inspected most-recent "
            "season per league. {activity_note} Activity values are centered so ordinary activity leaves "
            "the fitted intensity unchanged. xG remains zero when unavailable."
        ).format(n=len(modeling_matches), activity_note=activity_note),
    }

    serialized = json.dumps(
        {
            k: artifact[k]
            for k in (
                "baseGoalsPer90",
                "competitionBaseGoalsPer90",
                "coefficients",
                "redCardCoefficient",
                "activityCoefficients",
                "activityCenters",
                "validation",
            )
        },
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
    print(f"alert qualification: one_goal={one_qualified}, two_goal={two_qualified}")


if __name__ == "__main__":
    main()
