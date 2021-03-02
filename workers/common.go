package workers

import (
	"fmt"
	"os"

	"github.com/0xkraken/incognito-sdk-golang/crypto"
	"github.com/0xkraken/incognito-sdk-golang/metadata"
	"github.com/0xkraken/incognito-sdk-golang/rpcclient"
	"github.com/0xkraken/incognito-sdk-golang/transaction"
	"github.com/0xkraken/incognito-sdk-golang/wallet"
	"github.com/incognitochain/portal-workers/utils"
)

func initSetParams(curRpcClient *utils.HttpClient) (*rpcclient.HttpClient, *wallet.KeyWallet, error) {
	rpcClient := rpcclient.NewHttpClient(curRpcClient.GetURL(), "", "", 0)

	// create sender private key from private key string
	keyWallet, err := wallet.Base58CheckDeserialize(os.Getenv("INCOGNITO_PRIVATE_KEY"))
	if err != nil {
		return nil, nil, fmt.Errorf("Can not deserialize private key %v\n", err)
	}
	err = keyWallet.KeySet.InitFromPrivateKey(&keyWallet.KeySet.PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("sender private key is invalid")
	}

	return rpcClient, keyWallet, nil
}

func sendTx(rpcClient *rpcclient.HttpClient, keyWallet *wallet.KeyWallet, meta metadata.Metadata) (string, error) {
	// create tx
	tx := new(transaction.Tx)
	tx, err := tx.Init(
		rpcClient, keyWallet, []*crypto.PaymentInfo{}, DefaultFee, false, meta, nil, 1)
	if err != nil {
		return "", err
	}

	// send tx
	txID, err := tx.Send(rpcClient)
	if err != nil {
		tx.UnCacheUTXOs(keyWallet.KeySet.PaymentAddress.Pk)
		return "", err
	}

	tx.UpdateCacheUTXOsWithTxID(keyWallet.KeySet.PaymentAddress.Pk, tx.Proof.GetInputCoins())

	return txID, nil
}

func SplitUTXOs(
	client *utils.HttpClient,
	privateKeyStr string,
	numUTXOs int,
) error {
	rpcClient := rpcclient.NewHttpClient(client.GetURL(), "", "", 0)

	err := transaction.SplitUTXOs(rpcClient, privateKeyStr, numUTXOs, DefaultFee)
	if err != nil {
		return err
	}

	return nil
}
