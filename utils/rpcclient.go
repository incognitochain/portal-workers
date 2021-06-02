package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/incognitochain/portal-workers/entities"
)

type HttpClient struct {
	*http.Client
	url      string
	protocol string
	host     string
	port     string
}

// NewHttpClient to get http client instance
func NewHttpClient(url string, protocol string, host string, port string) *HttpClient {
	httpClient := &http.Client{
		Timeout: time.Second * 60,
	}
	return &HttpClient{
		Client:   httpClient,
		url:      url,
		protocol: protocol,
		host:     host,
		port:     port,
	}
}

func buildHttpServerAddress(url string, protocol string, host string, port string) string {
	if url != "" {
		return url
	}
	return fmt.Sprintf("%s://%s:%s", protocol, host, port)
}

func (client *HttpClient) RPCCall(
	method string,
	params interface{},
	rpcResponse interface{},
) (err error) {
	rpcEndpoint := buildHttpServerAddress(
		client.url, client.protocol, client.host, client.port,
	)
	payload := map[string]interface{}{
		"method": method,
		"params": params,
		"id":     0,
	}
	payloadInBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := client.Post(rpcEndpoint, "application/json", bytes.NewBuffer(payloadInBytes))
	// resp, err := client.Post("http://192.168.1.101:9334", "application/json", bytes.NewBuffer(payloadInBytes))

	if err != nil {
		fmt.Println("calling err: ", err)
		return err
	}

	respBody := resp.Body
	defer respBody.Close()

	body, err := ioutil.ReadAll(respBody)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, rpcResponse)
	if err != nil {
		return err
	}
	return nil
}

func (client HttpClient) GetURL() string {
	return buildHttpServerAddress(client.url, client.protocol, client.host, client.port)
}

func GetTxByHash(rpcClient *HttpClient, txID string) (*entities.TxDetail, error) {
	var txByHashRes entities.TxDetailRes
	params := []interface{}{
		txID,
	}
	err := rpcClient.RPCCall("gettransactionbyhash", params, &txByHashRes)
	if err != nil {
		return nil, err
	}
	if txByHashRes.RPCError != nil {
		return nil, errors.New(txByHashRes.RPCError.Message)
	}
	return txByHashRes.Result, nil
}
