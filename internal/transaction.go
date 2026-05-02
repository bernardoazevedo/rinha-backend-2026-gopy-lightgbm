package internal

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/coder/hnsw"
)

const vectorDimensions = 14

func LoadDataset(datasetPath string) (*VectorDatabase, error) {
	var err error
	graph := hnsw.NewGraph[int]()

	graph.M, err = strconv.Atoi(os.Getenv("GRAPH_M"))
	if err != nil {
		return nil, fmt.Errorf("error parsing GRAPH_M: %s", err.Error())
	}

	graph.Ml, err = strconv.ParseFloat(os.Getenv("GRAPH_ML"), 64)
	if err != nil {
		return nil, fmt.Errorf("error parsing GRAPH_ML: %s", err.Error())
	}

	savedGraph, err := os.ReadFile(os.Getenv("GRAPH_FILE_NAME"))
	if err != nil {
		return nil, fmt.Errorf("error reading graph file: %s", err.Error())
	}

	err = graph.Import(bytes.NewBuffer(savedGraph))
	if err != nil {
		return nil, fmt.Errorf("error exporting graph: %s", err.Error())
	}

	labelBytes, err := os.ReadFile(os.Getenv("LABEL_FILE_NAME"))
	if err != nil {
		return nil, fmt.Errorf("error reading label file: %s", err.Error())
	}

	var labelMap map[int]bool
	err = json.Unmarshal(labelBytes, &labelMap)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling label map: %s", err.Error())
	}

	return &VectorDatabase{
		graph:    graph,
		labelMap: labelMap,
	}, nil
}

// decodeVector extracts 14 float32 values from a binary record buffer (skipping the first label byte).
func decodeVector(recordBuf []byte) []float32 {
	vector := make([]float32, vectorDimensions)
	for j := 0; j < vectorDimensions; j++ {
		bits := binary.LittleEndian.Uint32(recordBuf[1+j*4:])
		vector[j] = math.Float32frombits(bits)
	}
	return vector
}

func (vd *VectorDatabase) VerifyVector(vector []float32) (bool, float32) {
	kResults := 5
	results := vd.graph.Search(vector, kResults)

	var fraudCount int
	for _, result := range results {
		if !vd.labelMap[result.Key] {
			fraudCount++
		}
	}

	const threshold = 0.6

	fraudScore := float32(fraudCount) / float32(kResults)
	if fraudScore >= threshold {
		return false, fraudScore
	}

	return true, fraudScore
}

func LoadDatasetAndVerifyVector(datasetPath string, vector []float32) (bool, float32, error) {
	vectorDatabase, err := LoadDataset(datasetPath)
	if err != nil {
		return false, 0, fmt.Errorf("error loading dataset: %s", err.Error())
	}

	approved, fraudScore := vectorDatabase.VerifyVector(vector)

	return approved, fraudScore, nil
}

func LoadReferenceVectors(path string) ([]TransactionVector, error) {
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
