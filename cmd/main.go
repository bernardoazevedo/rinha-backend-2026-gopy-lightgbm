package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/bernardoazevedo/rinha-de-backend-2026/internal"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	totalStart := time.Now()

	referencesPath := "./resources/references-lite.json"


	println("opening database")
	sqlite_vec.Auto()
	// os.Remove("./transaction.db")
	// db, err := sql.Open("sqlite3", "transaction.db")
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	// db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	println("creating virtual table vec_items")
	_, err = db.Exec("CREATE VIRTUAL TABLE vec_items USING vec0(embedding float[14], legit boolean)")
	if err != nil {
		log.Fatal(err)
	}

	println("reading references from: ", referencesPath)
	referenceVectors, err := internal.LoadReferenceVectors(referencesPath)
	if err != nil {
		log.Fatal(err)
	}

	// inserting
	insertTotalStart := time.Now()
	for id, item := range referenceVectors {
		insertStart := time.Now()

		v, err := sqlite_vec.SerializeFloat32(item.Vector)
		if err != nil {
			log.Fatal(err)
		}

		legit := item.Label == "legit"
		_, err = db.Exec("INSERT INTO vec_items(rowid, embedding, legit) VALUES (?, ?, ?)", id, v, legit)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("[insert] id=%d took %v\n", id, time.Since(insertStart))
	}
	insertTotalElapsed := time.Since(insertTotalStart)
	fmt.Printf("\n[insert total] %d items in %v\n\n", len(referenceVectors), insertTotalElapsed)

	// searching
	q := []float32{
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
	}
	query, err := sqlite_vec.SerializeFloat32(q)
	if err != nil {
		log.Fatal(err)
	}

	selectStart := time.Now()
	rows, err := db.Query(`
		SELECT
			rowid,
			distance,
			legit
		FROM vec_items
		WHERE embedding MATCH ?
		ORDER BY distance
		LIMIT 3
	`, query)

	if err != nil {
		log.Fatal(err)
	}

	for rows.Next() {
		var rowid int64
		var distance float64
		var legit bool
		err = rows.Scan(&rowid, &distance, &legit)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("rowid=%d, distance=%f, legit=%t\n", rowid, distance, legit)
	}
	err = rows.Err()
	if err != nil {
		log.Fatal((err))
	}

	fmt.Printf("\n[select] query took %v\n", time.Since(selectStart))
	fmt.Printf("\n[total] execution took %v\n", time.Since(totalStart))
}
