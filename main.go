package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/incognitochain/go-incognito-sdk-v2/incclient"
	"github.com/incognitochain/portal-workers/utxomanager"
	"github.com/joho/godotenv"
)

func main() {
	var envFile string
	flag.StringVar(&envFile, "config", ".env_main", ".env config file")
	flag.Parse()

	err := godotenv.Load(envFile)
	if err != nil {
		panic(fmt.Sprintf("Error loading %v file", envFile))
	}

	var myEnv map[string]string
	myEnv, _ = godotenv.Read(envFile)
	fmt.Println("=========Config============")
	for key, value := range myEnv {
		fmt.Println(key + ": " + value)
	}
	fmt.Println("=========End============")

	workersStr := os.Getenv("WORKER_IDS")
	workerIDsStr := strings.Split(workersStr, ",")
	workerIDs := []int{}
	for _, str := range workerIDsStr {
		workerIDInt, err := strconv.Atoi(str)
		if err != nil {
			panic("Worker ID is invalid")
		}
		workerIDs = append(workerIDs, workerIDInt)
	}

	incClient, err := incclient.NewIncClient(
		fmt.Sprintf("%v://%v:%v", os.Getenv("INCOGNITO_PROTOCOL"), os.Getenv("INCOGNITO_HOST"), os.Getenv("INCOGNITO_PORT")),
		"",
		2,
	)
	if err != nil {
		panic(fmt.Sprintf("Could not init IncClient: %v\n", err))
	}

	utxoManager := utxomanager.NewUTXOManager(incClient)

	runtime.GOMAXPROCS(runtime.NumCPU())
	s := NewServer(utxoManager, workerIDs)

	// split utxos before executing workers
	if os.Getenv("SPLITUTXO") == "true" {
		privateKey := os.Getenv("INCOGNITO_PRIVATE_KEY")
		paymentAddress := os.Getenv("INCOGNITO_PAYMENT_ADDRESS")
		minNumUTXOsStr := os.Getenv("NUMUTXO")
		minNumUTXOs, _ := strconv.Atoi(minNumUTXOsStr)

		err := utxomanager.SplitUTXOs(privateKey, paymentAddress, minNumUTXOs, utxoManager)
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
