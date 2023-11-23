package actions

import (
	"container/ring"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
)

type ChainJsonEntry struct {
	SDKNetwork            string `json:"sdkNetwork"`
	PriceFeedNetwork      string `json:"priceFeedNetwork"`
	DefaultPythWSEndpoint string `json:"priceServiceWSEndpoint"`
}

// trader-backend/chain.json structure. This must always contain "default" key.
type ChainJson map[string]ChainJsonEntry

// RPCUrlCollector collects RPC urls from the user
func (c *Container) RPCUrlCollector(rpcTransport, chainId string, requireAtLeast, recommended int) ([]string, error) {
	transportUpper := strings.ToUpper(rpcTransport)
	transportLower := strings.ToLower(rpcTransport)
	endpoints := []string{}
	for {
		fmt.Printf("Enter %s RPC url #%d for chain id %s\n", transportUpper, len(endpoints)+1, chainId)
		endpoint, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder(transportLower + "://localhost:8545"),
		)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, strings.TrimSpace(endpoint))
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
func (c *Container) CollectHTTPRPCUrls(cfg *configs.D8XConfig, chainId string) error {
	collectHttpRPCS := true
	httpRpcs, exists := cfg.HttpRpcList[chainId]
	if exists {
		fmt.Printf("The following HTTP RPC urls were found: \n%s \n", strings.Join(httpRpcs, "\n"))
		ok, err := c.TUI.NewPrompt("Do you want to change HTTP RPC urls?", true)
		if err != nil {
			return err
		}
		if !ok {
			collectHttpRPCS = false
		}
	}
	if collectHttpRPCS {
		httpRpcs, err := c.RPCUrlCollector("http", chainId, 3, 5)
		if err != nil {
			return err
		}
		cfg.HttpRpcList[chainId] = slices.Compact(httpRpcs)
	}

	return c.ConfigRWriter.Write(cfg)
}

// CollectWebsocketRPCUrls collects websocket rpc urls and writes them into the
// config file
func (c *Container) CollectWebsocketRPCUrls(cfg *configs.D8XConfig, chainId string) error {
	collectWSRPCS := true
	wspRpcs, exists := cfg.WsRpcList[chainId]
	if exists {
		fmt.Printf("The following Websocket RPC urls were found: \n%s \n", strings.Join(wspRpcs, "\n"))
		ok, err := c.TUI.NewPrompt("Do you want to change Websocket RPC urls?", true)
		if err != nil {
			return err
		}
		if !ok {
			collectWSRPCS = false
		}
	}
	if collectWSRPCS {
		wsRpcs, err := c.RPCUrlCollector("ws", chainId, 1, 2)
		if err != nil {
			return err
		}
		cfg.WsRpcList[chainId] = slices.Compact(wsRpcs)
	}

	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return err
	}

	return c.ConfigRWriter.Write(cfg)
}

// loadChainJson loads the chain.json file from the embedded configs and caches
// it on Container instance.
func (c *Container) loadChainJson() error {
	if c.cachedChainJson == nil {
		contents, err := configs.EmbededConfigs.ReadFile("embedded/trader-backend/chain.json")
		if err != nil {
			return err
		}

		chainJson := ChainJson{}
		if err := json.Unmarshal(contents, &chainJson); err != nil {
			return fmt.Errorf("unmarshalling chain.json: %w", err)
		}

		c.cachedChainJson = chainJson
	}

	return nil
}

// getChainSDKName retrieves the SDK compatible SDK_CONFIG_NAME
func (c *Container) getChainSDKName(chainId string) string {
	c.loadChainJson()

	chainJson, exists := c.cachedChainJson[chainId]
	if !exists {
		return c.cachedChainJson["default"].SDKNetwork
	}
	return chainJson.SDKNetwork
}

// getChainPriceFeedName retrieves the python compatible NETWORK_NAME
func (c *Container) getChainPriceFeedName(chainId string) string {
	c.loadChainJson()

	chainJson, exists := c.cachedChainJson[chainId]
	if !exists {
		return c.cachedChainJson["default"].PriceFeedNetwork
	}
	return chainJson.PriceFeedNetwork
}

// getDefaultPythWSEndpoint retrieves the default pyth websocket endpoint from
// chain.json config
func (c *Container) getDefaultPythWSEndpoint(chainId string) string {
	c.loadChainJson()

	chainJson, exists := c.cachedChainJson[chainId]
	if !exists {
		return c.cachedChainJson["default"].DefaultPythWSEndpoint
	}
	return chainJson.DefaultPythWSEndpoint
}

type RPCConfigEntry struct {
	ChainId  uint     `json:"chainId"`
	HttpRpcs []string `json:"HTTP"`
	// WsRpcs is optional and can be a nil, in that case we will set it to empty
	// slice
	WsRpcs []string `json:"WS"`
}

// editRpcConfigUrls updates rpc config file for given chainId with wsRpcs and
// httpRpcs. Any pre-existing rpc urls are kept. By default these will be the
// public rpc urls.
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
		WsRpcs:   wsRpcs,
		HttpRpcs: httpRpcs,
	}

	for i, entry := range rpcConfig {
		if entry.ChainId == chainId {
			// Append existing urls to our new entry
			entry.HttpRpcs = slices.Compact(append(entry.HttpRpcs, newEntry.HttpRpcs...))

			if newEntry.WsRpcs != nil {
				entry.WsRpcs = slices.Compact(append(entry.WsRpcs, newEntry.WsRpcs...))
			}

			// Prevent nulls in output json
			if entry.WsRpcs == nil {
				entry.WsRpcs = []string{}
			}

			rpcConfig[i] = entry
			found = true
		}
	}

	if !found {
		rpcConfig = append(rpcConfig, newEntry)
	}

	marshalled, err := json.MarshalIndent(rpcConfig, "", "\t")
	if err != nil {
		return err
	}

	return c.FS.WriteFile(rpcConfigFilePath, marshalled)
}

func (c *Container) getHttpWsRpcs(chainId string, cfg *configs.D8XConfig) func() ([]string, []string) {
	numHttpAvailable := len(cfg.HttpRpcList[chainId])
	numWsAvailable := len(cfg.WsRpcList[chainId])

	httpRing := ring.New(numHttpAvailable)
	for i := 0; i < numHttpAvailable; i++ {
		httpRing.Value = cfg.HttpRpcList[chainId][i]
		httpRing = httpRing.Next()
	}

	wsRing := ring.New(numWsAvailable)
	for i := 0; i < numWsAvailable; i++ {
		wsRing.Value = cfg.WsRpcList[chainId][i]
		wsRing = wsRing.Next()
	}

	return func() ([]string, []string) {
		httpRpcs := []string{}
		wsRpcs := []string{}

		// Take at least 2 enties for http or more if possible (or if not
		// possible - at lease numWs/Http)
		amountHttp := 2
		amountWs := 1
		if amountHttp < numHttpAvailable {
			amountHttp = int(math.Ceil(float64(numHttpAvailable) / float64(3)))
		} else {
			amountHttp = numHttpAvailable
		}
		if amountWs < numWsAvailable {
			amountWs = int(math.Ceil(float64(numWsAvailable) / float64(3)))
		} else {
			amountWs = numWsAvailable
		}

		for i := 0; i < amountHttp; i++ {
			httpRpcs = append(httpRpcs, httpRing.Value.(string))
			httpRing = httpRing.Next()
		}
		for i := 0; i < amountWs; i++ {
			wsRpcs = append(wsRpcs, wsRing.Value.(string))
			wsRing = wsRing.Next()
		}

		return httpRpcs, wsRpcs
	}
}