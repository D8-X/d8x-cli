package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) NewEnvironment(ctx *cli.Context) error {
	styles.PrintCommandTitle("Create new environment...")

	if err := c.RequireBitwardenField("GITHUB_TOKEN"); err != nil {
		return err
	}
	token := os.Getenv("GITHUB_TOKEN")

	existing, err := ghListDirs(token)
	if err != nil {
		return fmt.Errorf("cannot list environments from GitHub: %w", err)
	}

	fmt.Println("Enter environment name (e.g. arbitrum):")
	envName, err := c.TUI.NewInput(components.TextInputOptPlaceholder("my-new-env"))
	if err != nil {
		return err
	}
	envName = strings.ToLower(strings.TrimSpace(envName))
	if envName == "" {
		return fmt.Errorf("environment name cannot be empty")
	}

	if slices.Contains(existing, envName) {
		return fmt.Errorf("environment '%s' already exists", envName)
	}

	fmt.Println("Enter chain ID:")
	chainIDStr, err := c.TUI.NewInput(components.TextInputOptPlaceholder("42161"))
	if err != nil {
		return err
	}
	chainID, err := strconv.Atoi(strings.TrimSpace(chainIDStr))
	if err != nil {
		return fmt.Errorf("invalid chain ID: %w", err)
	}

	fmt.Println("Enter domain subdomain suffix (e.g. 84532 for api-84532.d8x.xyz):")
	subdomain, err := c.TUI.NewInput(components.TextInputOptValue(chainIDStr))
	if err != nil {
		return err
	}

	fmt.Println("Enter base domain:")
	baseDomain, err := c.TUI.NewInput(components.TextInputOptValue("d8x.xyz"))
	if err != nil {
		return err
	}

	fmt.Println("Select cloud provider:")
	providerSel, err := c.TUI.NewSelection(
		[]string{"linode", "aws"},
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)
	if err != nil {
		return err
	}
	provider := providerSel[0]

	fmt.Println("Enter server region:")
	region, err := c.TUI.NewInput(components.TextInputOptValue("eu-central"))
	if err != nil {
		return err
	}

	var numWorkersStr string
	for {
		fmt.Println("Enter number of workers:")
		numWorkersStr, err = c.TUI.NewInput(components.TextInputOptValue("3"))
		if err != nil {
			return err
		}
		n, perr := strconv.Atoi(strings.TrimSpace(numWorkersStr))
		if perr == nil && n > 0 {
			numWorkersStr = strconv.Itoa(n)
			break
		}
		fmt.Println(styles.AlertImportant.Render(fmt.Sprintf("%q is not a positive integer; try again.", numWorkersStr)))
	}

	fmt.Println("Enter server label prefix:")
	labelPrefix, err := c.TUI.NewInput(components.TextInputOptValue(envName))
	if err != nil {
		return err
	}

	createBroker, err := c.TUI.NewPrompt("Provision a broker server alongside the swarm?", true)
	if err != nil {
		return err
	}
	deploySwarm, err := c.TUI.NewPrompt("Deploy the swarm stack?", true)
	if err != nil {
		return err
	}

	brokerSize := "g6-dedicated-2"
	swarmNodeSize := "g6-dedicated-2"
	rdsInstanceClass := "db.t4g.small"
	useExternalDb := false
	externalDbId := ""
	switch provider {
	case "linode":
		if createBroker {
			fmt.Println("Broker node size:")
			brokerSize, err = c.TUI.NewInput(components.TextInputOptValue("g6-dedicated-2"))
			if err != nil {
				return err
			}
			brokerSize = strings.TrimSpace(brokerSize)
			if brokerSize == "" {
				brokerSize = "g6-dedicated-2"
			}
		}
		if deploySwarm {
			fmt.Println("Swarm node size:")
			swarmNodeSize, err = c.TUI.NewInput(components.TextInputOptValue("g6-dedicated-2"))
			if err != nil {
				return err
			}
			swarmNodeSize = strings.TrimSpace(swarmNodeSize)
			if swarmNodeSize == "" {
				swarmNodeSize = "g6-dedicated-2"
			}
			useExternalDb, err = c.TUI.NewPrompt("Use an external Linode managed Postgres cluster?", false)
			if err != nil {
				return err
			}
			if useExternalDb {
				fmt.Println("External Linode database cluster ID:")
				externalDbId, err = c.TUI.NewInput(components.TextInputOptPlaceholder("12345678"))
				if err != nil {
					return err
				}
				externalDbId = strings.TrimSpace(externalDbId)
			}
		}
	case "aws":
		if deploySwarm {
			fmt.Println("RDS instance class:")
			rdsInstanceClass, err = c.TUI.NewInput(components.TextInputOptValue("db.t4g.small"))
			if err != nil {
				return err
			}
			rdsInstanceClass = strings.TrimSpace(rdsInstanceClass)
			if rdsInstanceClass == "" {
				rdsInstanceClass = "db.t4g.small"
			}
		}
	}

	fmt.Println(styles.ItalicText.Render("\nNginx rate limiting throttles incoming requests per client IP across the api/ws/history/candles endpoints. When the limit is exceeded the client gets HTTP 503 until traffic slows down."))
	rateLimitEnabled, err := c.TUI.NewPrompt("Enable nginx rate limiting for this environment?", true)
	if err != nil {
		return err
	}
	rateLimitStr := "25"
	burstApiStr := "25"
	burstWsStr := "20"
	if rateLimitEnabled {
		rateLimitStr, err = c.promptPositiveInt("Max requests per second per client IP (default 25):", "25", "rate")
		if err != nil {
			return err
		}

		fmt.Println(styles.ItalicText.Render("\nBurst = extra requests a client can fire above the limit in a quick spike (e.g. page load fan-out)."))
		burstApiStr, err = c.promptPositiveInt("'api' burst (default 25):", "25", "api burst")
		if err != nil {
			return err
		}

		burstWsStr, err = c.promptPositiveInt("'ws', 'history' and 'candles' burst (default 20):", "20", "ws/history/candles burst")
		if err != nil {
			return err
		}
	} else {
		fmt.Println(styles.ItalicText.Render("Rate limiting will be written into the nginx configs but commented out. To turn it on later, uncomment the `limit_req`/`limit_req_zone` lines inside the `{enable_rate_limiting}` blocks in <env>/nginx.conf and <env>/sites.conf on the infra repo, then run `d8x setup swarm-nginx` to redeploy."))
	}

	commentPrefix := ""
	if !rateLimitEnabled {
		commentPrefix = "#"
	}

	apiHost := fmt.Sprintf("api-%s.%s", subdomain, baseDomain)
	wsHost := fmt.Sprintf("ws-%s.%s", subdomain, baseDomain)
	historyHost := fmt.Sprintf("history-%s.%s", subdomain, baseDomain)
	candlesHost := fmt.Sprintf("candles-%s.%s", subdomain, baseDomain)
	brokerHost := fmt.Sprintf("broker-%s.%s", subdomain, baseDomain)

	fmt.Println(styles.ItalicText.Render("\nServer names:"))
	fmt.Printf("  api:     %s\n", apiHost)
	fmt.Printf("  ws:      %s\n", wsHost)
	fmt.Printf("  history: %s\n", historyHost)
	fmt.Printf("  candles: %s\n", candlesHost)
	fmt.Printf("  broker:  %s\n", brokerHost)

	type fileEntry struct {
		path    string
		content string
	}

	numWorkers, _ := strconv.Atoi(numWorkersStr)
	scaffoldCfg := &configs.D8XConfig{
		ChainId:        uint(chainID),
		ServerProvider: configs.D8XServerProvider(provider),
	}
	switch provider {
	case "linode":
		scaffoldCfg.LinodeConfig = &configs.D8XLinodeConfig{
			LabelPrefix:        labelPrefix,
			Region:             region,
			NumWorker:          numWorkers,
			BrokerServerSize:   brokerSize,
			SwarmNodeSize:      swarmNodeSize,
			CreateBrokerServer: createBroker,
			DeploySwarm:        deploySwarm,
			DbId:               externalDbId,
		}
	case "aws":
		scaffoldCfg.AWSConfig = &configs.D8XAWSConfig{
			LabelPrefix:        labelPrefix,
			Region:             region,
			NumWorker:          numWorkers,
			RDSInstanceClass:   rdsInstanceClass,
			CreateBrokerServer: createBroker,
			DeploySwarm:        deploySwarm,
		}
	}
	configJSON, err := buildScaffoldConfigJSON(scaffoldCfg)
	if err != nil {
		return err
	}
	tfvars := buildTfvars(scaffoldCfg)

	files := []fileEntry{
		{
			path:    envName + "/config.json",
			content: configJSON,
		},
		{
			path:    envName + "/terraform.tfvars",
			content: tfvars,
		},
		{
			path:    envName + "/staging_origins.map",
			content: "# Staging origins > managed by: ./d8x staging-origins\n",
		},
		{
			path: envName + "/auth_check.conf",
			content: `if ($request_method = 'OPTIONS') {
    add_header 'Access-Control-Allow-Origin' '*' always;
    add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
    add_header 'Access-Control-Allow-Headers' 'Authorization,DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
    add_header 'Access-Control-Max-Age' 86400;
    return 204;
}

set $auth_ok 0;

if ($is_allowed_origin = 1) {
    set $auth_ok 1;
}

if ($http_x_api_key = 'API_KEY_HERE') {
    set $auth_ok 1;
}

# Grafana dashboard server
if ($remote_addr = '172.104.135.218') {
    set $auth_ok 1;
}

if ($auth_ok = 0) {
    return 403;
}
`,
		},
		{
			path: envName + "/nginx.conf",
			content: fmt.Sprintf(`user www-data;
worker_processes auto;
pid /run/nginx.pid;
include /etc/nginx/modules-enabled/*.conf;

events {
	worker_connections 5000;
}

http {

	map_hash_bucket_size 128;

	map $http_origin $is_allowed_origin {
		default 0;
		~^https?://localhost(:\d+)?$ 1;
		~^https?://127\.0\.0\.1(:\d+)?$ 1;
		include /etc/nginx/conf.d/staging_origins.map;
	}

	# Cloudflare real ip setup (cloudflare.com/ips-v4). Do not edit this
	# comment.
	#
	# {real_ip_cloudflare}

	set_real_ip_from 173.245.48.0/20;
	set_real_ip_from 103.21.244.0/22;
	set_real_ip_from 103.22.200.0/22;
	set_real_ip_from 103.31.4.0/22;
	set_real_ip_from 141.101.64.0/18;
	set_real_ip_from 108.162.192.0/18;
	set_real_ip_from 190.93.240.0/20;
	set_real_ip_from 188.114.96.0/20;
	set_real_ip_from 197.234.240.0/22;
	set_real_ip_from 198.41.128.0/17;
	set_real_ip_from 162.158.0.0/15;
	set_real_ip_from 104.16.0.0/13;
	set_real_ip_from 104.24.0.0/14;
	set_real_ip_from 172.64.0.0/13;
	set_real_ip_from 131.0.72.0/22;

	real_ip_header CF-Connecting-IP;

	# {/real_ip_cloudflare}

	# {enable_rate_limiting}
	%[1]slimit_req_zone $binary_remote_addr zone=primary_zone:10m rate=%[2]sr/s;
	# {/enable_rate_limiting}

	server_names_hash_bucket_size 128;

	sendfile on;
	tcp_nopush on;
	types_hash_max_size 2048;
	server_tokens off;
	include /etc/nginx/mime.types;

	ssl_protocols TLSv1.2 TLSv1.3;
	ssl_prefer_server_ciphers on;

	access_log /var/log/nginx/access.log;
	error_log /var/log/nginx/error.log;

	gzip on;

	include /etc/nginx/conf.d/*.conf;
	include /etc/nginx/sites-enabled/*;
}
`, commentPrefix, rateLimitStr),
		},
		{
			path: envName + "/sites.conf",
			content: fmt.Sprintf(`# Main service REST API
server {
    server_name %[1]s;
    listen 80;

    # {enable_rate_limiting}
    %[5]slimit_req zone=primary_zone burst=%[6]s nodelay;
    # {/enable_rate_limiting}

    location / {
        include /etc/nginx/auth_check.conf;

        proxy_pass http://127.0.0.1:3001;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        add_header 'Access-Control-Allow-Origin' '*' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
        add_header 'Access-Control-Allow-Headers' 'Authorization,DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';
    }
}

# Main service WS
server {
    server_name %[2]s;
    listen 80;

    # {enable_rate_limiting}
    %[5]slimit_req zone=primary_zone burst=%[7]s nodelay;
    # {/enable_rate_limiting}

    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-XSS-Protection "1; mode=block" always;

    location / {
        proxy_pass http://127.0.0.1:3002;

        proxy_read_timeout 60;
        proxy_connect_timeout 60;
        proxy_redirect off;

        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        add_header 'Access-Control-Allow-Origin' '*' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
        add_header 'Access-Control-Allow-Headers' 'Authorization,DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';
    }
}

# History service REST API
server {
    server_name %[3]s;
    listen 80;

    # {enable_rate_limiting}
    %[5]slimit_req zone=primary_zone burst=%[7]s nodelay;
    # {/enable_rate_limiting}

    location /health {
        proxy_pass http://127.0.0.1:3003;
        proxy_set_header Host $host;
    }

    location /status {
        proxy_pass http://127.0.0.1:3003;
        proxy_set_header Host $host;
    }

    location / {
        include /etc/nginx/auth_check.conf;

        proxy_pass http://127.0.0.1:3003;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        add_header 'Access-Control-Allow-Origin' '*' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
        add_header 'Access-Control-Allow-Headers' 'Authorization,DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';
    }
}

# Candlesticks websockets service
server {
    server_name %[4]s;
    listen 80;

    # {enable_rate_limiting}
    %[5]slimit_req zone=primary_zone burst=%[7]s nodelay;
    # {/enable_rate_limiting}

    location / {
        proxy_pass http://127.0.0.1:3005/ws;

        proxy_read_timeout 60;
        proxy_connect_timeout 60;
        proxy_redirect off;

        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        add_header 'Access-Control-Allow-Origin' '*' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
        add_header 'Access-Control-Allow-Headers' 'Authorization,DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';
    }
}
`, apiHost, wsHost, historyHost, candlesHost, commentPrefix, burstApiStr, burstWsStr),
		},
		{
			path: envName + "/broker-nginx.conf",
			content: fmt.Sprintf(`# Websocket or REST broker server

map $http_origin $is_allowed_origin {
    default 0;
    ~^https?://localhost(:\d+)?$ 1;
    ~^https?://127\.0\.0\.1(:\d+)?$ 1;
}

server {
    listen 80;
    server_name %s;

    location /rpc {
        # CORS preflight: origin-only check (browsers don't send X-Api-Key on preflight)
        if ($request_method = 'OPTIONS') {
            add_header 'Access-Control-Allow-Origin' "$http_origin" always;
            add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS' always;
            add_header 'Access-Control-Allow-Headers' 'Authorization,DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key' always;
            add_header 'Access-Control-Max-Age' 86400 always;
            return 204;
        }

        # Actual requests: allow if origin is in allowlist OR api key matches
        set $rpc_allowed $is_allowed_origin;
        if ($http_x_api_key = 'API_KEY_HERE') { set $rpc_allowed 1; }
        if ($rpc_allowed = 0) { return 403; }

        proxy_pass http://BROKER_PRIVATE_IP_HERE:8090/rpc;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        add_header 'Access-Control-Allow-Origin' "$http_origin" always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS' always;
        add_header 'Access-Control-Allow-Headers' 'Authorization,DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key' always;
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range' always;
    }

    location /ws {
        proxy_pass http://127.0.0.1:8080/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        add_header 'Access-Control-Allow-Origin' "$http_origin" always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS' always;
        add_header 'Access-Control-Allow-Headers' 'Authorization,DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range' always;
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range' always;
    }

    location / {
        if ($request_method = 'OPTIONS') {
            add_header 'Access-Control-Allow-Origin' "$http_origin" always;
            add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS' always;
            add_header 'Access-Control-Allow-Headers' 'Authorization,DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range' always;
            return 204;
        }

        proxy_pass http://127.0.0.1:8001;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        add_header 'Access-Control-Allow-Origin' "$http_origin" always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS' always;
        add_header 'Access-Control-Allow-Headers' 'Authorization,DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range' always;
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range' always;
    }
}
`, brokerHost),
		},
	}

	playbook, err := configs.GetSetupAnsiblePlaybook()
	if err != nil {
		return fmt.Errorf("loading embedded setup.ansible.yaml: %w", err)
	}
	files = append(files, fileEntry{
		path:    envName + "/setup.ansible.yaml",
		content: string(playbook),
	})

	operatorOverrides, err := c.collectOperatorConfigs(uint(chainID))
	if err != nil {
		return err
	}
	for envRelPath, content := range operatorOverrides {
		files = append(files, fileEntry{
			path:    envName + "/" + envRelPath,
			content: content,
		})
	}

	fmt.Println(styles.ItalicText.Render("\nCreating environment files..."))
	commitFiles := make([]ghCommitFile, 0, len(files))
	for _, f := range files {
		commitFiles = append(commitFiles, ghCommitFile{Path: f.path, Content: f.content})
		fmt.Printf("  %s %s\n", ok, f.path)
	}
	if err := ghCommitFiles(token, commitFiles, fmt.Sprintf("scaffold %s environment", envName)); err != nil {
		return fmt.Errorf("creating environment files: %w", err)
	}

	fmt.Println(styles.SuccessText.Render(fmt.Sprintf("\nEnvironment '%s' created (chain %d).", envName, chainID)))
	c.SelectedEnv = envName
	if c.Input != nil {
		c.Input.SelectedEnv = envName
	}
	return nil
}

