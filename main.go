package main

import (
	"fmt"
	"os"
	"runtime"

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
		// TODO: Split UTXO
	}

	s.Run()
	for range s.workers {
		<-s.finish
	}
	fmt.Println("Server stopped gracefully!")
}
