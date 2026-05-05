package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dmitryikh/leaves"
)

const fraudThreshold = 0.6

func LoadDataset(datasetPath string) (*Model, error) {
	log.Printf("Carregando modelo: %s", datasetPath)
	model, err := leaves.LGEnsembleFromFile(datasetPath, true)
	if err != nil {
		log.Fatalf("Erro ao carregar modelo: %v\nGere o modelo primeiro: python train_lgbm.py", err)
	}
	log.Printf("Modelo carregado: %d estimadores, %d grupo(s) de saída\n", model.NEstimators(), model.NOutputGroups())
	return &Model{model: model}, nil
}

func (model *Model) VerifyVector(vector []float32) (bool, float32, error) {
	vec64 := make([]float64, len(vector))
	for i, v := range vector {
		vec64[i] = float64(v)
	}

	predictions := make([]float64, model.model.NOutputGroups())
	if err := model.model.Predict(vec64, 0, predictions); err != nil {
		return false, 0, fmt.Errorf("error predicting: %s", err.Error())
	}

	fraudScore := predictions[0]
	approved := fraudScore < fraudThreshold

	return approved, float32(fraudScore), nil
}

func LoadDatasetAndVerifyVector(datasetPath string, vector []float32) (bool, float32, error) {
	vectorDatabase, err := LoadDataset(datasetPath)
	if err != nil {
		return false, 0, fmt.Errorf("error loading dataset: %s", err.Error())
	}

	approved, fraudScore, err := vectorDatabase.VerifyVector(vector)
	if err != nil {
		return false, 0, fmt.Errorf("error verifying vector: %s", err.Error())
	}

	return approved, fraudScore, nil
}

func loadReferenceVectors(path string) ([]TransactionVector, error) {
	var vectors []TransactionVector

	inputData, err := os.ReadFile(path)
	if err != nil {
		return vectors, fmt.Errorf("error reading reference vectors: %s", err.Error())
	}

	err = json.Unmarshal(inputData, &vectors)
	if err != nil {
		return vectors, fmt.Errorf("error unmarshalling reference vectors: %s", err.Error())
	}

	return vectors, nil
}

func TransactionToVector(transaction Transaction, normalizationConstants NormalizationConstants, mccRiskMap map[string]float32) ([]float32, error) {
	requestedAt, err := time.Parse(time.RFC3339Nano, transaction.Transaction.RequestedAt)
	if err != nil {
		return nil, fmt.Errorf("error parsing requestedAt [%s] err %s", transaction.Transaction.RequestedAt, err.Error())
	}

	var lastTransactionTime time.Time
	var minutesSinceLastTx float32
	var kmFromLastTx float32
	if transaction.LastTransaction != nil {
		lastTransactionTime, err = time.Parse(time.RFC3339Nano, transaction.LastTransaction.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("error parsing lastTransaction.timestamp [%s] err %s", transaction.LastTransaction.Timestamp, err.Error())
		}
		minutesSinceLastTx = clampFloat32(float32(requestedAt.Sub(lastTransactionTime).Minutes()) / normalizationConstants.MaxMinutes)

		kmFromLastTx = clampFloat32(transaction.LastTransaction.KmFromCurrent / normalizationConstants.MaxKm)
	} else {
		minutesSinceLastTx = -1
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

func LoadNormalizationConstants(path string) (NormalizationConstants, error) {
	var normalizationConstants NormalizationConstants

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

func LoadMccRiskMap(path string) (map[string]float32, error) {
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

func loadExampleTransactions(path string) ([]Transaction, error) {
	var transactions []Transaction

	inputData, err := os.ReadFile(path)
	if err != nil {
		return transactions, fmt.Errorf("error reading example transactions: %s", err.Error())
	}

	err = json.Unmarshal(inputData, &transactions)
	if err != nil {
		return transactions, fmt.Errorf("error unmarshalling example transactions: %s", err.Error())
	}

	return transactions, nil
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
