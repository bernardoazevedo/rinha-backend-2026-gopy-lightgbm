#!/usr/bin/env python3
"""
train_lgbm.py — Treina um classificador binário LightGBM para detecção de fraude.

Entrada : resources/references.json  (array de {vector: float32[14], label: string})
Saída   : testdata/lgfraud.model     (formato texto LightGBM, carregável pelo leaves)

Uso:
    pip install lightgbm==3.3.5 numpy
    python train_lgbm.py
"""

import json
import sys
from pathlib import Path

import numpy as np

try:
    import lightgbm as lgb
except ImportError:
    print("Instale o lightgbm: pip install lightgbm numpy")
    sys.exit(1)

REFERENCES_PATH = Path("resources/references.json")
MODEL_OUTPUT    = Path("testdata/lgfraud.model")

FEATURE_NAMES = [
    "amount",
    "installments",
    "amount_vs_avg",
    "hour_of_day",
    "day_of_week",
    "minutes_since_last_tx",
    "km_from_last_tx",
    "km_from_home",
    "tx_count_24h",
    "is_online",
    "card_present",
    "unknown_merchant",
    "mcc_risk",
    "merchant_avg_amount",
]


def main() -> None:
    # ── 1. Carregar dados ───────────────────────────────────────────────────────
    print(f"Carregando {REFERENCES_PATH} ...", flush=True)
    with open(REFERENCES_PATH, encoding="utf-8") as f:
        data = json.load(f)

    print(f"  {len(data):,} registros carregados", flush=True)

    X = np.array([item["vector"] for item in data], dtype=np.float32)
    y = np.array([1 if item["label"] == "fraud" else 0 for item in data], dtype=np.int32)

    fraud_count = int(y.sum())
    legit_count = len(y) - fraud_count
    print(f"  fraude    : {fraud_count:,} ({fraud_count / len(y) * 100:.1f}%)")
    print(f"  legítimas : {legit_count:,} ({legit_count / len(y) * 100:.1f}%)")

    # ── 2. Dataset LightGBM ─────────────────────────────────────────────────────
    # Pontuação de -1 em minutesSinceLastTx e kmFromLastTx indica ausência de
    train_data = lgb.Dataset(X, label=y, feature_name=FEATURE_NAMES, free_raw_data=False)

    # Compensar desequilíbrio de classes
    scale_pos_weight = legit_count / fraud_count if fraud_count > 0 else 1.0

    params = {
        "objective":         "binary",
        "metric":            "binary_logloss",
        "boosting_type":     "gbdt",
        "num_leaves":        63,
        "max_depth":         -1,
        "learning_rate":     0.05,
        "n_estimators":      200,
        "feature_fraction":  0.9,
        "bagging_fraction":  0.8,
        "bagging_freq":      5,
        "min_child_samples": 20,
        "scale_pos_weight":  scale_pos_weight,
        "verbose":           -1,
    }

    # ── 3. Treinar ──────────────────────────────────────────────────────────────
    print("Treinando modelo LightGBM...", flush=True)
    callbacks = [lgb.log_evaluation(period=20)]

    model = lgb.train(
        params,
        train_data,
        num_boost_round=200,
        callbacks=callbacks,
    )

    # ── 4. Salvar ───────────────────────────────────────────────────────────────
    MODEL_OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    model.save_model(str(MODEL_OUTPUT))
    print(f"\nModelo salvo em {MODEL_OUTPUT}")

    # ── 5. Relatório de importância de features ─────────────────────────────────
    importance = model.feature_importance(importance_type="gain")
    pairs = sorted(zip(FEATURE_NAMES, importance), key=lambda x: x[1], reverse=True)
    print("\nImportância das features (gain):")
    for name, score in pairs:
        print(f"  {name:<25} {score:.2f}")


if __name__ == "__main__":
    main()
