package actions

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestProcessNginxConfigComments(t *testing.T) {

	tests := []struct {
		name              string
		inputNginxConfig  io.Reader
		inputNginxSection NginxConfigSection
		wantOutput        string
		wantErr           string
	}{
		{
			name:              "should fail reading input",
			inputNginxConfig:  iotest.ErrReader(errors.New("read error")),
			inputNginxSection: "",
			wantErr:           "read error",
			wantOutput:        "",
		},
		{
			name: "should process one section correctly",
			inputNginxConfig: bytes.NewReader([]byte(`
# some comment	
# another comment
# {thing}
sdfsdf
# this line shall be uncommented
#{/thing}	
			`)),
			inputNginxSection: "thing",
			wantOutput: `
# some comment	
# another comment
# {thing}
sdfsdf
 this line shall be uncommented
#{/thing}	
			
`, // <-- trailing \n
		},
		{
			name: "should not process other sections",
			inputNginxConfig: bytes.NewReader([]byte(`
# some comment	
# another comment
# {thing}
sdfsdf
# this line shall be uncommented
#{/thing}	
			`)),
			inputNginxSection: "not-thing",
			wantOutput: `
# some comment	
# another comment
# {thing}
sdfsdf
# this line shall be uncommented
#{/thing}	
			
`, // <-- trailing \n
		},
		{
			name: "should process sequential sections",
			inputNginxConfig: bytes.NewReader([]byte(`
# some comment	
# another comment
# {thing}
sdfsdf
# this line shall be uncommented
#{/thing}	
blah blah
123123
123
123
123123
123
server root.com
# {thing}
antoher-dir thing;
# this line shall be uncommented
#
#
#
#
#
# comment 
#{/thing}`)),
			inputNginxSection: "thing",
			wantOutput: `
# some comment	
# another comment
# {thing}
sdfsdf
 this line shall be uncommented
#{/thing}	
blah blah
123123
123
123
123123
123
server root.com
# {thing}
antoher-dir thing;
 this line shall be uncommented





 comment 
#{/thing}
`, // <-- trailing \n
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := processNginxConfigComments(tt.inputNginxConfig, tt.inputNginxSection)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
			} else {
				assert.Equal(t, string(tt.wantOutput), string(output))
			}
		})
	}

}
