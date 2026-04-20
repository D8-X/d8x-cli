package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) NewEnvironment(ctx *cli.Context) error {
	styles.PrintCommandTitle("Create new environment...")

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN is required (add it to your Bitwarden d8x-cli item, export it, or set it in .env)")
	}

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

	for _, e := range existing {
		if e == envName {
			return fmt.Errorf("environment '%s' already exists", envName)
		}
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

	fmt.Println("Enter domain subdomain suffix (e.g. 84532-v2 for api-84532-v2.d8x.xyz):")
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

	fmt.Println("Enter number of workers:")
	numWorkersStr, err := c.TUI.NewInput(components.TextInputOptValue("3"))
	if err != nil {
		return err
	}

	fmt.Println("Enter server label prefix:")
	labelPrefix, err := c.TUI.NewInput(components.TextInputOptValue(envName))
	if err != nil {
		return err
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

	files := []fileEntry{
		{
			path: envName + "/config.json",
			content: func() string {
				b, _ := json.MarshalIndent(map[string]interface{}{
					"chain_id": chainID,
					"provider": provider,
				}, "", "  ")
				return string(b) + "\n"
			}(),
		},
		{
			path: envName + "/terraform.tfvars",
			content: fmt.Sprintf(`region               = "%s"
num_workers          = %s
broker_size          = "g6-dedicated-2"
server_label_prefix  = "%s"
create_broker_server = true
create_swarm         = true
`, region, numWorkersStr, labelPrefix),
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
    add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
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
		"https://app.predictex.io" 1;
		"https://predictex.io" 1;
		"https://app.predictex.com" 1;
		"https://predictex.com" 1;
		~^https://.*\.d8x-based-predictex-frontend\.pages\.dev$ 1;
		include /etc/nginx/conf.d/staging_origins.map;
	}

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

	limit_req_zone $binary_remote_addr zone=primary_zone:10m rate=25r/s;

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
`),
		},
		{
			path: envName + "/sites.conf",
			content: fmt.Sprintf(`# Main service REST API
server {
    server_name %s;
    listen 80;

    limit_req zone=primary_zone burst=25 nodelay;

    location / {
        include /etc/nginx/auth_check.conf;

        proxy_pass http://127.0.0.1:3001;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        add_header 'Access-Control-Allow-Origin' '*' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';
    }
}

# Main service WS
server {
    server_name %s;
    listen 80;

    limit_req zone=primary_zone burst=20 nodelay;

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
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';
    }
}

# History service REST API
server {
    server_name %s;
    listen 80;

    limit_req zone=primary_zone burst=20 nodelay;

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
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';
    }
}

# Candlesticks websockets service
server {
    server_name %s;
    listen 80;

    limit_req zone=primary_zone burst=20 nodelay;

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
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';
    }
}
`, apiHost, wsHost, historyHost, candlesHost),
		},
		{
			path: envName + "/broker-nginx.conf",
			content: fmt.Sprintf(`# Websocket or REST broker server
server {
    listen 80;
    server_name %s;

    # RPC proxy — restricted to allowed origins and API key
    location /rpc {
        if ($request_method = 'OPTIONS') {
            add_header 'Access-Control-Allow-Origin' '$http_origin' always;
            add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
            add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
            add_header 'Access-Control-Max-Age' 86400;
            return 204;
        }

        set $rpc_allowed 0;

        # Allowed origins
        if ($http_origin = 'https://app.predictex.io') { set $rpc_allowed 1; }
        if ($http_origin = 'https://predictex.io') { set $rpc_allowed 1; }
        if ($http_origin = 'https://app.predictex.com') { set $rpc_allowed 1; }
        if ($http_origin = 'https://predictex.com') { set $rpc_allowed 1; }
        if ($http_origin ~* '^https?://localhost(:\d+)?$') { set $rpc_allowed 1; }
        if ($http_origin ~* '\.d8x-based-predictex-frontend\.pages\.dev$') { set $rpc_allowed 1; }

        # API key
        if ($http_x_api_key = 'API_KEY_HERE') { set $rpc_allowed 1; }

        if ($rpc_allowed = 0) { return 403; }

        proxy_pass http://BROKER_PRIVATE_IP_HERE:8090/rpc;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        add_header 'Access-Control-Allow-Origin' '$http_origin' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,X-Api-Key';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';
    }

    location /ws {
        proxy_pass http://127.0.0.1:8080/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        add_header 'Access-Control-Allow-Origin' '*' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';
    }

    location / {
        proxy_pass http://127.0.0.1:8001;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        add_header 'Access-Control-Allow-Origin' '*' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range';
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range';

        if ($request_method = 'OPTIONS') {
            return 204;
        }
    }
}
`, brokerHost),
		},
	}

	fmt.Println(styles.ItalicText.Render("\nCreating environment files..."))
	for _, f := range files {
		_, err = ghWriteFile(token, f.path, f.content, "", "create "+f.path+" - d8x setup new-env")
		if err != nil {
			return fmt.Errorf("creating %s: %w", f.path, err)
		}
		fmt.Printf("  %s %s\n", ok, f.path)
	}

	fmt.Println(styles.SuccessText.Render(fmt.Sprintf("\nEnvironment '%s' created (chain %d).", envName, chainID)))
	fmt.Println("Next steps:")
	fmt.Printf("  1. d8x setup provision\n")
	fmt.Printf("     -> Create DNS A records for all domains pointing to the server IPs\n")
	fmt.Printf("  2. d8x setup configure\n")
	fmt.Printf("  3. d8x setup swarm-deploy\n")
	fmt.Printf("  4. d8x setup swarm-nginx\n")
	fmt.Printf("     -> Have the broker private key ready\n")
	fmt.Printf("  5. d8x setup broker-deploy\n")
	fmt.Printf("  6. d8x setup broker-nginx\n")

	return nil
}
