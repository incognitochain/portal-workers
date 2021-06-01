package utxomanager

import (
	"fmt"

	go_incognito "github.com/inc-backend/go-incognito"
	"github.com/inc-backend/go-incognito/common"
	"github.com/inc-backend/go-incognito/common/base58"
	"github.com/inc-backend/go-incognito/wallet"
	"github.com/incognitochain/portal-workers/utils"
)

func GetListUTXOs(w *go_incognito.Wallet, privateKey string) ([]UTXO, error) {
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

func GetListUnspentUTXO(
	w *go_incognito.Wallet, privateKey string, utxoCache *UTXOCache, rpcClient *utils.HttpClient,
) ([]UTXO, error) {
	keyWallet, err := wallet.Base58CheckDeserialize(privateKey)
	if err != nil {
		return []UTXO{}, fmt.Errorf("Can not deserialize private key %v\n", err)
	}
	err = keyWallet.KeySet.InitFromPrivateKey(&keyWallet.KeySet.PrivateKey)
	if err != nil {
		return []UTXO{}, fmt.Errorf("sender private key is invalid")
	}
	publicKeyBytes := keyWallet.KeySet.PaymentAddress.Pk
	publicKey := base58.Base58Check{}.Encode(publicKeyBytes, common.ZeroByte)

	utxoCache.UncachedUTXOsByCheckingTxID(publicKey, rpcClient)

	utxos, err := GetListUTXOs(w, privateKey)
	if err != nil {
		return []UTXO{}, err
	}

	return utxoCache.GetListUTXOKeyImagesWithoutCached(utxos, publicKey), nil
}

func CacheSpentUTXOs(privateKey string, txID string, utxos []UTXO, utxoManager *UTXOCache) {
	keyWallet, err := wallet.Base58CheckDeserialize(privateKey)
	if err != nil {
		return
	}
	err = keyWallet.KeySet.InitFromPrivateKey(&keyWallet.KeySet.PrivateKey)
	if err != nil {
		return
	}
	publicKeyBytes := keyWallet.KeySet.PaymentAddress.Pk
	publicKey := base58.Base58Check{}.Encode(publicKeyBytes, common.ZeroByte)

	utxoManager.CacheUTXOsByTxID(publicKey, txID, utxos)
}
