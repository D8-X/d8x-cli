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

func TestSetRpcEntry(t *testing.T) {
	emptyWs := []string{}
	someWs := []string{"wss://a", "wss://b"}

	tests := []struct {
		name         string
		entries      []RPCConfigEntry
		chainId      uint
		httpRpcs     []string
		wsRpcs       []string
		wantLen      int
		wantWsNil    bool
		wantHttp     []string
		wantWsValues []string
	}{
		{
			name:      "creates entry when chain missing, no ws",
			entries:   []RPCConfigEntry{{ChainId: 1, HttpRpcs: []string{"x"}}},
			chainId:   42,
			httpRpcs:  []string{"http://new"},
			wsRpcs:    nil,
			wantLen:   2,
			wantWsNil: true,
			wantHttp:  []string{"http://new"},
		},
		{
			name:         "creates entry with ws when chain missing and ws non-empty",
			entries:      []RPCConfigEntry{{ChainId: 1, HttpRpcs: []string{"x"}}},
			chainId:      42,
			httpRpcs:     []string{"http://new"},
			wsRpcs:       someWs,
			wantLen:      2,
			wantWsNil:    false,
			wantHttp:     []string{"http://new"},
			wantWsValues: someWs,
		},
		{
			name:         "updates existing entry that had nil ws, keeps nil when ws empty",
			entries:      []RPCConfigEntry{{ChainId: 42, HttpRpcs: []string{"old"}}},
			chainId:      42,
			httpRpcs:     []string{"http://new"},
			wsRpcs:       emptyWs,
			wantLen:      1,
			wantWsNil:    true,
			wantHttp:     []string{"http://new"},
			wantWsValues: nil,
		},
		{
			name:         "updates existing entry that had ws, reflects new ws exactly",
			entries:      []RPCConfigEntry{{ChainId: 42, HttpRpcs: []string{"old"}, WsRpcs: &someWs}},
			chainId:      42,
			httpRpcs:     []string{"http://new"},
			wsRpcs:       emptyWs,
			wantLen:      1,
			wantWsNil:    false,
			wantHttp:     []string{"http://new"},
			wantWsValues: []string{},
		},
		{
			name:         "promotes nil ws to slice when given non-empty ws",
			entries:      []RPCConfigEntry{{ChainId: 42, HttpRpcs: []string{"old"}}},
			chainId:      42,
			httpRpcs:     []string{"http://new"},
			wsRpcs:       someWs,
			wantLen:      1,
			wantWsNil:    false,
			wantHttp:     []string{"http://new"},
			wantWsValues: someWs,
		},
		{
			name:      "leaves other chain entries untouched",
			entries:   []RPCConfigEntry{{ChainId: 1, HttpRpcs: []string{"keep"}}, {ChainId: 42, HttpRpcs: []string{"old"}}},
			chainId:   42,
			httpRpcs:  []string{"new"},
			wsRpcs:    nil,
			wantLen:   2,
			wantWsNil: true,
			wantHttp:  []string{"new"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setRpcEntry(tt.entries, tt.chainId, tt.httpRpcs, tt.wsRpcs)
			assert.Equal(t, len(got), tt.wantLen)

			var target *RPCConfigEntry
			for i := range got {
				if got[i].ChainId == tt.chainId {
					target = &got[i]
					break
				}
			}
			if target == nil {
				t.Fatalf("chain %d not found in result", tt.chainId)
			}
			assert.Equal(t, target.HttpRpcs, tt.wantHttp)
			if tt.wantWsNil {
				if target.WsRpcs != nil {
					t.Fatalf("expected WsRpcs nil, got %v", *target.WsRpcs)
				}
			} else {
				if target.WsRpcs == nil {
					t.Fatalf("expected WsRpcs non-nil, got nil")
				}
				assert.Equal(t, *target.WsRpcs, tt.wantWsValues)
			}
		})
	}
}
