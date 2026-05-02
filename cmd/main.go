package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/bernardoazevedo/rinha-de-backend-2026/internal"
	"github.com/coder/hnsw"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	err = preProcessAndSaveGraph("./resources/" + os.Getenv("DATASET"))
	if err != nil {
		log.Fatal(err)
	}
}

func preProcessAndSaveGraph(datasetPath string) error {
	var err error
	graph := hnsw.NewGraph[int]()

	graph.M, err = strconv.Atoi(os.Getenv("GRAPH_M"))
	if err != nil {
		return fmt.Errorf("error parsing GRAPH_M: %s", err.Error())
	}

	graph.Ml, err = strconv.ParseFloat(os.Getenv("GRAPH_ML"), 64)
	if err != nil {
		return fmt.Errorf("error parsing GRAPH_ML: %s", err.Error())
	}
	println("using:")
	println("M: ", graph.M)
	println("Ml: ", graph.Ml)

	referenceVectors, err := internal.LoadReferenceVectors(datasetPath)
	if err != nil {
		return fmt.Errorf("error loading reference vectors: %s", err.Error())
	}

	labelMap := map[int]byte{}
	for i, v := range referenceVectors {
		if v.Label == "legit" {
			labelMap[i] = 1
		} else {
			labelMap[i] = 0
		}
		graph.Add(hnsw.MakeNode(i, v.Vector))
	}

	// saving graph
	os.Remove(os.Getenv("GRAPH_FILE_NAME"))
	fileToSave, err := os.OpenFile(os.Getenv("GRAPH_FILE_NAME"), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening file to save graph: %s", err.Error())
	}
	defer fileToSave.Close()

	err = graph.Export(fileToSave)
	if err != nil {
		return fmt.Errorf("error exporting graph: %s", err.Error())
	}
	info, err := fileToSave.Stat()
	if err != nil {
		return fmt.Errorf("error getting graph file info: %s", err.Error())
	}
	println("graph file size: ", float64(info.Size())/1024/1024, "MB")

	// saving label map
	os.Remove(os.Getenv("LABEL_FILE_NAME"))
	labelFile, err := os.OpenFile(os.Getenv("LABEL_FILE_NAME"), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening file to save label file: %s", err.Error())
	}
	defer labelFile.Close()

	labelBytes, err := json.Marshal(labelMap)
	if err != nil {
		return fmt.Errorf("error marshalling label map: %s", err.Error())
	}

	_, err = labelFile.Write(labelBytes)
	if err != nil {
		return fmt.Errorf("error writing label file: %s", err.Error())
	}
	info, err = labelFile.Stat()
	if err != nil {
		return fmt.Errorf("error getting label file info: %s", err.Error())
	}
	println("label file size: ", float64(info.Size())/1024/1024, "MB")

	return nil
}
