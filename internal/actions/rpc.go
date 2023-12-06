package actions

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
)

type ChainJsonEntry struct {
	SDKNetwork               string `json:"sdkNetwork"`
	PriceFeedNetwork         string `json:"priceFeedNetwork"`
	DefaultPythWSEndpoint    string `json:"priceServiceWSEndpoint"`
	DefaultPythHTTPSEndpoint string `json:"priceServiceHTTPSEndpoint"`
	// Chain type is either testnet or mainnet
	Type string `json:"type"`
}

// trader-backend/chain.json structure. This must always contain "default" key
// with values.
type ChainJson map[string]ChainJsonEntry

type rpcTransport string

func (r rpcTransport) SecureProtocolPrefix() string {
	return string(r) + "s://"
}
func (r rpcTransport) ProtocolPrefix() string {
	return string(r) + "://"
}

const (
	rpcTransportHTTP rpcTransport = "http"
	rpcTransportWS   rpcTransport = "ws"
)

// RPCUrlCollector collects RPC urls from the user. Slice suggestions is a list
// of RPC suggestions which will be added as initial value to the input field.
func (c *InputCollector) RPCUrlCollector(protocol rpcTransport, chainId string, requireAtLeast, recommended int, suggestions []string) ([]string, error) {
	transportUpper := strings.ToUpper(string(protocol))
	endpoints := []string{}

	validate := func(endpoint string) error {
		// Ensure prefix is added
		if !strings.HasPrefix(endpoint, protocol.SecureProtocolPrefix()) && !strings.HasPrefix(endpoint, protocol.ProtocolPrefix()) {
			return fmt.Errorf("invalid protocol prefix, should be %s or %s", protocol.SecureProtocolPrefix(), protocol.ProtocolPrefix())
		}

		_, err := url.Parse(endpoint)
		if err != nil {
			return err
		}

		return nil
	}

	for {
		// If suggestions are provided, use them as initial value
		val := ""
		if len(endpoints) < len(suggestions) {
			val = suggestions[len(endpoints)]
		}

		fmt.Printf("Enter %s RPC url #%d for chain id %s\n", transportUpper, len(endpoints)+1, chainId)
		endpoint, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder(protocol.SecureProtocolPrefix()+"your-rpc-provider.com"),
			components.TextInputOptDenyEmpty(),
			components.TextInputOptValue(val),
		)
		if err != nil {
			return nil, err
		}
		endpoint = strings.TrimSpace(endpoint)
		// Disallow empty strings
		if endpoint == "" {
			continue
		}
		// Validate entered endpoint
		if err := validate(endpoint); err != nil {
			fmt.Println(
				styles.ErrorText.Render(
					fmt.Sprintf("Invalid RPC url (%s), please try again...", err.Error()),
				),
			)
			continue
		}
		endpoints = append(endpoints, endpoint)
		if len(endpoints) >= requireAtLeast {
			recommendedText := "We recommend having at least " + strconv.Itoa(recommended) + " RPCs. "
			if len(endpoints) >= recommended {
				recommendedText = ""
			}
			ok, err := c.TUI.NewPrompt(
				fmt.Sprintf("%sAdd another one?", recommendedText),
				true,
			)
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
		}
	}
	return endpoints, nil
}

// CollectHTTPRPCUrls collects http rpc urls and writes them into the config
// file
func (c *InputCollector) CollectHTTPRPCUrls(cfg *configs.D8XConfig, chainId string) error {
	collectHttpRPCS := true
	httpRpcs, exists := cfg.HttpRpcList[chainId]
	if exists {
		fmt.Printf("The following HTTP RPC urls were found: \n%s \n", strings.Join(httpRpcs, "\n"))
		keep, err := c.TUI.NewPrompt("Do you want to keep these HTTP RPC urls?", true)
		if err != nil {
			return err
		}
		if keep {
			collectHttpRPCS = false
		}
	}
	if collectHttpRPCS {
		httpRpcs, err := c.RPCUrlCollector(rpcTransportHTTP, chainId, 3, 5, []string{})
		if err != nil {
			return err
		}
		cfg.HttpRpcList[chainId] = slices.Compact(httpRpcs)
	}

	return c.ConfigRWriter.Write(cfg)
}