func (c *Container) collectOperatorConfigs(chainID uint) (map[string]string, error) {
	out := map[string]string{}
	fmt.Println()
	fmt.Println(styles.PurpleBgText.Copy().Padding(0, 2).Render(
		fmt.Sprintf(" Per-chain config files for chain %d ", chainID),
	))
	fmt.Println(styles.ItalicText.Render("Each section configures one file in the infra repo. Skip prompts you don't need; defaults are used otherwise."))

	printConfigStep(1, 5, "trader-backend/rpc.main.json", "api service RPC list", chainID)
	httpMain, err := c.collectCommaList("HTTP RPC URLs (comma-separated, required):", true)
	if err != nil {
		return nil, err
	}
	wsMain, err := c.collectCommaList("WS RPC URLs (comma-separated, optional):", false)
	if err != nil {
		return nil, err
	}
	if len(httpMain) > 0 {
		out["trader-backend/rpc.main.json"] = renderRpcMainHistory(chainID, httpMain, wsMain)
	}

	printConfigStep(2, 5, "trader-backend/rpc.history.json", "history service RPC list", chainID)
	reuse, err := c.TUI.NewPrompt("Reuse the same RPCs as rpc.main.json?", true)
	if err != nil {
		return nil, err
	}
	if reuse {
		if len(httpMain) > 0 {
			out["trader-backend/rpc.history.json"] = renderRpcMainHistory(chainID, httpMain, wsMain)
		}
	} else {
		httpHist, err := c.collectCommaList("HTTP RPC URLs (comma-separated, required):", true)
		if err != nil {
			return nil, err
		}
		wsHist, err := c.collectCommaList("WS RPC URLs (comma-separated, optional):", false)
		if err != nil {
			return nil, err
		}
		if len(httpHist) > 0 {
			out["trader-backend/rpc.history.json"] = renderRpcMainHistory(chainID, httpHist, wsHist)
		}
	}

	printConfigStep(3, 5, "candles/rpc_conf.json", "candles WS listener RPCs", chainID)
	configureCandles, err := c.TUI.NewPrompt("Configure candles RPC list now? (skip to keep embedded defaults)", true)
	if err != nil {
		return nil, err
	}
	if configureCandles {
		fmt.Println("Short description (optional, e.g. \"base mainnet RPCs for univ2 listener\"):")
		desc, err := c.TUI.NewInput(components.TextInputOptPlaceholder(""))
		if err != nil {
			return nil, err
		}
		httpsCandles, err := c.collectCommaList("HTTPS RPC URLs (comma-separated, required):", true)
		if err != nil {
			return nil, err
		}
		wssCandles, err := c.collectCommaList("WSS RPC URLs (comma-separated, required):", true)
		if err != nil {
			return nil, err
		}
		if len(httpsCandles) > 0 && len(wssCandles) > 0 {
			out["candles/rpc_conf.json"] = renderCandlesRpcConf(chainID, strings.TrimSpace(desc), httpsCandles, wssCandles)
		}
	}

	printConfigStep(4, 5, "broker-server/rpc.json", "broker RPC list", chainID)
	reuseBroker, err := c.TUI.NewPrompt("Reuse rpc.main.json HTTPs for the broker?", true)
	if err != nil {
		return nil, err
	}
	if reuseBroker && len(httpMain) > 0 {
		out["broker-server/rpc.json"] = renderBrokerRpc(chainID, httpMain)
	} else if !reuseBroker {
		httpsBroker, err := c.collectCommaList("HTTPS RPC URLs (comma-separated, required):", true)
		if err != nil {
			return nil, err
		}
		if len(httpsBroker) > 0 {
			out["broker-server/rpc.json"] = renderBrokerRpc(chainID, httpsBroker)
		}
	}

	printConfigStep(5, 5, "broker-server/chainConfig.json", "broker chain name", chainID)
	fmt.Println("Chain name (e.g. \"base\", \"arbitrum\", \"berachain\"; skip to keep embedded defaults):")
	chainName, err := c.TUI.NewInput(components.TextInputOptPlaceholder(""))
	if err != nil {
		return nil, err
	}
	if chainName = strings.TrimSpace(chainName); chainName != "" {
		out["broker-server/chainConfig.json"] = renderBrokerChainConfig(chainID, chainName)
	}

	return out, nil
}

