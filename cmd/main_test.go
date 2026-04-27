package main

import (
	"encoding/json"
	"testing"

	"github.com/bernardoazevedo/rinha-de-backend-2026/internal"
)

func TestTransactionToVector(t *testing.T) {
	normalizationConstants, err := loadNormalizationConstants("../resources/normalization.json")
	if err != nil {
		t.Fatalf("error loading normalizationConstants err %v", err)
	}

	mccRiskMap, err := loadMccRiskMap("../resources/mcc_risk.json")
	if err != nil {
		t.Fatalf("error loading mccRiskMap err %v", err)
	}

	transactionJson := `{
      "id": "tx-1329056812",
		"transaction": {"amount": 41.12, "installments": 2, "requested_at": "2026-03-11T18:45:53Z"},
		"customer": {"avg_amount": 82.24, "tx_count_24h": 3, "known_merchants": ["MERC-003", "MERC-016"]},
		"merchant": {"id": "MERC-016", "mcc": "5411", "avg_amount": 60.25},
		"terminal": {"is_online": false, "card_present": true, "km_from_home": 29.23},
		"last_transaction": null
	}`
	var transaction internal.Transaction
	err = json.Unmarshal([]byte(transactionJson), &transaction)
	if err != nil {
		t.Fatalf("error unmarshalling transactionJson err %v", err)
	}

	expected := []float32{
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

	vector, err := transactionToVector(transaction, normalizationConstants, mccRiskMap)
	if err != nil {
		t.Fatalf("transactionToVector returned error: %v", err)
	}

	if len(vector) != len(expected) {
		t.Fatalf("vector size: got %d, expected %d", len(vector), len(expected))
	}

	// const epsilon = 1e-4
	// for i := range expected {
	// 	if math.Abs(float64(vector[i]-expected[i])) > epsilon {
	// 		// t.Errorf("index %d: got %s, expected %s",
	// 		// 	i,
	// 		// 	fmt.Sprintf("%.6f", vector[i]),
	// 		// 	fmt.Sprintf("%.6f", expected[i]),
	// 		// )
	// 		if vector[i] != expected[i] {
	// 			t.Errorf("(%d) %s: got %f, expected %f", i, expectedKeys[i], vector[i], expected[i])
	// 		}
	// 	}
	// }
	for index := range expectedKeys {
		if vector[index] != expected[index] {
			t.Errorf("(%d) %s: got %f, expected %f", index, expectedKeys[index], vector[index], expected[index])
		}
	}
}