// CollectWebsocketRPCUrls collects websocket rpc urls and writes them into the
// config file
func (c *InputCollector) CollectWebsocketRPCUrls(cfg *configs.D8XConfig, chainId string) error {
	collectWSRPCS := true
	wspRpcs, exists := cfg.WsRpcList[chainId]
	if exists {
		fmt.Printf("The following Websocket RPC urls were found: \n%s \n", strings.Join(wspRpcs, "\n"))
		keep, err := c.TUI.NewPrompt("Do you want to keep these Websocket RPC urls?", true)
		if err != nil {
			return err
		}
		if keep {
			collectWSRPCS = false
		}
	}

	// When http rpcs are provided - make a suggestion list for websocket rpcs
	// by changing http(s):// to wss:// prefix
	suggestions := []string{}
	if httpRpcs, ok := cfg.HttpRpcList[chainId]; ok {
		for _, rpc := range httpRpcs {
			suggestions = append(suggestions,
				"wss://"+strings.TrimPrefix(
					strings.TrimPrefix(rpc, "http://"),
					"https://",
				),
			)
		}
	}

	if collectWSRPCS {
		wsRpcs, err := c.RPCUrlCollector(rpcTransportWS, chainId, 1, 2, suggestions)
		if err != nil {
			return err
		}
		cfg.WsRpcList[chainId] = slices.Compact(wsRpcs)
	}
	return c.ConfigRWriter.Write(cfg)
}

// LoadChainJson loads the chain.json file contents from the embedded configs
func LoadChainJson() (ChainJson, error) {
	chainJson := ChainJson{}

	contents, err := configs.EmbededConfigs.ReadFile("embedded/trader-backend/chain.json")
	if err != nil {
		return chainJson, err
	}

	if err := json.Unmarshal(contents, &chainJson); err != nil {
		return chainJson, fmt.Errorf("unmarshalling chain.json: %w", err)
	}

	return chainJson, err
}

// LoadChainJson loads and caches ChainJson data
func (c *Container) LoadChainJson() (ChainJson, error) {
	if c.cachedChainJson == nil {
		chjs, err := LoadChainJson()
		if err != nil {
			return nil, err
		}
		c.cachedChainJson = chjs
	}

	return c.cachedChainJson, nil
}

// getChainSDKName retrieves the SDK compatible SDK_CONFIG_NAME
func (c ChainJson) getChainSDKName(chainId string) string {
	entry, exists := c[chainId]
	if !exists {
		return c["default"].SDKNetwork
	}
	return entry.SDKNetwork
}

// getChainPriceFeedName retrieves the python compatible NETWORK_NAME
func (c ChainJson) getChainPriceFeedName(chainId string) string {
	entry, exists := c[chainId]
	if !exists {
		return c["default"].PriceFeedNetwork
	}
	return entry.PriceFeedNetwork
}

// getDefaultPythWSEndpoint retrieves the default pyth websocket endpoint from
// chain.json config
func (c ChainJson) getDefaultPythWSEndpoint(chainId string) string {
	entry, exists := c[chainId]
	if !exists {
		return c["default"].DefaultPythWSEndpoint
	}
	return entry.DefaultPythWSEndpoint
}

// getDefaultPythHTTPSEndpoint retrieves the default pyth https endpoint from
// chain.json config
func (c *Container) getDefaultPythHTTPSEndpoint(chainId string) string {
	c.LoadChainJson()

	chainJson, exists := c.cachedChainJson[chainId]
	if !exists {
		return c.cachedChainJson["default"].DefaultPythHTTPSEndpoint
	}
	return chainJson.DefaultPythHTTPSEndpoint
}

func (c ChainJson) GetChainType(chainId string) string {
	entry, exists := c[chainId]
	if !exists {
		return c["default"].Type
	}
	return entry.Type
}

type WSRPCSlice struct {
	Entries     []string
	OmitIfEmpty bool
}

type RPCConfigEntry struct {
	ChainId  uint     `json:"chainId"`
	HttpRpcs []string `json:"HTTP"`
	// WsRpcs is optional and can be a nil, in that case we will omit it.
	// However, if it is an empty slice, we want to include empty array.
	WsRpcs *[]string `json:"WS,omitempty"`
}

