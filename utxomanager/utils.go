package utxomanager

import (
	go_incognito "github.com/inc-backend/go-incognito"
)

func getListUTXOs(w *go_incognito.Wallet, privateKey string) ([]UTXO, error) {
	inputCoins, err := w.GetUTXO(privateKey, PRVIDStr)
	if err != nil {
		return []UTXO{}, err
	}

	utxos := []UTXO{}
	for _, coin := range inputCoins {
		utxos = append(utxos, UTXO{
			KeyImage: coin.KeyImages,
			Amount:   coin.Value,
		})
	}
	return utxos, nil
}
