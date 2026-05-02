package internal

import (
	"encoding/json"
	"math"
	"testing"
)

func TestTransactionToVector(t *testing.T) {
	normalizationConstants, err := LoadNormalizationConstants("../resources/normalization.json")
	if err != nil {
		t.Fatalf("error LoadingNormalizationConstants err %v", err)
	}

	mccRiskMap, err := LoadMccRiskMap("../resources/mcc_risk.json")
	if err != nil {
		t.Fatalf("error LoadingMccRiskMap err %v", err)
	}

	transactionsJson := []string{
		`{
			"id": "tx-1329056812",
			"transaction": {"amount": 41.12, "installments": 2, "requested_at": "2026-03-11T18:45:53Z"},
			"customer": {"avg_amount": 82.24, "tx_count_24h": 3, "known_merchants": ["MERC-003", "MERC-016"]},
			"merchant": {"id": "MERC-016", "mcc": "5411", "avg_amount": 60.25},
			"terminal": {"is_online": false, "card_present": true, "km_from_home": 29.23},
			"last_transaction": null
		}`,
		`{
			"id": "tx-3330991687",
			"transaction":      { "amount": 9505.97, "installments": 10, "requested_at": "2026-03-14T05:15:12Z" },
			"customer":         { "avg_amount": 81.28, "tx_count_24h": 20, "known_merchants": ["MERC-008", "MERC-007", "MERC-005"] },
			"merchant":         { "id": "MERC-068", "mcc": "7802", "avg_amount": 54.86 },
			"terminal":         { "is_online": false, "card_present": true, "km_from_home": 952.27 },
			"last_transaction": null
		}`,
	}
	var transactions []Transaction
	for _, transactionJson := range transactionsJson {
		var transaction Transaction
		err = json.Unmarshal([]byte(transactionJson), &transaction)
		if err != nil {
			t.Fatalf("error unmarshalling transactionJson err %v", err)
		}
		transactions = append(transactions, transaction)
	}

	expected := map[int][]float32{
		0: {
			0.0041, // 0  - amount
			0.1667, // 1  - installments
			0.05,   // 2  - amount_vs_avg
			0.7826, // 3  - hour_of_day
			0.3333, // 4  - day_of_week
			-1,     // 5  - minutes_since_last_tx
			-1,     // 6  - km_from_last_tx
			0.0292, // 7  - km_from_home
			0.15,   // 8  - tx_count_24h
			0,      // 9  - is_online
			1,      // 10 - card_present
			0,      // 11 - unknown_merchant
			0.15,   // 12 - mcc_risk
			0.006,  // 13 - merchant_avg_amount
		},
		1: {
			0.9506, // 0  - amount
			0.8333, // 1  - installments
			1.0,    // 2  - amount_vs_avg
			0.2174, // 3  - hour_of_day
			0.8333, // 4  - day_of_week
			-1,     // 5  - minutes_since_last_tx
			-1,     // 6  - km_from_last_tx
			0.9523, // 7  - km_from_home
			1.0,    // 8  - tx_count_24h
			0,      // 9  - is_online
			1,      // 10 - card_present
			1,      // 11 - unknown_merchant
			0.75,   // 12 - mcc_risk
			0.0055, // 13 - merchant_avg_amount
		},
	}
	expectedKeys := []string{
		"amount",                // 0  - amount
		"installments",          // 1  - installments
		"amount_vs_avg",         // 2  - amount_vs_avg
		"hour_of_day",           // 3  - hour_of_day
		"day_of_week",           // 4  - day_of_week
		"minutes_since_last_tx", // 5  - minutes_since_last_tx
		"km_from_last_tx",       // 6  - km_from_last_tx
		"km_from_home",          // 7  - km_from_home
		"tx_count_24h",          // 8  - tx_count_24h
		"is_online",             // 9  - is_online
		"card_present",          // 10 - card_present
		"unknown_merchant",      // 11 - unknown_merchant
		"mcc_risk",              // 12 - mcc_risk
		"merchant_avg_amount",   // 13 - merchant_avg_amount
	}

	const epsilon = 1e-4
	for txIndex, eachTx := range transactions {
		vector, err := TransactionToVector(eachTx, normalizationConstants, mccRiskMap)
		if err != nil {
			t.Fatalf("TransactionToVector returned error: %v", err)
		}

		if len(vector) != len(expected[txIndex]) {
			t.Fatalf("transaction %d vector size: got %d, expected %d", txIndex, len(vector), len(expected[txIndex]))
		}

		for keysIndex := range expectedKeys {
			if math.Abs(float64(vector[keysIndex]-expected[txIndex][keysIndex])) > epsilon {
				t.Errorf("transaction %d (%d) %s: got %f, expected %f", txIndex, keysIndex, expectedKeys[keysIndex], vector[keysIndex], expected[txIndex][keysIndex])
			}
		}
	}
}

func TestLoadDatasetAndVerifyVector(t *testing.T) {
	vectors := map[bool][]float32{
		true: {
			0.0041, // 0  - amount
			0.1667, // 1  - installments
			0.05,   // 2  - amount_vs_avg
			0.7826, // 3  - hour_of_day
			0.3333, // 4  - day_of_week
			-1,     // 5  - minutes_since_last_tx
			-1,     // 6  - km_from_last_tx
			0.0292, // 7  - km_from_home
			0.15,   // 8  - tx_count_24h
			0,      // 9  - is_online
			1,      // 10 - card_present
			0,      // 11 - unknown_merchant
			0.15,   // 12 - mcc_risk
			0.006,  // 13 - merchant_avg_amount
		},
		false: {
			0.9506, // 0  - amount
			0.8333, // 1  - installments
			1.0,    // 2  - amount_vs_avg
			0.2174, // 3  - hour_of_day
			0.8333, // 4  - day_of_week
			-1,     // 5  - minutes_since_last_tx
			-1,     // 6  - km_from_last_tx
			0.9523, // 7  - km_from_home
			1.0,    // 8  - tx_count_24h
			0,      // 9  - is_online
			1,      // 10 - card_present
			1,      // 11 - unknown_merchant
			0.75,   // 12 - mcc_risk
			0.0055, // 13 - merchant_avg_amount
		},
	}

	for expectedApproved, vector := range vectors {
		approved, _, err := LoadDatasetAndVerifyVector("../resources/references-lite.json", vector)
		if err != nil {
			t.Fatalf("LoadDatasetAndVerifyVector returned error: %v", err)
		}
		if approved != expectedApproved {
			t.Errorf("vector %v: got label %v, expected %v", vector, approved, expectedApproved)
		}
	}
}
