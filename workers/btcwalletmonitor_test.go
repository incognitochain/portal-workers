package workers

import (
	"testing"
)

func TestExtractMemo(t *testing.T) {
	b := &BTCWalletMonitor{}

	type TestCase struct {
		memo               string
		expectedIncAddress string
	}

	cases := []*TestCase{
		{
			memo:               "12S4oseu3scZJJuoLeGSEYZka1mmxHBNg7VbC1tQ67ZDjTUqzuRY4ABf4Bjop7uR22U1AxsLEheixfenpcc63tVG7E7QxF1zy5r1SXv",
			expectedIncAddress: "",
		},
		{
			memo:               "PS1-12S5Lrs1XeQLbqN4ySyKtjAjd2d7sBP2tjFijzmp6avrrkQCNFMpkXm3FPzj2Wcu2ZNqJEmh9JriVuRErVwhuQnLmWSaggobEWsBEci",
			expectedIncAddress: "12S5Lrs1XeQLbqN4ySyKtjAjd2d7sBP2tjFijzmp6avrrkQCNFMpkXm3FPzj2Wcu2ZNqJEmh9JriVuRErVwhuQnLmWSaggobEWsBEci",
		},
		{
			memo:               "PS1-12S5Lrs1XeQLbqM4ySyKtjAjd2d7sBP2tjFijzmp6avrrkQCNFMpkXm3FPzj2Wcu2ZNqJEmh9JriVuRErVwhuQnLmWSaggobEWsBEci",
			expectedIncAddress: "",
		},
	}

	for _, testcase := range cases {
		outputIncAddress, _ := b.extractMemo(testcase.memo)
		if outputIncAddress != testcase.expectedIncAddress {
			t.FailNow()
		}
	}
}
