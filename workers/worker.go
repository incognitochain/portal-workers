package workers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	go_incognito "github.com/inc-backend/go-incognito"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/sirupsen/logrus"
)

type WorkerAbs struct {
	ID        int
	Name      string
	Frequency int // in sec
	Quit      chan bool
	RPCClient *utils.HttpClient
	Client    *go_incognito.PublicIncognito
	Network   string // mainnet, testnet, ...
	Logger    *logrus.Entry
}

type Worker interface {
	Init(id int, name string, freq int, network string) error
	Execute()
	ExportErrorLog(msg string)
	ExportInfoLog(msg string)
	GetName() string
	GetFrequency() int
	GetQuitChan() chan bool
	GetNetwork() string
}

func (a *WorkerAbs) Init(id int, name string, freq int, network string) error {
	a.ID = id
	a.Name = name
	a.Frequency = freq
	a.Quit = make(chan bool)

	a.RPCClient = utils.NewHttpClient("", os.Getenv("INCOGNITO_PROTOCOL"), os.Getenv("INCOGNITO_HOST"), os.Getenv("INCOGNITO_PORT"))

	client := &http.Client{}
	publicIncognito := go_incognito.NewPublicIncognito(
		client,
		fmt.Sprintf("%v://%v:%v", os.Getenv("INCOGNITO_PROTOCOL"), os.Getenv("INCOGNITO_HOST"), os.Getenv("INCOGNITO_PORT")),
		"",
		//CoinServiceTest,
		2,
	)
	a.Client = publicIncognito

	a.Network = network
	logger, err := instantiateLogger(a.Name)
	if err != nil {
		panic(fmt.Sprintf("Could instantiate a logger for worker: %v\n", a.Name))
	}
	a.Logger = logger
	return err
}

func (a *WorkerAbs) Execute() {
	fmt.Println("Abstract worker is executing...")
}

func (a *WorkerAbs) ExportErrorLog(msg string) {
	a.Logger.Error(msg)
	utils.SendSlackNotification(fmt.Sprintf("[ERR] %v", msg))
}

func (a *WorkerAbs) ExportInfoLog(msg string) {
	a.Logger.Info(msg)
	utils.SendSlackNotification(fmt.Sprintf("[INF] %v", msg))
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

func instantiateLogger(workerName string) (*logrus.Entry, error) {
	var log = logrus.New()
	logsPath := filepath.Join(".", "logs")
	os.MkdirAll(logsPath, os.ModePerm)
	file, err := os.OpenFile(fmt.Sprintf("%s/%s.log", logsPath, workerName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Infof("Failed to log to file - with error: %v", err)
		return nil, err
	}
	log.Out = file
	logger := log.WithFields(logrus.Fields{
		"worker": workerName,
	})
	return logger, nil
}
