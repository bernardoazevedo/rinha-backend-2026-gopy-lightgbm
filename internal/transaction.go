package internal

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/wizenheimer/comet"
)

const vectorDimensions = 14
const recordSize = 1 + vectorDimensions*4 // 1 byte label + 14 x float32
const trainingSampleSize = 100_000

func LoadDataset(datasetPath string) (*VectorDatabase, error) {
	index, err := comet.NewIVFPQIndex(
		14,              // vector dimensions
		comet.Euclidean, // distance function
		1732,            // nClusters: number of partitions
		7,               // m: number of PQ subspaces
		8,               // nBits: bits per PQ subspace
	)
	if err != nil {
		return nil, fmt.Errorf("error creating ivfpq index: %s", err.Error())
	}

	f, err := os.Open(datasetPath)
	if err != nil {
		return nil, fmt.Errorf("error opening binary dataset: %s", err.Error())
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, 256*1024)

	// Read record count from header
	var count uint32
	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("error reading binary header: %s", err.Error())
	}

	log.Printf("dataset has %d vectors", count)

	// Determine training sample size
	trainSize := trainingSampleSize
	if int(count) < trainSize {
		trainSize = int(count)
	}

	// Read training sample
	trainNodes := make([]comet.VectorNode, 0, trainSize)
	trainLabels := make([]bool, 0, trainSize)
	recordBuf := make([]byte, recordSize)

	for i := 0; i < trainSize; i++ {
		if _, err := io.ReadFull(reader, recordBuf); err != nil {
			return nil, fmt.Errorf("error reading training record #%d: %s", i, err.Error())
		}

		vector := decodeVector(recordBuf)
		isLegitLabel := recordBuf[0] == 1

		node := comet.NewVectorNode(vector)
		trainNodes = append(trainNodes, *node)
		trainLabels = append(trainLabels, isLegitLabel)
	}

	log.Printf("training with %d vectors...", trainSize)
	if err := index.Train(trainNodes); err != nil {
		return nil, fmt.Errorf("error training index: %s", err.Error())
	}

	// Add training vectors to index and build label map
	isLegit := make([]bool, count+1)
	for i, node := range trainNodes {
		if err := index.Add(node); err != nil {
			return nil, fmt.Errorf("error adding training vector #%d: %s", i, err.Error())
		}
		id := node.ID()
		if int(id) >= len(isLegit) {
			newIsLegit := make([]bool, int(id)*2+1)
			copy(newIsLegit, isLegit)
			isLegit = newIsLegit
		}
		isLegit[id] = trainLabels[i]

		if (i+1)%1000 == 0 {
			log.Printf("trained %d/%d vectors", i+1, trainSize)
		}
	}

	// Free training slices
	trainNodes = nil
	trainLabels = nil

	// Stream remaining vectors one at a time
	remaining := int(count) - trainSize
	log.Printf("adding remaining %d vectors...", remaining)

	// CRITICAL MEMORY OPTIMIZATION:
	// Reuse a single slice for all remaining vectors to bypass Comet's VectorNode retaining the original arrays.
	// Since Comet's Add() only reads the vector for PQ encoding and we never read the original vector again,
	sharedVector := make([]float32, vectorDimensions)

	for i := 0; i < remaining; i++ {
		if _, err := io.ReadFull(reader, recordBuf); err != nil {
			return nil, fmt.Errorf("error reading record #%d: %s", trainSize+i, err.Error())
		}

		decodeVectorInto(recordBuf, sharedVector)
		isLegitLabel := recordBuf[0] == 1

		node := comet.NewVectorNode(sharedVector)
		id := node.ID()
		if int(id) >= len(isLegit) {
			newIsLegit := make([]bool, int(id)*2+1)
			copy(newIsLegit, isLegit)
			isLegit = newIsLegit
		}
		isLegit[id] = isLegitLabel
		if err := index.Add(*node); err != nil {
			return nil, fmt.Errorf("error adding vector #%d: %s", trainSize+i, err.Error())
		}

		if (i+1)%500000 == 0 {
			log.Printf("added %d/%d remaining vectors", i+1, remaining)
			
			// Shrink slice capacities inside Comet's inverted lists by calling Flush
			// Flush creates a new perfectly-sized slice and discards the bloated one
			log.Println("index.Flush()")
			index.Flush()

			// Force GC to free the old discarded slices and any slice growth overhead from inverted lists
			log.Println("runtime.GC()")
			runtime.GC()
		}
	}

	log.Printf("dataset loaded: %d vectors indexed", count)
	
	// Final shrink to ensure no capacity waste after the loop is over
	index.Flush()
	runtime.GC()
	
	return &VectorDatabase{index: index, isLegit: isLegit}, nil
}

// decodeVector extracts 14 float32 values from a binary record buffer (skipping the first label byte).
func decodeVector(recordBuf []byte) []float32 {
	vector := make([]float32, vectorDimensions)
	decodeVectorInto(recordBuf, vector)
	return vector
}

// decodeVectorInto extracts 14 float32 values from a binary record buffer directly into the provided slice.
func decodeVectorInto(recordBuf []byte, vector []float32) {
	for j := 0; j < vectorDimensions; j++ {
		bits := binary.LittleEndian.Uint32(recordBuf[1+j*4:])
		vector[j] = math.Float32frombits(bits)
	}
}

func (vd *VectorDatabase) VerifyVector(vector []float32) (bool, float32, error) {
	kResults := 5
	results, err := vd.index.NewSearch().
		WithQuery(vector).
		WithK(kResults).
		WithNProbes(8).
		Execute()
	if err != nil {
		return false, 0, fmt.Errorf("error searching index: %s", err.Error())
	}

	var fraudCount int
	for _, result := range results {
		id := result.GetId()
		if int(id) < len(vd.isLegit) && !vd.isLegit[id] {
			fraudCount++
		}
	}

	const threshold = 0.6

	fraudScore := float32(fraudCount) / float32(kResults)
	if fraudScore >= threshold {
		return false, fraudScore, nil
	}

	return true, fraudScore, nil
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