// editRpcConfigUrls updates rpc config file for given chainId with wsRpcs and
// httpRpcs. Any pre-existing rpc urls are kept. By default these will be the
// public rpc urls (in embedded configs). When wsRpcs is nil, WS field will be
// omitted, however, when it is empty slice - it will be included as empty array
// in json output.
func (c *Container) editRpcConfigUrls(rpcConfigFilePath string, chainId uint, wsRpcs, httpRpcs []string) error {
	rpcCfg, err := os.ReadFile(rpcConfigFilePath)
	if err != nil {
		return err
	}
	rpcConfig := []RPCConfigEntry{}
	if err := json.Unmarshal(rpcCfg, &rpcConfig); err != nil {
		return err
	}

	// Find and replace our RPC config entry or create it if not found (for
	// given chainId)
	found := false
	newEntry := RPCConfigEntry{
		ChainId:  chainId,
		HttpRpcs: httpRpcs,
	}

	for i, entry := range rpcConfig {
		if entry.ChainId == chainId {
			// Append existing urls to our new entry
			entry.HttpRpcs = slices.Compact(append(entry.HttpRpcs, newEntry.HttpRpcs...))

			// Make sure to remove any pre-existing empty entries
			entry.HttpRpcs = slices.DeleteFunc(entry.HttpRpcs, func(s string) bool {
				return s == ""
			})

			// Only append ws rpcs if they are provided. If ws values are non
			// nil we must create WS field entry if it doesn't exist.
			if wsRpcs != nil {
				if entry.WsRpcs == nil {
					entry.WsRpcs = &[]string{}
				}
				tmp := slices.Compact(append(*entry.WsRpcs, wsRpcs...))
				entry.WsRpcs = &tmp
			}

			if entry.WsRpcs != nil {
				// Make sure to remove any pre-existing empty entries
				*entry.WsRpcs = slices.DeleteFunc(*entry.WsRpcs, func(s string) bool {
					return s == ""
				})
			}

			rpcConfig[i] = entry
			found = true
		}
	}

	// ChainID entry was not found - append it to the output
	if !found {
		if wsRpcs != nil {
			newEntry.WsRpcs = &wsRpcs
		}
		rpcConfig = append(rpcConfig, newEntry)
	}

	marshalled, err := json.MarshalIndent(rpcConfig, "", "\t")
	if err != nil {
		return err
	}

	return c.FS.WriteFile(rpcConfigFilePath, marshalled)
}

// DistributeRpcs distribute rpc from cfg (user supplied rpcs) based on provided
// serviceIndex. RPC distribution is done in a card dealing way (serviceIndex 0
// gets 0, 0 + numServices, 0 + 2*numServices, etc.). We currently support 4
// services which need rpcs: main, history, referral and broker-server
// (optional) with serviceIndex values 0, 1,2 and 3 respectively. It is
// suggested to have at least 4 Http rpcs added (3 without broker). Only
// serviceIndex 0 and 1 gets websockets (main, history). Returned slices are
// http and ws rpcs list.
func DistributeRpcs(serviceIndex int, chainId string, cfg *configs.D8XConfig) ([]string, []string) {
	// Maximum number of serviceIndex for http/ws lists
	// main, history, referral
	httpServices := 3
	// main, history
	wsServices := 2
	if cfg.BrokerDeployed {
		// and broker-server
		httpServices = 4
	}

	httpAvailable := len(cfg.HttpRpcList[chainId])
	wsAvailable := len(cfg.WsRpcList[chainId])

	distributionFunc := func(rpcsAvailable, rpcsServices int, rpcsList []string) []string {
		returnList := []string{}

		// Deny higher serviceIndex than supported via httpServices or
		// wsServices.
		if serviceIndex+1 > rpcsServices {
			return returnList
		}

		// Special case - only single rpc available - always use it
		if rpcsAvailable == 1 {
			returnList = append(returnList, rpcsList[0])
		} else if rpcsAvailable > 0 && rpcsAvailable < rpcsServices {
			// Not enough http rpcs available - make sure serviceIndex 0 gets only
			// 0th and others get everything else in sequence
			rpcIndexToGet := serviceIndex
			if serviceIndex > 0 {
				// Start from 1st slice element and distribute in sequence one
				// by one. Basically we cut the first element out of the
				// equation.
				rpcIndexToGet = 1 + (serviceIndex-1)%(rpcsAvailable-1)
			}

			returnList = []string{rpcsList[rpcIndexToGet]}
		} else if rpcsAvailable >= rpcsServices {
			for i := serviceIndex; i < rpcsAvailable; i += rpcsServices {
				returnList = append(returnList, rpcsList[i])
			}
		}

		return returnList
	}

	httpRpcs := distributionFunc(httpAvailable, httpServices, cfg.HttpRpcList[chainId])
	wsRpcs := distributionFunc(wsAvailable, wsServices, cfg.WsRpcList[chainId])

	return httpRpcs, wsRpcs
}
