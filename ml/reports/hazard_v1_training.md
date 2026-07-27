# Hazard model v1 — training report

- Matches: 3287 (3177 distinct)
- Slices: 316583 (257762 training / 58821 validation)
- Validation seasons: {648: '2026', 743: '2026/2027', 779: '2026', 1122: '2026'}
- Excluded single-season leagues: []
- Observed goals per match: 2.734
- Constant-rate baseline: 2.473 goals/90min
- Fitted base rate at reference state: 1.949 goals/90min

## Fitted coefficients

| term | coefficient |
|---|---|
| match_time_frac | +0.67472328 |
| match_time_frac_sq | -0.41032089 |
| abs_score_diff | +0.04717964 |
| goals_so_far | -0.00778669 |
| abs_red_diff | +0.00712882 |
| shots_off_target_10m_total | +0.01012100 |
| shots_on_target_10m_total | +0.02200916 |
| corners_10m_total | -0.00102759 |

## Validation performance

| horizon | model | Brier | LogLoss | ECE | mean pred | base rate |
|---|---|---|---|---|---|---|
| 5m | fitted | 0.11574 | 0.39246 | 0.00886 | 0.1261 | 0.1338 |
| 5m | constant | 0.11574 | 0.39243 | 0.00837 | 0.1259 | 0.1338 |
| 10m | fitted | 0.18365 | 0.55171 | 0.01599 | 0.2300 | 0.2454 |
| 10m | constant | 0.18361 | 0.55152 | 0.01573 | 0.2301 | 0.2454 |
| ft | fitted | 0.15036 | 0.46767 | 0.02513 | 0.6683 | 0.6920 |
| ft | constant | 0.15042 | 0.46573 | 0.02627 | 0.6661 | 0.6920 |
| two_ft | fitted | 0.18012 | 0.52766 | 0.03106 | 0.3785 | 0.4041 |
| two_ft | constant | 0.17860 | 0.52394 | 0.01510 | 0.3896 | 0.4041 |

## Reliability — goal before full time (validation)

| predicted | observed | n |
|---|---|---|
| 0.057 | 0.046 | 1755 |
| 0.150 | 0.139 | 2372 |
| 0.251 | 0.251 | 2598 |
| 0.351 | 0.366 | 3050 |
| 0.452 | 0.466 | 3512 |
| 0.552 | 0.569 | 4360 |
| 0.653 | 0.669 | 5862 |
| 0.755 | 0.775 | 9637 |
| 0.857 | 0.895 | 25109 |
| 0.906 | 0.945 | 566 |

## Shipped artifact

Validation used the newest already-inspected season; the exported artifact is refitted on all 316583 slices from 3287 matches. Base rate 1.9640 goals/90 at the reference state (kickoff, level, no red cards).

| term | coefficient |
|---|---|
| match_time_frac | +0.67683102 |
| match_time_frac_sq | -0.41846959 |
| abs_score_diff | +0.02760680 |
| goals_so_far | +0.00284950 |
| abs_red_diff | -0.00901633 |
| shots_off_target_10m_total | +0.01349101 |
| shots_on_target_10m_total | +0.01036422 |
| corners_10m_total | +0.00153252 |
