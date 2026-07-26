# Hazard model v1 — training report

- Matches: 3282 (3282 distinct)
- Slices: 327238 (268924 training / 58314 validation)
- Validation seasons: {648: '2026', 743: '2026/2027', 779: '2026', 1122: '2026'}
- Excluded single-season leagues: []
- Observed goals per match: 2.734
- Constant-rate baseline: 2.459 goals/90min
- Fitted base rate at reference state: 1.900 goals/90min

## Fitted coefficients

| term | coefficient |
|---|---|
| match_time_frac | +0.78304052 |
| match_time_frac_sq | -0.51112134 |
| abs_score_diff | +0.04867069 |
| goals_so_far | -0.00629641 |
| abs_red_diff | +0.02217672 |

## Validation performance

| horizon | model | Brier | LogLoss | ECE | mean pred | base rate |
|---|---|---|---|---|---|---|
| 5m | fitted | 0.11573 | 0.39250 | 0.00914 | 0.1252 | 0.1338 |
| 5m | constant | 0.11573 | 0.39242 | 0.00896 | 0.1252 | 0.1338 |
| 10m | fitted | 0.18370 | 0.55190 | 0.01737 | 0.2286 | 0.2452 |
| 10m | constant | 0.18357 | 0.55147 | 0.01664 | 0.2289 | 0.2452 |
| ft | fitted | 0.15130 | 0.47003 | 0.02528 | 0.6665 | 0.6912 |
| ft | constant | 0.15104 | 0.46728 | 0.02709 | 0.6645 | 0.6912 |
| two_ft | fitted | 0.18083 | 0.52932 | 0.03152 | 0.3763 | 0.4044 |
| two_ft | constant | 0.17895 | 0.52480 | 0.01740 | 0.3875 | 0.4044 |

## Reliability — goal before full time (validation)

| predicted | observed | n |
|---|---|---|
| 0.055 | 0.046 | 1758 |
| 0.148 | 0.149 | 2419 |
| 0.251 | 0.253 | 2644 |
| 0.351 | 0.373 | 2958 |
| 0.451 | 0.471 | 3449 |
| 0.551 | 0.571 | 4332 |
| 0.652 | 0.666 | 5788 |
| 0.755 | 0.777 | 9528 |
| 0.856 | 0.893 | 25257 |
| 0.903 | 1.000 | 181 |

## Shipped artifact

Validation used the newest already-inspected season; the exported artifact is refitted on all 327238 slices from 3282 matches. Base rate 1.9182 goals/90 at the reference state (kickoff, level, no red cards).

| term | coefficient |
|---|---|
| match_time_frac | +0.77584455 |
| match_time_frac_sq | -0.50498987 |
| abs_score_diff | +0.02882643 |
| goals_so_far | +0.00276526 |
| abs_red_diff | +0.00422537 |
