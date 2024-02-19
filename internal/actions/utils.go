package actions

import (
	"regexp"
	"slices"

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

// UniqStrings returns a new slice with unique strings and sorts the result.
func UniqStrings(slice []string) []string {
	uniq := make(map[string]struct{})
	for _, s := range slice {
		uniq[s] = struct{}{}
	}

	var result []string
	for s := range uniq {
		result = append(result, s)
	}

	slices.Sort(result)

	return result
}
