package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	go_incognito "github.com/inc-backend/go-incognito"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/incognitochain/portal-workers/utxomanager"
	"github.com/joho/godotenv"
)

func main() {
	var envFile string
	flag.StringVar(&envFile, "config", ".env_dev", ".env config file")
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

	publicIncognito := go_incognito.NewPublicIncognito(
		fmt.Sprintf("%v://%v:%v", os.Getenv("INCOGNITO_PROTOCOL"), os.Getenv("INCOGNITO_HOST"), os.Getenv("INCOGNITO_PORT")),
		os.Getenv("INCOGNITO_COINSERVICE_URL"),
	)
	blockInfo := go_incognito.NewBlockInfo(publicIncognito)
	wallet := go_incognito.NewWallet(publicIncognito, blockInfo)
	httpClient := utils.NewHttpClient("", os.Getenv("INCOGNITO_PROTOCOL"), os.Getenv("INCOGNITO_HOST"), os.Getenv("INCOGNITO_PORT"))

	utxoManager := utxomanager.NewUTXOManager(wallet, httpClient)

	runtime.GOMAXPROCS(runtime.NumCPU())
	s := NewServer(utxoManager, workerIDs)

	// split utxos before executing workers
	if os.Getenv("SPLITUTXO") == "true" {
		privateKey := os.Getenv("INCOGNITO_PRIVATE_KEY")
		paymentAddress := os.Getenv("INCOGNITO_PAYMENT_ADDRESS")
		minNumUTXOsStr := os.Getenv("NUMUTXO")
		minNumUTXOs, _ := strconv.Atoi(minNumUTXOsStr)
		protocol := os.Getenv("INCOGNITO_PROTOCOL")
		endpointUri := fmt.Sprintf("%v://%v:%v", os.Getenv("INCOGNITO_PROTOCOL"), os.Getenv("INCOGNITO_HOST"), os.Getenv("INCOGNITO_PORT"))

		err := utxomanager.SplitUTXOs(
			endpointUri, protocol, privateKey, paymentAddress, minNumUTXOs, utxoManager, httpClient,
		)
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