func printConfigStep(step, total int, path, summary string, chainID uint) {
	fmt.Println()
	banner := styles.PurpleBgText.Copy().Padding(0, 2).Render(
		fmt.Sprintf(" [%d/%d] %s ", step, total, path),
	)
	fmt.Println(banner)
	fmt.Println(styles.CommandTitleText.Render(fmt.Sprintf("%s for chain %d", summary, chainID)))
}

func (c *Container) collectCommaList(prompt string, required bool) ([]string, error) {
	for {
		fmt.Println(prompt)
		raw, err := c.TUI.NewInput(components.TextInputOptPlaceholder(""))
		if err != nil {
			return nil, err
		}
		parts := []string{}
		for _, p := range strings.Split(raw, ",") {
			if v := strings.TrimSpace(p); v != "" {
				parts = append(parts, v)
			}
		}
		if len(parts) > 0 || !required {
			return parts, nil
		}
		fmt.Println(styles.ErrorText.Render("At least one value is required. Try again."))
	}
}

func renderRpcMainHistory(chainID uint, http, ws []string) string {
	type entry struct {
		ChainID uint     `json:"chainId"`
		HTTP    []string `json:"HTTP"`
		WS      []string `json:"WS"`
	}
	if ws == nil {
		ws = []string{}
	}
	data, _ := json.MarshalIndent([]entry{{ChainID: chainID, HTTP: http, WS: ws}}, "", "  ")
	return string(data) + "\n"
}

