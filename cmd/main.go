package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/bernardoazevedo/rinha-de-backend-2026/internal"
)

func main() {
	exampleJson := `{
		"id": "tx-1329056812",
		"transaction": {
			"amount": 41.12,
			"installments": 2,
			"requested_at": "2026-03-11T18:45:53Z"
		},
		"customer": {
			"avg_amount": 82.24,
			"tx_count_24h": 3,
			"known_merchants": ["MERC-003", "MERC-016"]
		},
		"merchant": { "id": "MERC-016", "mcc": "5411", "avg_amount": 60.25 },
		"terminal": {
			"is_online": false,
			"card_present": true,
			"km_from_home": 29.23
		},
		"last_transaction": null
	}`

	normalizationConstants, err := loadNormalizationConstants("./resources/normalization.json")
	if err != nil {
		panic(err)
	}

	mccRiskMap, err := loadMccRiskMap("./resources/mcc_risk.json")
	if err != nil {
		panic(err)
	}

	var transaction internal.Transaction
	err = json.Unmarshal([]byte(exampleJson), &transaction)
	if err != nil {
		panic(err)
	}

	vector, err := transactionToVector(transaction, normalizationConstants, mccRiskMap)
	if err != nil {
		panic(err)
	}

	vectorJson, err := json.MarshalIndent(vector, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(vectorJson))

	// graph := hnsw.NewGraph[string]()

	// var transactions []internal.Transaction
	// inputData, err := os.ReadFile("resources/example-payloads.json")
	// if err != nil {
	// 	panic(err)
	// }

	// err = json.Unmarshal(inputData, &transactions)
	// if err != nil {
	// 	panic(err)
	// }

	// for i, transaction := range transactions {
	// 	// graph.Add(hnsw.MakeNode(transaction.ID, transaction)))
	// 	// transactionJson, err := json.MarshalIndent(transaction, "", "  ")
	// 	// if err != nil {
	// 	// 	println("error parsing back transaction [", i, "] from [", transaction.ID, "] error ", err.Error())
	// 	// 	continue
	// 	// }
	// 	// println(string(transactionJson))
	// 	println(i+1, transaction.ID)
	// }
}

