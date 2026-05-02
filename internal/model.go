package internal

import (
	"github.com/coder/hnsw"
)

// https://github.com/zanfranceschi/rinha-de-backend-2026/blob/main/docs/br/API.md#campos-da-requisição
//
// ### Campos da requisição
// | Campo                        | Tipo             | Descrição										 |
// |------------------------------|------------------|---------------------------------------------------|
// | `id`                         | string      	 | Identificador da transação (ex.: `tx-1329056812`) |
// | `transaction.amount`         | number      	 | Valor da transação |
// | `transaction.installments`   | integer     	 | Número de parcelas |
// | `transaction.requested_at`   | string ISO  	 | Timestamp UTC da requisição |
// | `customer.avg_amount`        | number      	 | Média histórica de gasto do portador do cartão |
// | `customer.tx_count_24h`      | integer     	 | Quantidade de transações do portador nas últimas 24h |
// | `customer.known_merchants`   | string[]    	 | Comerciantes já utilizados pelo portador |
// | `merchant.id`                | string      	 | Identificador do comerciante |
// | `merchant.mcc`               | string      	 | MCC (Merchant Category Code), código da categoria do comerciante |
// | `merchant.avg_amount`        | number      	 | Ticket médio do comerciante |
// | `terminal.is_online`         | boolean     	 | Indica se a transação é online (`true`) ou presencial (`false`) |
// | `terminal.card_present`      | boolean     	 | Indica se o cartão está presente no terminal |
// | `terminal.km_from_home`      | number      	 | Distância, em km, do endereço do portador |
// | `last_transaction`           | object \| `null` | Dados da transação anterior (pode ser `null` quando não houver transação anterior) |
// | `last_transaction.timestamp` | string ISO 		 | Timestamp UTC da transação anterior |

type Transaction struct {
	ID              string           `json:"id"`
	Transaction     TransactionData  `json:"transaction"`
	Customer        CustomerData     `json:"customer"`
	Merchant        MerchantData     `json:"merchant"`
	Terminal        TerminalData     `json:"terminal"`
	LastTransaction *LastTransaction `json:"last_transaction,omitempty"`
}

type TransactionData struct {
	Amount       float32 `json:"amount"`
	Installments float32 `json:"installments"`
	RequestedAt  string  `json:"requested_at"`
}

type CustomerData struct {
	AvgAmount      float32  `json:"avg_amount"`
	TxCount24h     float32  `json:"tx_count_24h"`
	KnownMerchants []string `json:"known_merchants"`
}

type MerchantData struct {
	ID        string  `json:"id"`
	MCC       string  `json:"mcc"`
	AvgAmount float32 `json:"avg_amount"`
}

type TerminalData struct {
	IsOnline    bool    `json:"is_online"`
	CardPresent bool    `json:"card_present"`
	KmFromHome  float32 `json:"km_from_home"`
}

type LastTransaction struct {
	Timestamp     string  `json:"timestamp"`
	KmFromCurrent float32 `json:"km_from_current"`
}

type Response struct {
	Approved   bool    `json:"approved"`
	FraudScore float32 `json:"fraud_score"`
}

type NormalizationConstants struct {
	MaxAmount            float32 `json:"max_amount"`
	MaxInstallments      float32 `json:"max_installments"`
	AmountVsAvgRatio     float32 `json:"amount_vs_avg_ratio"`
	MaxMinutes           float32 `json:"max_minutes"`
	MaxKm                float32 `json:"max_km"`
	MaxTxCount24h        float32 `json:"max_tx_count_24h"`
	MaxMerchantAvgAmount float32 `json:"max_merchant_avg_amount"`
}

type TransactionVector struct {
	Vector []float32 `json:"vector"`
	Label  string    `json:"label"`
}

type VectorDatabase struct {
	graph    *hnsw.Graph[int]
	labelMap map[int]bool
}

type FraudScoreResponse struct {
	Approved   bool    `json:"approved"`
	FraudScore float32 `json:"fraud_score"`
}