func renderCandlesRpcConf(chainID uint, desc string, https, wss []string) string {
	type entry struct {
		ChainID     uint     `json:"chainId"`
		Description string   `json:"description"`
		HTTPS       []string `json:"https"`
		WSS         []string `json:"wss"`
	}
	data, _ := json.MarshalIndent([]entry{{ChainID: chainID, Description: desc, HTTPS: https, WSS: wss}}, "", "  ")
	return string(data) + "\n"
}

func renderBrokerRpc(chainID uint, https []string) string {
	type entry struct {
		ChainID uint     `json:"chainId"`
		HTTPS   []string `json:"HTTPS"`
	}
	data, _ := json.MarshalIndent([]entry{{ChainID: chainID, HTTPS: https}}, "", "  ")
	return string(data) + "\n"
}

func renderBrokerChainConfig(chainID uint, name string) string {
	type entry struct {
		ChainID uint   `json:"chainId"`
		Name    string `json:"name"`
	}
	data, _ := json.MarshalIndent([]entry{{ChainID: chainID, Name: name}}, "", "  ")
	return string(data) + "\n"
}

func buildScaffoldConfigJSON(cfg *configs.D8XConfig) (string, error) {
	out := map[string]any{
		"chain_id":        cfg.ChainId,
		"server_provider": cfg.ServerProvider,
	}
	if cfg.LinodeConfig != nil {
		out["linode_config"] = map[string]any{
			"label_prefix":         cfg.LinodeConfig.LabelPrefix,
			"region":               cfg.LinodeConfig.Region,
			"num_worker":           cfg.LinodeConfig.NumWorker,
			"broker_server_size":   cfg.LinodeConfig.BrokerServerSize,
			"swarm_node_size":      cfg.LinodeConfig.SwarmNodeSize,
			"create_broker_server": cfg.LinodeConfig.CreateBrokerServer,
			"deploy_swarm":         cfg.LinodeConfig.DeploySwarm,
			"db_id":                cfg.LinodeConfig.DbId,
		}
	}
	if cfg.AWSConfig != nil {
		out["aws_config"] = map[string]any{
			"label_prefix":         cfg.AWSConfig.LabelPrefix,
			"region":               cfg.AWSConfig.Region,
			"num_worker":           cfg.AWSConfig.NumWorker,
			"rds_instance_class":   cfg.AWSConfig.RDSInstanceClass,
			"create_broker_server": cfg.AWSConfig.CreateBrokerServer,
			"deploy_swarm":         cfg.AWSConfig.DeploySwarm,
		}
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

func (c *Container) promptPositiveInt(prompt, defaultVal, label string) (string, error) {
	for {
		fmt.Println(prompt)
		v, err := c.TUI.NewInput(components.TextInputOptValue(defaultVal))
		if err != nil {
			return "", err
		}
		v = strings.TrimSpace(v)
		if v == "" {
			v = defaultVal
		}
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return v, nil
		}
		fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Invalid %s '%s': must be a positive integer. Try again.", label, v)))
	}
}
