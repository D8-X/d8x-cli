package actions

import (
	"regexp"

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

func ValidWalletAddress(address string) bool {
	return regexp.MustCompile("^0x[0-9a-fA-F]{40}$").MatchString(address)
}
