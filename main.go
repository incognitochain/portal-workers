package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/incognitochain/portal-workers/utils"
	"github.com/joho/godotenv"
)

func main() {
	flag.Parse()
	tail := flag.Args()
	workerIDs := []int{}
	for _, str := range tail {
		one_int, err := strconv.Atoi(str)
		if err != nil {
			panic("Worker ID is invalid")
		}
		workerIDs = append(workerIDs, one_int)
	}
	fmt.Printf("List of executed worker IDs: %+v\n", workerIDs)

	err := godotenv.Load()
	if err != nil {
		panic("Error loading .env file")
	}

	var myEnv map[string]string
	myEnv, _ = godotenv.Read()
	fmt.Println("=========Config============")
	for key, value := range myEnv {
		fmt.Println(key + ": " + value)
	}
	fmt.Println("=========End============")

	runtime.GOMAXPROCS(runtime.NumCPU())
	s := NewServer(workerIDs)

	// split utxos before executing workers
	if os.Getenv("SPLITUTXO") == "true" {
		privateKey := os.Getenv("INCOGNITO_PRIVATE_KEY")
		paymentAddress := os.Getenv("INCOGNITO_PAYMENT_ADDRESS")
		minNumUTXOsStr := os.Getenv("NUMUTXO")
		minNumUTXOs, _ := strconv.Atoi(minNumUTXOsStr)
		protocol := os.Getenv("INCOGNITO_PROTOCOL")
		endpointUri := fmt.Sprintf("%v://%v:%v", os.Getenv("INCOGNITO_PROTOCOL"), os.Getenv("INCOGNITO_HOST"), os.Getenv("INCOGNITO_PORT"))

		err := utils.SplitUTXOs(endpointUri, protocol, privateKey, paymentAddress, minNumUTXOs)
		if err != nil {
			panic("Could not split UTXOs")
		}
	}

	s.Run()
	for range s.workers {
		<-s.finish
	}
	fmt.Println("Server stopped gracefully!")
}
