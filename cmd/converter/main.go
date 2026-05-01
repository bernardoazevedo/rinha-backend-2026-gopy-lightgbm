package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
)

const vectorDimensions = 14
const recordSize = 1 + vectorDimensions*4 // 1 byte label + 14 x float32

type TransactionVector struct {
	Vector []float32 `json:"vector"`
	Label  string    `json:"label"`
}

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("Usage: %s <input.json> <output.bin>", os.Args[0])
	}

	inputPath := os.Args[1]
	outputPath := os.Args[2]

	inputFile, err := os.Open(inputPath)
	if err != nil {
		log.Fatalf("Error opening input file: %s", err)
	}
	defer inputFile.Close()

	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("Error creating output file: %s", err)
	}
	defer outputFile.Close()

	writer := bufio.NewWriterSize(outputFile, 256*1024) // 256KB buffer

	// Write placeholder header (will update with actual count at the end)
	if err := binary.Write(writer, binary.LittleEndian, uint32(0)); err != nil {
		log.Fatalf("Error writing header: %s", err)
	}

	decoder := json.NewDecoder(bufio.NewReaderSize(inputFile, 256*1024))

	// Read opening bracket '['
	token, err := decoder.Token()
	if err != nil {
		log.Fatalf("Error reading opening token: %s", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		log.Fatalf("Expected '[', got %v", token)
	}

	recordBuf := make([]byte, recordSize)
	var count uint32

	for decoder.More() {
		var tv TransactionVector
		if err := decoder.Decode(&tv); err != nil {
			log.Fatalf("Error decoding vector #%d: %s", count, err)
		}

		if len(tv.Vector) != vectorDimensions {
			log.Fatalf("Vector #%d has %d dimensions, expected %d", count, len(tv.Vector), vectorDimensions)
		}

		// Pack record into buffer: [1 byte label][14 x 4 bytes float32]
		if tv.Label == "fraud" {
			recordBuf[0] = 1
		} else {
			recordBuf[0] = 0
		}

		for j, v := range tv.Vector {
			binary.LittleEndian.PutUint32(recordBuf[1+j*4:], math.Float32bits(v))
		}

		if _, err := writer.Write(recordBuf); err != nil {
			log.Fatalf("Error writing record #%d: %s", count, err)
		}

		count++
		if count%100000 == 0 {
			fmt.Printf("converted %d vectors...\n", count)
		}
	}

	if err := writer.Flush(); err != nil {
		log.Fatalf("Error flushing writer: %s", err)
	}

	// Update header with actual count
	if _, err := outputFile.Seek(0, 0); err != nil {
		log.Fatalf("Error seeking to header: %s", err)
	}
	if err := binary.Write(outputFile, binary.LittleEndian, count); err != nil {
		log.Fatalf("Error updating header count: %s", err)
	}

	fmt.Printf("done: %d vectors written to %s\n", count, outputPath)
}
