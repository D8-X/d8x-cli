package configs

import "github.com/ethereum/go-ethereum/common"

type ReferralSettingConfig struct {
	ChainId                int    `json:"chainId"`
	PaymentMaxLookBackDays int    `json:"paymentMaxLookBackDays"`
	PayCronSchedule        string `json:"paymentScheduleCron"`
	TokenX                 struct {
		Address  string `json:"address"`
		Decimals uint8  `json:"decimals"`
	} `json:"tokenX"`
	ReferrerCut      [][]float64    `json:"referrerCutPercentForTokenXHolding"`
	BrokerPayoutAddr common.Address `json:"brokerPayoutAddr"`
	BrokerId         string         `json:"brokerId"`
}
