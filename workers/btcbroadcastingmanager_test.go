package workers

import (
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/incognitochain/portal-workers/utils"
)

func TestBuildProof(t *testing.T) {

	b := &BTCBroadcastingManager{}
	// init bitcoin rpcclient
	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", "51.161.119.66", "18443"),
		User:         "thach",
		Pass:         "deptrai",
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}

	var err error
	b.btcClient, err = rpcclient.New(connCfg, nil)
	if err != nil {
		t.FailNow()
	}

	var txHash string
	var btcBlockHeight uint64
	var btcProof string

	txHash = "9fbfc05bc9359544ff1925ea89812ed81f38353af13f83cd34439f83769c6ba4"
	btcBlockHeight = 1937581
	btcProof, err = utils.BuildProof(b.btcClient, txHash, btcBlockHeight)
	if err != nil {
		t.Logf("Gen proof failed - with err: %v", err)
		return
	}
	t.Logf("Proof: %+v", btcProof)
}

func TestEstimateFee(t *testing.T) {

	b := &BTCBroadcastingManager{}
	// init bitcoin rpcclient
	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", "51.161.119.66", "18443"),
		User:         "thach",
		Pass:         "deptrai",
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}

	var err error
	b.btcClient, err = rpcclient.New(connCfg, nil)
	if err != nil {
		t.FailNow()
	}

	mode := btcjson.EstimateSmartFeeMode("CONSERVATIVE")
	result, err := b.btcClient.EstimateSmartFee(1, &mode)
	if err != nil {
		fmt.Printf("Error: %+v\n", err)
		t.Fail()
	}
	feePerVbyte := *result.FeeRate * 1e8 / 1024
	fmt.Printf("Fee/VByte: %+v\n", feePerVbyte)
}

func TestGetVSizeTx(t *testing.T) {
	txContent := "01000000000102dce82c0bdcc50f462a120a07ce3d4b9ec6f2643cf20c4e5aa97aa120457a55da000000000080020000b39208d5878afac07bc2e773427b68a128cde334ba389b0a798f1454c48760e80000000000800200000330170000000000001976a91482fcba9508be122432f7bb90da5f740140a65b8888ac30170000000000001976a91482fcba9508be122432f7bb90da5f740140a65b8888ac401f0000000000002200204a0576b5d99e6d6a7a30f87606f1636d347cc8e5b47dfdc8758af659defcff1f0500483045022100daa70309f498289a01e1fb62f1522366957a1fd5784ed2c231ead75b7ec6803602204b275ad469c20417e27c24089c54c94ed59de9616d70c7df957d32531f576ae70147304402203f41b3077a9fa64a8666bdd6348e333ee2d62870e2f004ee77c203a971d1a9f402205be009a2795c263913b9cf6aed6c430b57259b6ff70dafc63d5264824f4d13ba01473044022055a86132137a26ec70a52bef9b094ce9f5b84c3ab70135cb46489687b834329002207f3fabce516062ff105cbd60b7b81389ff980cc9771eca500a95e1d43d935316018b532103b2d3167d949c2503e69c9f29787d9c088d39178db4754035f5ae6af0171211002103987a87d19913bde3eff0557902b49057ed1c9c8b32f902bbbb85713a991fdc41210373235eb1c8f184e759176ce38737b79119471bba6356bcab8dcc144b42998601210329e7593189ca7af601b635673db153d419d70619032a32945776b2b38065e15d54ae05004730440220761553bcd7807e87b06a2d63b49b60e4dccea4a9c4c18c7782bad8247f1f0d8f0220632f34df797fcc3d89d6b59cc54b92dd6790d696f0eab4bb2ad587f208a888a90148304502210099a93555a98db091186c61c2ea06175985da2ea5d9bb89aad1f28273c39b434602200af853c903176cc540f312693e5bae7010163f5dbbedf7cf785771e3eaf1aa1f01473044022028af31b8a97da2b26e1f815599cd7a255a7c1ded87c66789fd4395dfe4806ef102201324b6038f36728649cba73f3bf0b928e770539ce9ffd96ed6bcec54a4bf5950018b532103b2d3167d949c2503e69c9f29787d9c088d39178db4754035f5ae6af0171211002103987a87d19913bde3eff0557902b49057ed1c9c8b32f902bbbb85713a991fdc41210373235eb1c8f184e759176ce38737b79119471bba6356bcab8dcc144b42998601210329e7593189ca7af601b635673db153d419d70619032a32945776b2b38065e15d54ae00000000"

	b := &BTCBroadcastingManager{}
	// init bitcoin rpcclient
	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", "51.161.119.66", "18443"),
		User:         "thach",
		Pass:         "deptrai",
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}

	var err error
	b.btcClient, err = rpcclient.New(connCfg, nil)
	if err != nil {
		t.FailNow()
	}

	size, err := b.getVSizeBTCTx(txContent)
	if err != nil {
		t.FailNow()
	}

	fmt.Printf("Size: %v, VSize: %v\n", len(txContent)/2, size)
}
