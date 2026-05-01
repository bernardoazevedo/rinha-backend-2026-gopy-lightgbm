package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	sqlite3 "github.com/mattn/go-sqlite3"
)

func LoadFileToMemory(dbPath string) (*sql.DB, error) {
	fileDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("error opening file database: %w", err)
	}
	defer fileDB.Close()

	memDB, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		return nil, fmt.Errorf("error opening memory database: %w", err)
	}
	_, err = memDB.Exec("PRAGMA journal_mode=OFF;")
	if err != nil {
		log.Fatal(err)
	}
	_, err = memDB.Exec("PRAGMA synchronous=OFF;")
	if err != nil {
		log.Fatal(err)
	}
	_, err = memDB.Exec("PRAGMA cache_size=-100;")
	if err != nil {
		log.Fatal(err)
	}

	fileConn, err := fileDB.Conn(context.Background())
	if err != nil {
		memDB.Close()
		return nil, fmt.Errorf("error getting file connection: %w", err)
	}
	defer fileConn.Close()

	memConn, err := memDB.Conn(context.Background())
	if err != nil {
		memDB.Close()
		return nil, fmt.Errorf("error getting memory connection: %w", err)
	}
	defer memConn.Close()

	err = memConn.Raw(func(memDC interface{}) error {
		return fileConn.Raw(func(fileDC interface{}) error {
			memSQLiteConn, ok := memDC.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("memory connection is not *sqlite3.SQLiteConn")
			}
			fileSQLiteConn, ok := fileDC.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("file connection is not *sqlite3.SQLiteConn")
			}

			backup, err := memSQLiteConn.Backup("main", fileSQLiteConn, "main")
			if err != nil {
				return fmt.Errorf("error creating backup: %w", err)
			}

			_, err = backup.Step(-1)
			if err != nil {
				return fmt.Errorf("error during backup step: %w", err)
			}

			return backup.Finish()
		})
	})
	if err != nil {
		memDB.Close()
		return nil, fmt.Errorf("error during backup: %w", err)
	}

	return memDB, nil
}

func LoadMemoryToFile(memDB *sql.DB, filePath string) (*sql.DB, error) {
	os.Remove(filePath)

	fileDB, err := sql.Open("sqlite3", filePath)
	if err != nil {
		return nil, fmt.Errorf("error opening file database: %w", err)
	}
	defer fileDB.Close()

	fileConn, err := fileDB.Conn(context.Background())
	if err != nil {
		memDB.Close()
		return nil, fmt.Errorf("error getting file connection: %w", err)
	}
	defer fileConn.Close()

	memConn, err := memDB.Conn(context.Background())
	if err != nil {
		memDB.Close()
		return nil, fmt.Errorf("error getting memory connection: %w", err)
	}
	defer memConn.Close()

	err = fileConn.Raw(func(fileDC interface{}) error {
		return memConn.Raw(func(memDC interface{}) error {
			fileSQLiteConn, ok := fileDC.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("file connection is not *sqlite3.SQLiteConn")
			}
			memSQLiteConn, ok := memDC.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("memory connection is not *sqlite3.SQLiteConn")
			}

			backup, err := fileSQLiteConn.Backup("main", memSQLiteConn, "main")
			if err != nil {
				return fmt.Errorf("error creating backup: %w", err)
			}

			_, err = backup.Step(-1)
			if err != nil {
				return fmt.Errorf("error during backup step: %w", err)
			}

			return backup.Finish()
		})
	})
	if err != nil {
		fileDB.Close()
		return nil, fmt.Errorf("error during backup: %w", err)
	}

	return fileDB, nil
}

func Query(db *sql.DB, vectorQuery []float32) (bool, float32, error) {
	query, err := sqlite_vec.SerializeFloat32(vectorQuery)
	if err != nil {
		return false, 0, fmt.Errorf("error serializing vector: %s", err)
	}
	nResults := 5

	selectStart := time.Now()
	rows, err := db.Query(`
		SELECT
			distance,
			legit
		FROM vec_items
		WHERE embedding MATCH ?
		AND k = ?
	`, query, nResults)

	if err != nil {
		log.Fatal(err)
	}

	var fraudCount int
	for rows.Next() {
		var distance float64
		var legit bool

		err = rows.Scan(&distance, &legit)
		if err != nil {
			log.Fatal(err)
		}

		if !legit {
			fraudCount++
		}
		fmt.Printf("distance=%f, legit=%t\n", distance, legit)
	}
	err = rows.Err()
	if err != nil {
		log.Fatal((err))
	}

	fmt.Printf("\n[select] query took %v\n", time.Since(selectStart))

	const threshold = 0.6

	fraudScore := float32(fraudCount) / float32(nResults)
	if fraudScore >= threshold {
		return false, fraudScore, nil
	}

	return true, fraudScore, nil
}

func LoadDatasetAndVerifyVector(datasetPath string, vector []float32) (bool, float32, error) {
	return true, 0, nil
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
