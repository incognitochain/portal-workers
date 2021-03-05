package workers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	metadata2 "github.com/0xkraken/incognito-sdk-golang/metadata"
	"github.com/0xkraken/incognito-sdk-golang/wallet"
	"github.com/blockcypher/gobcy"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/incognitochain/incognito-chain/portalv4/metadata"
	"github.com/incognitochain/portal-workers/utils"
)

func (b *BTCWalletMonitor) isReceivingTx(tx *gobcy.TX) bool {
	for _, input := range tx.Inputs {
		if input.Addresses[0] == MultisigAddress {
			return false
		}
	}
	return true
}

func (b *BTCWalletMonitor) buildProof(txID string, blkHeight uint64) (string, error) {
	cypherBlock, err := b.bcy.GetBlock(
		int(blkHeight),
		"",
		map[string]string{
			"txstart": "0",
			"limit":   "500",
		},
	)

	if err != nil {
		return "", err
	}

	txIDs := cypherBlock.TXids
	txHashes := make([]*chainhash.Hash, len(txIDs))
	for i := 0; i < len(txIDs); i++ {
		txHashes[i], _ = chainhash.NewHashFromStr(txIDs[i])
	}

	msgTx := utils.BuildMsgTxFromCypher(txID, b.GetNetwork())
	txHash := msgTx.TxHash()
	blkHash, _ := chainhash.NewHashFromStr(cypherBlock.Hash)

	merkleProofs := utils.BuildMerkleProof(txHashes, &txHash)
	btcProof := utils.BTCProof{
		MerkleProofs: merkleProofs,
		BTCTx:        msgTx,
		BlockHash:    blkHash,
	}
	btcProofBytes, _ := json.Marshal(btcProof)
	btcProofStr := base64.StdEncoding.EncodeToString(btcProofBytes)

	return btcProofStr, nil
}

func (b *BTCWalletMonitor) submitShieldingRequest(incAddress string, proof string) (string, error) {
	rpcClient, keyWallet, err := initSetParams(b.RPCClient)
	if err != nil {
		return "", err
	}
	meta, _ := metadata.NewPortalShieldingRequest(PortalShieldingRequestMeta, BTCID, incAddress, proof)
	var meta2 metadata2.Metadata
	var metaIf interface{}
	metaIf = meta
	meta2 = metaIf.(metadata2.Metadata)
	return sendTx(rpcClient, keyWallet, meta2)
}

func (b *BTCWalletMonitor) extractMemo(memo string) (string, error) {
	if len(memo) <= 4 {
		return "", fmt.Errorf("The memo is too short")
	}
	if memo[:4] != "SP1-" {
		return "", fmt.Errorf("Memo prefix is not match")
	}

	// privacy v1
	incAddress := memo[4:]
	// validate IncogAddressStr
	keyWallet, err := wallet.Base58CheckDeserialize(incAddress)
	if err != nil {
		return "", fmt.Errorf("Incognito address is invalid")
	}
	incogAddr := keyWallet.KeySet.PaymentAddress
	if len(incogAddr.Pk) == 0 {
		return "", fmt.Errorf("Incognito address is invalid")
	}

	return incAddress, nil

	// privacy v2
}
