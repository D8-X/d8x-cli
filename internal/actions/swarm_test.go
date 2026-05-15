package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
