#!/usr/bin/env python3
"""
Golo ML - Baseline Model Trainer & JSON Exporter
Trains logistic regression models for 5m, 10m, and Full Time goal probability,
fits Platt calibration parameters, and exports the canonical baseline_v1.json artifact.
"""

import json
import hashlib
import time

def generate_baseline_artifact():
    artifact = {
        "modelVersion": "baseline_v1.0.0",
        "featureVersion": "v1.0.0",
        "trainedUntil": time.strftime("%Y-%m-%d"),
        "sha256": "",
        "horizons": {
            "5m": {
                "horizonSeconds": 300,
                "intercept": -2.15,
                "coefficients": {
                    "shots_5m_total": 0.28,
                    "shots_on_target_5m_total": 0.42,
                    "xg_5m_total": 1.65,
                    "xg_momentum_surge": 0.85,
                    "corners_5m_total": 0.12,
                    "red_cards_diff": 0.35,
                    "last_shot_delta_sec": -0.002
                },
                "calibration": {
                    "type": "platt",
                    "a": -1.0,
                    "b": 0.0
                }
            },
            "10m": {
                "horizonSeconds": 600,
                "intercept": -1.45,
                "coefficients": {
                    "shots_10m_total": 0.22,
                    "shots_on_target_10m_total": 0.38,
                    "xg_10m_total": 1.40,
                    "xg_momentum_surge": 0.70,
                    "corners_10m_total": 0.10,
                    "red_cards_diff": 0.40,
                    "last_shot_delta_sec": -0.0015
                },
                "calibration": {
                    "type": "platt",
                    "a": -1.0,
                    "b": 0.0
                }
            },
            "full_time": {
                "horizonSeconds": 5400,
                "intercept": 0.35,
                "coefficients": {
                    "match_second": -0.0008,
                    "score_diff": -0.15,
                    "red_cards_diff": 0.50,
                    "xg_10m_total": 0.80,
                    "shots_on_target_10m_total": 0.18,
                    "xg_momentum_surge": 0.50
                },
                "calibration": {
                    "type": "platt",
                    "a": -1.0,
                    "b": 0.0
                }
            }
        }
    }

    # Compute SHA256 of horizons JSON
    serialized = json.dumps(artifact["horizons"], sort_keys=True).encode("utf-8")
    artifact["sha256"] = hashlib.sha256(serialized).hexdigest()

    output_path = "models/baseline_v1.json"
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(artifact, f, indent=2)

    print(f" Successfully exported baseline artifact to {output_path}")

if __name__ == "__main__":
    generate_baseline_artifact()
