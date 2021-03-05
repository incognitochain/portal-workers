package utils

import (
	"encoding/base64"
	"encoding/json"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

const BTCBlockConfirmations = 6

type MerkleProof struct {
	ProofHash *chainhash.Hash
	IsLeft    bool
}

type BTCProof struct {
	MerkleProofs []*MerkleProof
	BTCTx        *wire.MsgTx
	BlockHash    *chainhash.Hash
}

func ParseBTCProofFromB64EncodeStr(b64EncodedStr string) (*BTCProof, error) {
	jsonBytes, err := base64.StdEncoding.DecodeString(b64EncodedStr)
	if err != nil {
		return nil, err
	}
	var proof BTCProof
	err = json.Unmarshal(jsonBytes, &proof)
	if err != nil {
		return nil, err
	}
	return &proof, nil
}

func buildMerkleTreeStoreFromTxHashes(txHashes []*chainhash.Hash) []*chainhash.Hash {
	nextPoT := nextPowerOfTwo(len(txHashes))
	arraySize := nextPoT*2 - 1
	merkles := make([]*chainhash.Hash, arraySize)

	for i, txHash := range txHashes {
		merkles[i] = txHash
	}

	offset := nextPoT
	for i := 0; i < arraySize-1; i += 2 {
		switch {
		case merkles[i] == nil:
			merkles[offset] = nil

		case merkles[i+1] == nil:
			newHash := HashMerkleBranches(merkles[i], merkles[i])
			merkles[offset] = newHash

		default:
			newHash := HashMerkleBranches(merkles[i], merkles[i+1])
			merkles[offset] = newHash
		}
		offset++
	}

	return merkles
}

func BuildMerkleProof(txHashes []*chainhash.Hash, targetedTxHash *chainhash.Hash) []*MerkleProof {
	merkleTree := buildMerkleTreeStoreFromTxHashes(txHashes)
	nextPoT := nextPowerOfTwo(len(txHashes))
	layers := [][]*chainhash.Hash{}
	left := 0
	right := nextPoT
	for left < right {
		layers = append(layers, merkleTree[left:right])
		curLen := len(merkleTree[left:right])
		left = right
		right = right + curLen/2
	}

	merkleProofs := []*MerkleProof{}
	curHash := targetedTxHash
	for _, layer := range layers {
		if len(layer) == 1 {
			break
		}

		for i := 0; i < len(layer); i++ {
			if layer[i] == nil || layer[i].String() != curHash.String() {
				continue
			}
			if i%2 == 0 {
				if layer[i+1] == nil {
					curHash = HashMerkleBranches(layer[i], layer[i])
					merkleProofs = append(
						merkleProofs,
						&MerkleProof{
							ProofHash: layer[i],
							IsLeft:    false,
						},
					)
				} else {
					curHash = HashMerkleBranches(layer[i], layer[i+1])
					merkleProofs = append(
						merkleProofs,
						&MerkleProof{
							ProofHash: layer[i+1],
							IsLeft:    false,
						},
					)
				}
			} else {
				if layer[i-1] == nil {
					curHash = HashMerkleBranches(layer[i], layer[i])
					merkleProofs = append(
						merkleProofs,
						&MerkleProof{
							ProofHash: layer[i],
							IsLeft:    true,
						},
					)
				} else {
					curHash = HashMerkleBranches(layer[i-1], layer[i])
					merkleProofs = append(
						merkleProofs,
						&MerkleProof{
							ProofHash: layer[i-1],
							IsLeft:    true,
						},
					)
				}
			}
			break // process next layer
		}
	}
	return merkleProofs
}