// ### As 14 dimensões do vetor
//
// As transações ([exemplos realistas aqui](/resources/example-payloads.json)) precisam ser transformadas em vetores de
// 14 posições, seguindo a ordem e as regras de normalização abaixo.
//
// | índice | dimensão                 | fórmula                                                                          |
// |-----|--------------------------|----------------------------------------------------------------------------------|
// | 0   | `amount`                 | `limitar(transaction.amount / max_amount)`                                         |
// | 1   | `installments`           | `limitar(transaction.installments / max_installments)`                             |
// | 2   | `amount_vs_avg`          | `limitar((transaction.amount / customer.avg_amount) / amount_vs_avg_ratio)`        |
// | 3   | `hour_of_day`            | `hora(transaction.requested_at) / 23`  (0-23, UTC)                               |
// | 4   | `day_of_week`            | `dia_da_semana(transaction.requested_at) / 6`    (seg=0, dom=6)                  |
// | 5   | `minutes_since_last_tx`  | `limitar(minutos / max_minutes)` ou `-1` se `last_transaction: null`             |
// | 6   | `km_from_last_tx`        | `limitar(last_transaction.km_from_current / max_km)` ou `-1` se `last_transaction: null` |
// | 7   | `km_from_home`           | `limitar(terminal.km_from_home / max_km)`                                          |
// | 8   | `tx_count_24h`           | `limitar(customer.tx_count_24h / max_tx_count_24h)`                                |
// | 9   | `is_online`              | `1` se `terminal.is_online`, senão `0`                                           |
// | 10  | `card_present`           | `1` se `terminal.card_present`, senão `0`                                        |
// | 11  | `unknown_merchant`       | `1` se `merchant.id` não estiver em `customer.known_merchants`, senão `0` (invertido: `1` = desconhecido) |
// | 12  | `mcc_risk`               | `mcc_risk.json[merchant.mcc]` (valor padrão `0.5`)                               |
// | 13  | `merchant_avg_amount`    | `limitar(merchant.avg_amount / max_merchant_avg_amount)`                           |
func transactionToVector(transaction internal.Transaction, normalizationConstants internal.NormalizationConstants, mccRiskMap map[string]float32) ([]float32, error) {
	requestedAt, err := time.Parse(time.RFC3339Nano, transaction.Transaction.RequestedAt)
	if err != nil {
		return nil, fmt.Errorf("error parsing requestedAt [%s] err %s", transaction.Transaction.RequestedAt, err.Error())
	}

	var lastTransactionTime time.Time
	var minutesSinceLastTx float32
	if transaction.LastTransaction != nil {
		lastTransactionTime, err = time.Parse(time.RFC3339Nano, transaction.LastTransaction.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("error parsing lastTransaction.timestamp [%s] err %s", transaction.LastTransaction.Timestamp, err.Error())
		}
		minutesSinceLastTx = clampFloat32(float32(requestedAt.Sub(lastTransactionTime).Minutes() / float64(normalizationConstants.MaxMinutes)))
	} else {
		minutesSinceLastTx = -1
	}

	var kmFromLastTx float32
	if transaction.LastTransaction != nil {
		kmFromLastTx = clampFloat32(transaction.LastTransaction.KmFromCurrent / normalizationConstants.MaxKm)
	} else {
		kmFromLastTx = -1
	}

	var unknown_merchant bool = true
	for _, knownMerchant := range transaction.Customer.KnownMerchants {
		if knownMerchant == transaction.Merchant.ID {
			unknown_merchant = false
			break
		}
	}

	mccRisk, ok := mccRiskMap[transaction.Merchant.MCC]
	if !ok {
		mccRisk = 0.5
	}

	return []float32{
		clampFloat32(float32(transaction.Transaction.Amount / normalizationConstants.MaxAmount)),                                           // 0  - `amount`
		clampFloat32(float32(transaction.Transaction.Installments / normalizationConstants.MaxInstallments)),                               // 1  - `installments`
		clampFloat32(float32((transaction.Transaction.Amount / transaction.Customer.AvgAmount) / normalizationConstants.AmountVsAvgRatio)), // 2  - `amount_vs_avg`
		float32(float32(requestedAt.UTC().Hour()) / 23.0),                                                                                  // 3  - `hour_of_day`
		float32(weekdayToRinhaDay(requestedAt.UTC().Weekday()) / 6.0),                                                                      // 4  - `day_of_week`
		float32(minutesSinceLastTx), // 5  - `minutes_since_last_tx`
		float32(kmFromLastTx),       // 6  - `km_from_last_tx`
		clampFloat32(transaction.Terminal.KmFromHome / normalizationConstants.MaxKm),                        // 7  - `km_from_home`
		clampFloat32(transaction.Customer.TxCount24h / normalizationConstants.MaxTxCount24h),                // 8  - `tx_count_24h`
		float32(boolToFloat32(transaction.Terminal.IsOnline)),                                               // 9  - `is_online`
		float32(boolToFloat32(transaction.Terminal.CardPresent)),                                            // 10 - `card_present`
		float32(boolToFloat32(unknown_merchant)),                                                            // 11 - `unknown_merchant`
		float32(mccRisk),                                                                                    // 12 - `mcc_risk`
		clampFloat32(float32(transaction.Merchant.AvgAmount / normalizationConstants.MaxMerchantAvgAmount)), // 13 - `merchant_avg_amount`
	}, nil

}

func loadNormalizationConstants(path string) (internal.NormalizationConstants, error) {
	var normalizationConstants internal.NormalizationConstants

	inputData, err := os.ReadFile(path)
	if err != nil {
		return normalizationConstants, fmt.Errorf("error reading normalization constants: %s", err.Error())
	}

	err = json.Unmarshal(inputData, &normalizationConstants)
	if err != nil {
		return normalizationConstants, fmt.Errorf("error unmarshalling normalization constants: %s", err.Error())
	}

	return normalizationConstants, nil
}

func loadMccRiskMap(path string) (map[string]float32, error) {
	var mccRiskMap map[string]float32

	inputData, err := os.ReadFile(path)
	if err != nil {
		return mccRiskMap, fmt.Errorf("error reading mcc risk map: %s", err.Error())
	}

	err = json.Unmarshal(inputData, &mccRiskMap)
	if err != nil {
		return mccRiskMap, fmt.Errorf("error unmarshalling mcc risk map: %s", err.Error())
	}

	return mccRiskMap, nil
}

func boolToFloat32(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func clampFloat32(value float32) float32 {
	const MIN = 0
	const MAX = 1
	if value < MIN {
		return MIN
	}
	if value > MAX {
		return MAX
	}
	return value
}

func weekdayToRinhaDay(weekday time.Weekday) float32 {
	switch weekday {
	case time.Monday:
		return 0
	case time.Tuesday:
		return 1
	case time.Wednesday:
		return 2
	case time.Thursday:
		return 3
	case time.Friday:
		return 4
	case time.Saturday:
		return 5
	case time.Sunday:
		return 6
	default:
		return 0
	}
}
