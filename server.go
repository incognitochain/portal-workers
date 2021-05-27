package main

import (
	"fmt"
	"os"
	"time"

	"github.com/incognitochain/portal-workers/workers"
)

type Server struct {
	quit    chan os.Signal
	finish  chan bool
	workers []workers.Worker
}

func NewServer(workerIDs []int) *Server {
	listWorkers := []workers.Worker{}
	var err error

	if contain(workerIDs, 1) {
		btcBroadcastingManager := &workers.BTCBroadcastingManager{}
		err = btcBroadcastingManager.Init(1, "BTC Broadcasting Manager", 60, os.Getenv("BTC_NETWORK"))
		if err != nil {
			panic("Can't init BTC Broadcasting Manager")
		}
		listWorkers = append(listWorkers, btcBroadcastingManager)
	}
	if contain(workerIDs, 2) {
		btcWalletMonitorWorker := &workers.BTCWalletMonitor{}
		err = btcWalletMonitorWorker.Init(2, "BTC Wallet Monitor", 60, os.Getenv("BTC_NETWORK"))
		if err != nil {
			panic("Can't init BTC Wallet Monitor")
		}
		listWorkers = append(listWorkers, btcWalletMonitorWorker)
	}
	if contain(workerIDs, 3) {
		btcRelayingHeaderWorker := &workers.BTCRelayerV2{}
		err = btcRelayingHeaderWorker.Init(3, "BTC Header Relayer", 60, os.Getenv("BTC_NETWORK"))
		if err != nil {
			panic("Can't init BTC Header Relayer")
		}
		listWorkers = append(listWorkers, btcRelayingHeaderWorker)
	}
	if contain(workerIDs, 4) {
		relayingAlerterWorker := &workers.RelayingAlerter{}
		err = relayingAlerterWorker.Init(4, "Relaying Alerter", 60, os.Getenv("BTC_NETWORK"))
		if err != nil {
			panic("Can't init Relaying Alerter")
		}
		listWorkers = append(listWorkers, relayingAlerterWorker)
	}
	if contain(workerIDs, 5) {
		unshieldingAlerterWorker := &workers.UnshieldingAlerter{}
		err = unshieldingAlerterWorker.Init(5, "Unshielding Alerter", 60, os.Getenv("BTC_NETWORK"))
		if err != nil {
			panic("Can't init Unshielding Alerter")
		}
		listWorkers = append(listWorkers, unshieldingAlerterWorker)
	}

	quitChan := make(chan os.Signal, 1)
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
