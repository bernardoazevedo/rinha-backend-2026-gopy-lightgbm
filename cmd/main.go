package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/bernardoazevedo/rinha-de-backend-2026/internal"
	"github.com/joho/godotenv"
)

func main() {
	totalStart := time.Now()

	if len(os.Args) < 2 {
		log.Fatal("usage: main <create|query>")
	}
	mode := os.Args[1]

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	sqlite_vec.Auto()

	switch mode {
	case "create":
		runCreate()
	case "query":
		dbPath := "./transaction.db"

		println("loading database from file to memory...")
		loadStart := time.Now()

		db, err := internal.LoadFileToMemory(dbPath)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		fmt.Printf("[load] database loaded to memory in %v\n\n", time.Since(loadStart))

		println("querying database...")
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

		queryStmt, err := internal.PrepareQueryStatement(db)
		if err != nil {
			log.Printf("Error preparing query statement: %s", err)
			return
		}
		approved, fraudScore, err := internal.Query(queryStmt, q)
		if err != nil {
			log.Printf("Error verifying vector: %s", err)
			return
		}

		response := internal.FraudScoreResponse{
			Approved:   approved,
			FraudScore: fraudScore,
		}

		jsonResponse, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			log.Printf("Error marshalling response: %s", err)
			return
		}

		println(string(jsonResponse))
	default:
		log.Fatalf("unknown mode: %s (use 'create' or 'query')", mode)
	}

	fmt.Printf("\n[total] execution took %v\n", time.Since(totalStart))
}

func runCreate() {
	referencesPath := "./resources/"+os.Getenv("DATASET")
	dbPath := "./transaction.db"

	println("opening database")
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
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

	loadStart := time.Now()
	newDb, err := internal.LoadMemoryToFile(db, dbPath)
	if err != nil {
		log.Fatal(err)
	}
	newDb.Close()
	fmt.Printf("\n[load to file] loaded to file in %v\n\n", time.Since(loadStart))
}
