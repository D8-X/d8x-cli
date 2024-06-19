package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateReferralSettingsBrokerPayoutAddress(t *testing.T) {
	refSettings := &[]map[string]any{
		{"chainId": float64(400), "brokerPayoutAddr": "0x123400"},
		{"chainId": float64(401), "brokerPayoutAddr": "0x123401"},
	}
	err := UpdateReferralSettingsBrokerPayoutAddress("0xnew-payout-address", 401)(refSettings)
	assert.NoError(t, err)
	assert.Equal(t, "0xnew-payout-address", (*refSettings)[1]["brokerPayoutAddr"])
}

func TestUpdateCandlesPriceConfigPriceServices(t *testing.T) {
	pricesConf := &map[string]any{
		"priceServiceHTTPSEndpoints": []string{},
	}
	err := UpdateCandlesPriceConfigPriceServices(
		[]string{"new-http-endpoint-service", "new-http-endpoint-service1", "new-http-endpoint-service12"},
	)(pricesConf)
	assert.NoError(t, err)
	assert.Equal(t, []string{"new-http-endpoint-service", "new-http-endpoint-service1", "new-http-endpoint-service12"}, (*pricesConf)["priceServiceHTTPSEndpoints"])
}
