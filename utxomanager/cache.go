package utxomanager

import (
	"fmt"

	"github.com/inc-backend/go-incognito/common"
	"github.com/inc-backend/go-incognito/common/base58"
	"github.com/inc-backend/go-incognito/wallet"
	"github.com/incognitochain/portal-workers/utils"
)

func (c *UTXOManager) getCachedUTXOByPublicKey(publicKey string) map[string][]UTXO {
	_, isExisted := c.Caches[publicKey]
	if isExisted {
		return c.Caches[publicKey]
	}
	return map[string][]UTXO{}
}

func (c *UTXOManager) getListUTXOKeyImagesWithoutCached(utxos []UTXO, publicKey string) []UTXO {
	cachedUTXOs := c.getCachedUTXOByPublicKey(publicKey)

	cached := map[string]bool{} // key image: interface
	for _, utxos := range cachedUTXOs {
		for _, utxo := range utxos {
			cached[utxo.KeyImage] = true
		}
	}

	result := utxos
	idx := 0

	for idx < len(result) {
		keyImage := result[idx].KeyImage
		_, isExisted := cached[keyImage]
		if isExisted {
			result = append(result[:idx], result[idx+1:]...)
		} else {
			idx++
		}
	}

	return result
}

func (c *UTXOManager) cacheUTXOsByTmpTxID(publicKey string, txID string, utxos []UTXO) {
	cachedUTXOs := c.getCachedUTXOByPublicKey(publicKey)
	cachedUTXOs[txID] = utxos
	c.Caches[publicKey] = cachedUTXOs
}

func (c *UTXOManager) uncachedUTXOsByCheckingTxID(publicKey string, rpcClient *utils.HttpClient) {
	cachedUTXOs := c.getCachedUTXOByPublicKey(publicKey)
	for txID := range cachedUTXOs {
		txDetail, err := getTxByHash(rpcClient, txID)
		// tx was rejected or tx was confirmed
		if (txDetail == nil && err != nil) || (txDetail.IsInBlock) {
			delete(cachedUTXOs, txID)
		}
	}
	c.Caches[publicKey] = cachedUTXOs
}

func (c *UTXOManager) UpdateTxID(tmpTxID string, txID string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	val, isExisted := c.Caches[tmpTxID]
	if isExisted {
		delete(c.Caches, tmpTxID)
		c.Caches[txID] = val
	}
}

func (c *UTXOManager) UncachedUTXOByTmpTxID(tmpTxID string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	_, isExisted := c.Caches[tmpTxID]
	if isExisted {
		delete(c.Caches, tmpTxID)
	}
}

func (c *UTXOManager) GetUTXOsByAmount(privateKey string, amount uint64) ([]UTXO, string, error) {
	c.mux.Lock()
	defer c.mux.Unlock()

	keyWallet, err := wallet.Base58CheckDeserialize(privateKey)
	if err != nil {
		return []UTXO{}, "", fmt.Errorf("Can not deserialize private key %v\n", err)
	}
	err = keyWallet.KeySet.InitFromPrivateKey(&keyWallet.KeySet.PrivateKey)
	if err != nil {
		return []UTXO{}, "", fmt.Errorf("sender private key is invalid")
	}
	publicKeyBytes := keyWallet.KeySet.PaymentAddress.Pk
	publicKey := base58.Base58Check{}.Encode(publicKeyBytes, common.ZeroByte)

	fmt.Printf("Length: %v\n", len(c.Unspent[publicKey]))

	rescan := true
	_, isExisted := c.Unspent[publicKey]
	if isExisted {
		sum := uint64(0)
		for _, utxo := range c.Unspent[publicKey] {
			sum += utxo.Amount
		}
		if sum >= amount {
			rescan = false
		}
	}

	if rescan {
		c.uncachedUTXOsByCheckingTxID(publicKey, c.RPCClient)
		utxos, err := getListUTXOs(c.Wallet, privateKey)
		if err != nil {
			return []UTXO{}, "", err
		}
		c.Unspent[publicKey] = c.getListUTXOKeyImagesWithoutCached(utxos, publicKey)
	}

	sum := uint64(0)
	for idx, utxo := range c.Unspent[publicKey] {
		sum += utxo.Amount
		if sum >= amount {
			utxos := c.Unspent[publicKey][:idx+1]
			c.TmpIdx = (c.TmpIdx + 1) % 10000
			tmpTxID := fmt.Sprint("Tmp%v", c.TmpIdx)
			c.cacheUTXOsByTmpTxID(publicKey, tmpTxID, utxos)
			c.Unspent[publicKey] = c.Unspent[publicKey][idx+1:]
			return utxos, tmpTxID, nil
		}
	}

	return []UTXO{}, "", fmt.Errorf("Not enough UTXO")
}

func (c *UTXOManager) CacheUTXOsByTxID(privateKey string, txID string, utxos []UTXO) {
	c.mux.Lock()
	defer c.mux.Unlock()

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

	cachedUTXOs := c.getCachedUTXOByPublicKey(publicKey)
	cachedUTXOs[txID] = utxos
	c.Caches[publicKey] = cachedUTXOs
}

func (c *UTXOManager) GetListUnspentUTXO(privateKey string) ([]UTXO, error) {
	c.mux.Lock()
	defer c.mux.Unlock()

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

	c.uncachedUTXOsByCheckingTxID(publicKey, c.RPCClient)
	utxos, err := getListUTXOs(c.Wallet, privateKey)
	if err != nil {
		return []UTXO{}, err
	}
	c.Unspent[publicKey] = c.getListUTXOKeyImagesWithoutCached(utxos, publicKey)

	return c.Unspent[publicKey], nil
}
