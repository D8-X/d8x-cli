package actions

import (
	"testing"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/magiconair/properties/assert"
)

func TestDistributeRpcs(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *configs.D8XConfig
		chainId      string
		serviceIndex int
		wantHttp     []string
		wantWss      []string
	}{
		{
			name: "service 0 one http ok",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1"},
				},
			},
			chainId:      "1442",
			wantHttp:     []string{"http-rpc-1"},
			wantWss:      []string{},
			serviceIndex: 0,
		},
		{
			name: "service 1 one http ok",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1"},
				},
			},
			chainId:      "1442",
			wantHttp:     []string{"http-rpc-1"},
			wantWss:      []string{},
			serviceIndex: 1,
		},
		{
			name: "service 2 one http ok",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1"},
				},
			},
			chainId:      "1442",
			wantHttp:     []string{"http-rpc-1"},
			wantWss:      []string{},
			serviceIndex: 2,
		},
		{
			name: "service 3 one http ok",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1"},
				},
				BrokerDeployed: true,
			},
			chainId:      "1442",
			wantHttp:     []string{"http-rpc-1"},
			wantWss:      []string{},
			serviceIndex: 3,
		},
		{
			name: "service 3 (rpcs less than services) http ok",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3"},
				},
				BrokerDeployed: true,
			},
			chainId:      "1442",
			wantHttp:     []string{"http-rpc-2"},
			wantWss:      []string{},
			serviceIndex: 3,
		},
		{
			name: "service 2 (rpcs less than services) http ok",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3"},
				},
				BrokerDeployed: true,
			},
			chainId:      "1442",
			wantHttp:     []string{"http-rpc-3"},
			wantWss:      []string{},
			serviceIndex: 2,
		},
		{
			name: "service 0 http/wss ok #1",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3"},
				},
				WsRpcList: map[string][]string{
					"1442": {"ws-rpc-1", "ws-rpc-2", "ws-rpc-3"},
				},
				BrokerDeployed: true,
			},
			chainId:      "1442",
			wantHttp:     []string{"http-rpc-1"},
			wantWss:      []string{"ws-rpc-1", "ws-rpc-3"},
			serviceIndex: 0,
		},
		{
			name: "service 1 http/wss ok #1",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3"},
				},
				WsRpcList: map[string][]string{
					"1442": {"ws-rpc-1", "ws-rpc-2", "ws-rpc-3"},
				},
				BrokerDeployed: true,
			},
			chainId:      "1442",
			wantHttp:     []string{"http-rpc-2"},
			wantWss:      []string{"ws-rpc-2"},
			serviceIndex: 1,
		},
		{
			name: "service 2 http/wss ok #1",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3"},
				},
				WsRpcList: map[string][]string{
					"1442": {"ws-rpc-1", "ws-rpc-2", "ws-rpc-3"},
				},
				BrokerDeployed: true,
			},
			chainId:      "1442",
			wantHttp:     []string{"http-rpc-3"},
			wantWss:      []string{},
			serviceIndex: 2,
		},
		{
			name: "service 1 http/wss ok #1",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3", "h-r-4", "h-r-5", "h-r-6"},
				},
				WsRpcList: map[string][]string{
					"1442": {"ws-rpc-1", "ws-rpc-2", "ws-rpc-3", "w4", "w5"},
				},
				// Make only 3 http services
				BrokerDeployed: false,
			},
			chainId: "1442",
			wantHttp: []string{
				"http-rpc-2",
				"h-r-5",
			},
			wantWss: []string{
				"ws-rpc-2",
				"w4",
			},
			serviceIndex: 1,
		},
		{
			name: "broker service 3 http/wss ok #1",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3", "h-r-4", "h-r-5", "h-r-6", "h7", "h8", "h9"},
				},
				WsRpcList: map[string][]string{
					"1442": {"ws-rpc-1", "ws-rpc-2", "ws-rpc-3", "w4", "w5"},
				},
				BrokerDeployed: true,
			},
			chainId: "1442",
			wantHttp: []string{
				"h-r-4",
				"h8",
			},
			wantWss:      []string{},
			serviceIndex: 3,
		},
		{
			name: "history service 1 http/wss ok full",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3", "h-r-4", "h-r-5", "h-r-6", "h7", "h8", "h9", "h10"},
				},
				WsRpcList: map[string][]string{
					"1442": {"ws-rpc-1", "ws-rpc-2", "ws-rpc-3", "w4", "w5"},
				},
				BrokerDeployed: true,
			},
			chainId: "1442",
			wantHttp: []string{
				"http-rpc-2",
				"h-r-6",
				"h10",
			},
			wantWss:      []string{"ws-rpc-2", "w4"},
			serviceIndex: 1,
		},
		{
			name: "main service 0 http/wss ok full",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3", "h-r-4", "h-r-5", "h-r-6", "h7", "h8", "h9", "h10"},
				},
				WsRpcList: map[string][]string{
					"1442": {"ws-rpc-1", "ws-rpc-2", "ws-rpc-3", "w4", "w5"},
				},
				BrokerDeployed: true,
			},
			chainId: "1442",
			wantHttp: []string{
				"http-rpc-1",
				"h-r-5",
				"h9",
			},
			wantWss:      []string{"ws-rpc-1", "ws-rpc-3", "w5"},
			serviceIndex: 0,
		},
		{
			name: "main service 0 http full wss 1 ok",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3", "h-r-4", "h-r-5", "h-r-6", "h7", "h8", "h9", "h10"},
				},
				WsRpcList: map[string][]string{
					"1442": {"ws-rpc-1"},
				},
				BrokerDeployed: true,
			},
			chainId: "1442",
			wantHttp: []string{
				"http-rpc-1",
				"h-r-5",
				"h9",
			},
			wantWss:      []string{"ws-rpc-1"},
			serviceIndex: 0,
		},
		{
			name: "hisotry service 1 http full wss 1 ok",
			cfg: &configs.D8XConfig{
				HttpRpcList: map[string][]string{
					"1442": {"http-rpc-1", "http-rpc-2", "http-rpc-3", "h-r-4", "h-r-5", "h-r-6", "h7", "h8", "h9", "h10"},
				},
				WsRpcList: map[string][]string{
					"1442": {"ws-rpc-1"},
				},
				BrokerDeployed: true,
			},
			chainId: "1442",
			wantHttp: []string{
				"http-rpc-2",
				"h-r-6",
				"h10",
			},
			wantWss:      []string{"ws-rpc-1"},
			serviceIndex: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHttp, gotWss := DistributeRpcs(tt.serviceIndex, tt.chainId, tt.cfg)
			assert.Equal(t, gotHttp, tt.wantHttp)
			assert.Equal(t, gotWss, tt.wantWss)
		})
	}
}
