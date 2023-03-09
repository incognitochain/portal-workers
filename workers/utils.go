package workers

import (
	"encoding/json"
	"fmt"
	"os"

	resty "github.com/go-resty/resty/v2"
)

type BlockchainFeeResponse struct {
	Result float64
	Error  error
}

func getBitcoinFee() (float64, error) {
	client := resty.New()

	response, err := client.R().
		Get(os.Getenv("BLOCKCHAIN_FEE_HOST"))

	if err != nil {
		return 0, err
	}
	if response.StatusCode() != 200 {
		return 0, fmt.Errorf("Response status code: %v", response.StatusCode())
	}
	var responseBody BlockchainFeeResponse
	err = json.Unmarshal(response.Body(), &responseBody)
	if err != nil {
		return 0, fmt.Errorf("Could not parse response: %v", response.Body())
	}
	return responseBody.Result, nil
}

func broadcastTxToBlockStream(txHex string) (string, error) {
	client := resty.New()

	url := "https://blockstream.info/api/tx"

	response, err := client.R().SetBody(txHex).Post(url)
	if err != nil {
		return "", err
	}
	fmt.Printf("response: %+v\n", response)
	if response.StatusCode() != 200 {
		return "", fmt.Errorf("Request status code: %v - Error %v", response.StatusCode(), response)
	}

	return response.String(), nil
}
