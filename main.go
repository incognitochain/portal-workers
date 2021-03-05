package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/incognitochain/portal-workers/utils"
	"github.com/incognitochain/portal-workers/workers"
	"github.com/joho/godotenv"
)

func main() {
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
	s := NewServer()

	// split utxos before executing agents
	if os.Getenv("SPLITUTXO") == "true" {
		incognitoPrivateKey := os.Getenv("INCOGNITO_PRIVATE_KEY")
		minNumUTXOTmp := os.Getenv("NUMUTXO")
		minNumUTXOs, _ := strconv.Atoi(minNumUTXOTmp)

		rpcClient := utils.NewHttpClient("", os.Getenv("INCOGNITO_PROTOCOL"), os.Getenv("INCOGNITO_HOST"), os.Getenv("INCOGNITO_PORT")) // incognito chain rpc endpoint
		err := workers.SplitUTXOs(rpcClient, incognitoPrivateKey, minNumUTXOs)
		if err != nil {
			fmt.Printf("Split utxos error: %v\n", err)
			return
		}
	}

	s.Run()
	for range s.workers {
		<-s.finish
	}
	fmt.Println("Server stopped gracefully!")
}
