# Hazard model v1 — training report

- Matches: 18596 (10886 distinct)
- Slices: 1041609 (966787 training / 74822 validation)
- Validation seasons: {2: '2026/2027', 5: '2026/2027', 271: '2026/2027', 648: '2026', 743: '2026/2027', 779: '2026', 1122: '2026', 1328: '2025/2026', 2286: '2026/2027'}
- Excluded single-season leagues: []
- Observed goals per match: 2.526
- Constant-rate baseline: 2.571 goals/90min
- Fitted base rate at reference state: 2.020 goals/90min

## Fitted coefficients

| term | coefficient |
|---|---|
| match_time_frac | +0.73972034 |
| match_time_frac_sq | -0.50553161 |
| abs_score_diff | +0.03197990 |
| goals_so_far | +0.01023603 |
| abs_red_diff | +0.08761947 |
| shots_off_target_10m_total | +0.00196279 |
| shots_on_target_10m_total | +0.01351643 |
| corners_10m_total | +0.02390408 |

## Validation performance

| horizon | model | Brier | LogLoss | ECE | mean pred | base rate |
|---|---|---|---|---|---|---|
| 5m | fitted | 0.11592 | 0.39281 | 0.00349 | 0.1313 | 0.1342 |
| 5m | constant | 0.11592 | 0.39283 | 0.00407 | 0.1305 | 0.1342 |
| 10m | fitted | 0.18403 | 0.55252 | 0.00984 | 0.2387 | 0.2464 |
| 10m | constant | 0.18404 | 0.55250 | 0.00894 | 0.2379 | 0.2464 |
| ft | fitted | 0.15096 | 0.46889 | 0.01818 | 0.6778 | 0.6952 |
| ft | constant | 0.15099 | 0.46719 | 0.02173 | 0.6739 | 0.6952 |
| two_ft | fitted | 0.17904 | 0.52555 | 0.02939 | 0.3913 | 0.4020 |
| two_ft | constant | 0.17816 | 0.52314 | 0.00754 | 0.4004 | 0.4020 |

## Reliability — goal before full time (validation)

| predicted | observed | n |
|---|---|---|
| 0.058 | 0.048 | 2231 |
| 0.151 | 0.150 | 2852 |
| 0.251 | 0.259 | 3240 |
| 0.351 | 0.369 | 3708 |
| 0.451 | 0.471 | 4332 |
| 0.552 | 0.560 | 5285 |
| 0.653 | 0.653 | 7088 |
| 0.755 | 0.771 | 11547 |
| 0.861 | 0.888 | 32735 |
| 0.907 | 0.929 | 1804 |

## Shipped artifact

Validation used the newest already-inspected season; the exported artifact is refitted on all 1041609 slices from 18596 matches. Base rate 2.0195 goals/90 at the reference state (kickoff, level, no red cards).

| term | coefficient |
|---|---|
| match_time_frac | +0.73426855 |
| match_time_frac_sq | -0.50007830 |
| abs_score_diff | +0.02681447 |
| goals_so_far | +0.01280236 |
| abs_red_diff | +0.07434327 |
| shots_off_target_10m_total | +0.00396381 |
| shots_on_target_10m_total | +0.00684322 |
| corners_10m_total | +0.02332933 |
