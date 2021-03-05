package workers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	metadata2 "github.com/0xkraken/incognito-sdk-golang/metadata"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/incognitochain/incognito-chain/portalv4/metadata"
	"github.com/incognitochain/incognito-chain/wallet"
	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"
)

func (b *BTCBroadcastingManager) isTimeoutBTCTx(broadcastBlockHeight uint64, curBlockHeight uint64) bool {
	return curBlockHeight-broadcastBlockHeight <= TimeoutBTCFeeReplacement
}

// return boolean value of transaction confirmation and bitcoin block height
func (b *BTCBroadcastingManager) isConfirmedBTCTx(txHash string) (bool, uint64) {
	tx, err := b.bcy.GetTX(txHash, nil)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not check the confirmation of tx in BTC chain - with err: %v \n Tx hash: %v", err, txHash))
		return false, 0
	}
	return tx.Confirmations >= ConfirmationThreshold, uint64(tx.BlockHeight)
}

func (b *BTCBroadcastingManager) broadcastTx(txContent string) error {
	skel, err := b.bcy.PushTX(txContent)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not broadcast tx to BTC chain - with err: %v \n Decoded tx: %v", err, skel))
		return err
	}
	return nil
}

// Return fee per unshield request and number of requests
func (b *BTCBroadcastingManager) getUnshieldTxInfo(btcTxContent string) (uint, uint) {
	skel, err := b.bcy.DecodeTX(btcTxContent)
	if err != nil {
		panic(fmt.Sprintf("Can't parse tx content: %v", btcTxContent))
	}
	fee := skel.Trans.Fees
	numberOfRequest := uint(0)

	for _, address := range skel.Trans.Addresses {
		if address != MultisigAddress {
			numberOfRequest++
		}
	}

	return uint(fee), numberOfRequest
}

func (b *BTCBroadcastingManager) getLatestBeaconHeight() (uint64, error) {
	params := []interface{}{}
	var beaconBestStateRes entities.BeaconBestStateRes
	err := b.RPCClient.RPCCall("getbeaconbeststate", params, &beaconBestStateRes)
	if err != nil {
		return 0, err
	}

	if beaconBestStateRes.RPCError != nil {
		b.Logger.Errorf("getLatestBeaconHeight: call RPC error, %v\n", beaconBestStateRes.RPCError.StackTrace)
		return 0, errors.New(beaconBestStateRes.RPCError.Message)
	}
	return beaconBestStateRes.Result.BeaconHeight, nil
}

func (b *BTCBroadcastingManager) getLatestBTCBlockHashFromIncog() (uint64, error) {
	params := []interface{}{}
	var btcRelayingBestStateRes entities.BTCRelayingBestStateRes
	err := b.RPCClient.RPCCall("getbtcrelayingbeststate", params, &btcRelayingBestStateRes)
	if err != nil {
		return 0, err
	}
	if btcRelayingBestStateRes.RPCError != nil {
		b.Logger.Errorf("getLatestBTCBlockHashFromIncog: call RPC error, %v\n", btcRelayingBestStateRes.RPCError.StackTrace)
		return 0, errors.New(btcRelayingBestStateRes.RPCError.Message)
	}

	// check whether there was a fork happened or not
	btcBestState := btcRelayingBestStateRes.Result
	if btcBestState == nil {
		return 0, errors.New("BTC relaying best state is nil")
	}
	currentBTCBlkHeight := btcBestState.Height
	return uint64(currentBTCBlkHeight), nil
}

