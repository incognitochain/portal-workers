package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/incognitochain/portal-workers/workers"
)

type Server struct {
	quit    chan os.Signal
	finish  chan bool
	workers []workers.Worker
}

func NewServer() *Server {
	listWorkers := []workers.Worker{}
	var err error

	btcBroadcastingManager := &workers.BTCBroadcastingManager{}
	err = btcBroadcastingManager.Init(1, "BTC Broadcasting Manager", 60, os.Getenv("BTC_NETWORK"))
	if err != nil {
		panic("Can't init BTC Broadcasting Manager")
	}

	btcWalletMonitorWorker := &workers.BTCWalletMonitor{}
	err = btcWalletMonitorWorker.Init(1, "BTC Wallet Monitor", 60, os.Getenv("BTC_NETWORK"))
	if err != nil {
		panic("Can't init BTC Wallet Monitor")
	}

	listWorkers = append(listWorkers, btcBroadcastingManager)
	listWorkers = append(listWorkers, btcWalletMonitorWorker)

	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, syscall.SIGTERM)
	signal.Notify(quitChan, syscall.SIGINT)
	return &Server{
		quit:    quitChan,
		finish:  make(chan bool, len(listWorkers)),
		workers: listWorkers,
	}
}

func (s *Server) NotifyQuitSignal(workers []workers.Worker) {
	sig := <-s.quit
	fmt.Printf("Caught sig: %+v \n", sig)
	// notify all workers about quit signal
	for _, a := range workers {
		a.GetQuitChan() <- true
	}
}

func (s *Server) Run() {
	workers := s.workers
	go s.NotifyQuitSignal(workers)
	for _, a := range workers {
		go executeWorker(s.finish, a)
	}
}

func executeWorker(finish chan bool, worker workers.Worker) {
	worker.Execute() // execute as soon as starting up
	for {
		select {
		case <-worker.GetQuitChan():
			fmt.Printf("Finishing task for %s ...\n", worker.GetName())
			time.Sleep(time.Second * 1)
			fmt.Printf("Task for %s done! \n", worker.GetName())
			finish <- true
			break
		case <-time.After(time.Duration(worker.GetFrequency()) * time.Second):
			worker.Execute()
		}
	}
}
