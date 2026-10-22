package actions

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func PrivateKeyToAddress(pk string) (common.Address, error) {
	pkecdsa, err := crypto.HexToECDSA(pk)
	if err != nil {
		return common.Address{}, err
	}

	return crypto.PubkeyToAddress(pkecdsa.PublicKey), nil
}
