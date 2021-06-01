package utxomanager

import (
	"sync"

	"github.com/incognitochain/portal-workers/utils"
)

type UTXO struct {
	KeyImage string
	Amount   uint64
}

type UTXOCache struct {
	Caches map[string]map[string][]UTXO // public key: txID: UTXO
	mux    sync.Mutex
}

func (c *UTXOCache) getCachedUTXOByPublicKey(publicKey string) map[string][]UTXO {
	_, isExisted := c.Caches[publicKey]
	if isExisted {
		return c.Caches[publicKey]
	}
	return map[string][]UTXO{}
}

func (c *UTXOCache) GetListUTXOKeyImagesWithoutCached(utxos []UTXO, publicKey string) []UTXO {
	c.mux.Lock()
	defer c.mux.Unlock()

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

func (c *UTXOCache) CacheUTXOsByTxID(publicKey string, txID string, utxos []UTXO) {
	c.mux.Lock()
	defer c.mux.Unlock()

	cachedUTXOs := c.getCachedUTXOByPublicKey(publicKey)
	cachedUTXOs[txID] = utxos
	c.Caches[publicKey] = cachedUTXOs
}

func (c *UTXOCache) UncachedUTXOsByTxID(publicKey string, clearTxID string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	cachedUTXOs := c.getCachedUTXOByPublicKey(publicKey)
	delete(cachedUTXOs, clearTxID)
	c.Caches[publicKey] = cachedUTXOs
}

func (c *UTXOCache) UncachedUTXOsByCheckingTxID(publicKey string, rpcClient *utils.HttpClient) {
	c.mux.Lock()
	defer c.mux.Unlock()

	cachedUTXOs := c.getCachedUTXOByPublicKey(publicKey)
	for txID := range cachedUTXOs {
		txDetail, err := GetTxByHash(rpcClient, txID)
		// tx was rejected or tx was confirmed
		if (txDetail == nil && err != nil) || (txDetail.IsInBlock) {
			delete(cachedUTXOs, txID)
		}
	}
	c.Caches[publicKey] = cachedUTXOs
}
