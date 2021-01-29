package workers

import (
	"fmt"

	"github.com/incognitochain/portal-workers/utils"
	"github.com/sirupsen/logrus"
)

type WorkerAbs struct {
	ID        int
	Name      string
	Frequency int // in sec
	Quit      chan bool
	RPCClient *utils.HttpClient
	Network   string // mainnet, testnet, ...
	Logger    *logrus.Entry
}

type Worker interface {
	Execute()
	GetName() string
	GetFrequency() int
	GetQuitChan() chan bool
	GetNetwork() string
}

func (a *WorkerAbs) Execute() {
	fmt.Println("Abstract worker is executing...")
}

func (a *WorkerAbs) GetName() string {
	return a.Name
}

func (a *WorkerAbs) GetFrequency() int {
	return a.Frequency
}

func (a *WorkerAbs) GetQuitChan() chan bool {
	return a.Quit
}

func (a *WorkerAbs) GetNetwork() string {
	return a.Network
}
