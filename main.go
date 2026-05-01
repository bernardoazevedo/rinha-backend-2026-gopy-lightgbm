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
	"github.com/fasthttp/router"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"github.com/valyala/fasthttp"
)

var db *sql.DB
var normalizationConstants internal.NormalizationConstants
var mccRiskMap map[string]float32

func main() {
	sqlite_vec.Auto()

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	log.SetPrefix("main: ")
	if os.Getenv("DEBUG") == "true" {
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile | log.Lmsgprefix)
	} else {
		log.SetFlags(0)
	}

	dbPath := "./transaction.db"
	println("loading database from file to memory...")
	db, err = internal.LoadFileToMemory(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	normalizationConstants, err = internal.LoadNormalizationConstants("./resources/normalization.json")
	if err != nil {
		log.Fatalf("Error loading normalization constants: %s", err)
	}

	mccRiskMap, err = internal.LoadMccRiskMap("./resources/mcc_risk.json")
	if err != nil {
		log.Fatalf("Error loading mcc risk map: %s", err)
	}

	r := router.New()
	r.GET("/ready", isReady)
	r.POST("/fraud-score", fraudScore)

	log.Printf("application started")

	if os.Getenv("DEBUG") == "true" {
		log.Fatal(fasthttp.ListenAndServe(":1234", Logger(r.Handler)))
	} else {
		log.Fatal(fasthttp.ListenAndServe(":1234", r.Handler))
	}
}

func Logger(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	// 2026-02-22 21:27:26 | GET "/ready" | 200 | 3.24189ms
	return func(ctx *fasthttp.RequestCtx) {
		startTime := time.Now()
		next(ctx)
		duration := time.Since(startTime)
		fmt.Printf(
			"%s | %s \"%s\" | %d | %s\n",
			startTime.UTC().Add(-3*time.Hour).Format(time.DateTime),
			ctx.Method(),
			ctx.Path(),
			ctx.Response.StatusCode(),
			duration,
		)
	}
}

func isReady(ctx *fasthttp.RequestCtx) {
	ready := true
	if ready {
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		return
	} else {
		ctx.Response.SetStatusCode(fasthttp.StatusServiceUnavailable)
		return
	}
}

func fraudScore(ctx *fasthttp.RequestCtx) {
	var transaction internal.Transaction
	err := json.Unmarshal(ctx.Request.Body(), &transaction)
	if err != nil {
		log.Printf("Error unmarshalling transaction: %s", err)
		ctx.Response.SetStatusCode(fasthttp.StatusBadRequest)
		return
	}

	vector, err := internal.TransactionToVector(transaction, normalizationConstants, mccRiskMap)
	if err != nil {
		log.Printf("Error transforming transaction to vector: %s", err)
		ctx.Response.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	approved, fraudScore, err := internal.Query(db, vector)
	if err != nil {
		log.Printf("Error verifying vector: %s", err)
		ctx.Response.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	response := internal.FraudScoreResponse{
		Approved:   approved,
		FraudScore: fraudScore,
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling response: %s", err)
		ctx.Response.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.Response.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.SetBody(jsonResponse)
}