func (b *BTCBroadcastingManager) getBroadcastTxsFromBeaconHeight(processedBatchIDs map[string]bool, height uint64, curIncBlkHeight uint64) ([]*BroadcastTx, error) {
	txArray := []*BroadcastTx{}
	var params []interface{}

	params = []interface{}{
		map[string]string{
			"BeaconHeight": strconv.FormatUint(height, 10),
		},
	}
	var portalStateRes entities.PortalV4StateByHeightRes
	err := b.RPCClient.RPCCall("getportalv4state", params, &portalStateRes)
	if err != nil {
		return txArray, err
	}
	if portalStateRes.RPCError != nil {
		b.Logger.Errorf("getportalv4state: call RPC error, %v\n", portalStateRes.RPCError.StackTrace)
		return txArray, errors.New(portalStateRes.RPCError.Message)
	}

	for _, batch := range portalStateRes.Result.ProcessedUnshieldRequests[BTCID] {
		_, exists := processedBatchIDs[batch.BatchID]
		if !exists {
			params = []interface{}{
				map[string]string{
					"BatchID": batch.BatchID,
				},
			}
			var signedRawTxRes entities.SignedRawTxRes
			err := b.RPCClient.RPCCall("getporalsignedrawtransaction", params, &signedRawTxRes)
			if err != nil {
				return txArray, err
			}
			if signedRawTxRes.RPCError != nil {
				b.Logger.Errorf("getporalsignedrawtransaction: call RPC error, %v\n", signedRawTxRes.RPCError.StackTrace)
				return txArray, errors.New(signedRawTxRes.RPCError.Message)
			}

			btcTxContent := signedRawTxRes.Result.SignedTx
			btcTx, err := b.bcy.DecodeTX(btcTxContent)
			if err != nil {
				return txArray, err
			}
			btcTxHash := btcTx.Trans.Hash
			feePerRequest, numberRequest := b.getUnshieldTxInfo(btcTxContent)
			txArray = append(txArray, &BroadcastTx{
				TxContent:     btcTxContent,
				TxHash:        btcTxHash,
				BatchID:       batch.BatchID,
				FeePerRequest: feePerRequest,
				NumOfRequests: numberRequest,
				BlkHeight:     curIncBlkHeight,
			})
		}
	}
	return txArray, nil
}

func (b *BTCBroadcastingManager) getBroadcastReplacementTx(feeReplacementTxArray []*FeeReplacementTx, curIncBlkHeight uint64) ([]*FeeReplacementTx, []*BroadcastTx, error) {
	var params []interface{}
	idx := 0
	lenArray := len(feeReplacementTxArray)
	txArray := []*BroadcastTx{}

	for idx < lenArray {
		tx := feeReplacementTxArray[idx]
		params = []interface{}{
			map[string]string{
				"TxID": tx.ReqTxID,
			},
		}
		var signedRawTxRes entities.SignedRawTxRes
		err := b.RPCClient.RPCCall("getporalsignedrawreplacefeetransaction", params, &signedRawTxRes)
		if err != nil {
			return feeReplacementTxArray, txArray, err
		}
		if signedRawTxRes.RPCError == nil {
			btcTxContent := signedRawTxRes.Result.SignedTx
			btcTx, err := b.bcy.DecodeTX(btcTxContent)
			if err != nil {
				return feeReplacementTxArray, txArray, err
			}
			btcTxHash := btcTx.Trans.Hash
			feePerRequest, numberRequest := b.getUnshieldTxInfo(btcTxContent)
			txArray = append(txArray, &BroadcastTx{
				TxContent:     btcTxContent,
				TxHash:        btcTxHash,
				BatchID:       tx.BatchID,
				FeePerRequest: feePerRequest,
				NumOfRequests: numberRequest,
				BlkHeight:     curIncBlkHeight,
			})
			idx++
		} else {
			feeReplacementTxArray[lenArray-1], feeReplacementTxArray[idx] = feeReplacementTxArray[idx], feeReplacementTxArray[lenArray-1]
			lenArray--
		}
	}
	feeReplacementTxArray = feeReplacementTxArray[:lenArray]

	return feeReplacementTxArray, txArray, nil
}

func (b *BTCBroadcastingManager) buildProof(txID string, blkHeight uint64) (string, error) {
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

func (b *BTCBroadcastingManager) submitConfirmedTx(proof string, batchID string) (string, error) {
	rpcClient, keyWallet, err := initSetParams(b.RPCClient)
	if err != nil {
		return "", err
	}
	meta, _ := metadata.NewPortalSubmitConfirmedTxRequest(PortalSubmitConfirmedTxMeta, proof, BTCID, batchID)
	var meta2 metadata2.Metadata
	var metaIf interface{}
	metaIf = meta
	meta2 = metaIf.(metadata2.Metadata)
	return sendTx(rpcClient, keyWallet, meta2)
}

func (b *BTCBroadcastingManager) requestFeeReplacement(batchID string, newFee uint) (string, error) {
	rpcClient, keyWallet, err := initSetParams(b.RPCClient)
	if err != nil {
		return "", err
	}
	paymentAddrStr := keyWallet.Base58CheckSerialize(wallet.PaymentAddressType)
	meta, _ := metadata.NewPortalReplacementFeeRequest(PortalReplacementFeeRequestMeta, paymentAddrStr, BTCID, batchID, newFee)
	var meta2 metadata2.Metadata
	var metaIf interface{}
	metaIf = meta
	meta2 = metaIf.(metadata2.Metadata)
	return sendTx(rpcClient, keyWallet, meta2)
}
