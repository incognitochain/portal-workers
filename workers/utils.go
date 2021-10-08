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
