package main

import (
	"fmt"

	"github.com/bernardoazevedo/rinha-de-backend-2026/internal"
)

func main() {
	vector := []float32{
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
	approved, fraudScore := internal.LoadDatasetAndVerifyVector("./resources/references.json", vector)
	fmt.Println("aproved:", approved)
	fmt.Println("fraudScore:", fraudScore)
}
