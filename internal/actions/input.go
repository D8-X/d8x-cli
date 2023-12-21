package actions

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
	"github.com/xo/dburl"
)

// InputCollector manages the state of user input. User input is collected on
// almost all setup actions. InputCollector eases the management of how and
// where we collect input values, exposing only the neccessary methods to
// collect input for each specific action and retrieve the collected input.
// Input collection methods automatically check if input was already collected
// or is available in the given config.
type InputCollector struct {
	// Cfg should be refreshed on each collect action, since we might hold a
	// stale reference here.
	Cfg *configs.D8XConfig
	// func to refresh or persist cfg
	ConfigRWriter configs.D8XConfigReadWriter

	// Cached chain.json data, loaded when InputCollector is created
	ChainJson ChainJson

	// Default ssh key path
	SSHKeyPath string
	// Whenever ssh key hash changes - this will be set to true. SSH key change
	// implies that all servers will be reprovisioned.
	sshKeyChanged bool

	TUI components.ComponentsRunner

	chainIdSelected bool

	// setup subcommand state
	setup InputCollectorSetupData

	// Provisioning subcommand state
	provisioning ProvisioningInput

	// Collected broker deployment data
	brokerDeployInput BrokerDeployInput
	// Collected broker nginx setup data
	brokerNginxInput BrokerNginxInput

	// Collected swarm deployment data
	swarmDeployInput SwarmDeployInput
	// Collected swarm nginx setup data
	swarmNginxInput SwarmNginxInput

	// Whether we should run the nginx+certbot setup for broker/swarm servers.
	// Only when ssh key changes, or when broker/swarm was not deployed before.
	runBrokerNginxCertbot bool
	runSwarmNginxCertbot  bool
}

type ProvisioningInput struct {
	collected bool

	selectedServerProvider SupportedServerProvider

	collectedLinodeConfigurer *linodeConfigurer
	collectedAwsConfigurer    *awsConfigurer
}

type InputCollectorSetupData struct {
	// Whether to provision and deploy broker server
	deployBroker bool
	// Whether to provision and deploy swarm servers (+rds on aws)
	deploySwarm bool

	setupDomainEntered bool

	certbotEmailEntered bool

	deployMetrics bool
}

type BrokerDeployInput struct {
	// Is data already collected
	collected bool

	// Broker private key
	privateKey string

	// Broker fee
	feeTBPS string
}

type BrokerNginxInput struct {
	// Is this data already collected
	collected bool

	setupNginx   bool
	setupCertbot bool

	// broker-server domain name
	domainName string
}

type SwarmDeployInput struct {
	collected bool
	// whether user selected to be guided through configuration by cli
	guideConfig bool

	// Pyth endpoints for candles/prices.config.json
	priceServiceWSEndpoints []string

	referralPaymentExecutorPrivateKey    string
	referralPaymentExecutorWalletAddress string
}

type SwarmNginxInput struct {
	// Is this data already collected
	collected bool

	setupNginx   bool
	setupCertbot bool

	// Collected service domain names
	collectedServiceDomains []hostnameTuple
}

// CollectFullSetupInput collects complete deployment information for both swarm
// and broker server deployments. This does not include credentials collection for
// server provider setup.
func (input *InputCollector) CollectFullSetupInput(ctx *cli.Context) error {
	// Collect initial values of deployment status. These values will reflect
	// the state of broker/swarm/metrics deployments before any setup actions
	// are run. This cfg should not be used after determining deployment
	// statuses since the actual cfg will be updated within setup actions.
	cfg, err := input.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	isBrokerDeployed := cfg.BrokerDeployed
	isSwarmDeployed := cfg.SwarmDeployed

	// Collect server provider related provisioning data
	if err := input.CollectProvisioningData(ctx); err != nil {
		return err
	}

	// input.sshKeyChanged  will be updated after provisioning data is
	// collected. Here we'll determine if broker/swarm was deployed previously
	// and only if it was - don't mark it to run the nginx+certbot setup if key
	// did not change. Otherwise we want to prompt user to run nginx+certbot
	input.runBrokerNginxCertbot = input.sshKeyChanged || (!isBrokerDeployed && input.setup.deployBroker && !cfg.BrokerNginxDeployed && !cfg.BrokerCertbotDeployed)
	input.runSwarmNginxCertbot = input.sshKeyChanged || (!isSwarmDeployed && input.setup.deploySwarm && !cfg.SwarmNginxDeployed && !cfg.SwarmCertbotDeployed)

	// Collect private keys whenever they are not collected
	if err := input.CollectPrivateKeys(ctx); err != nil {
		return err
	}

	// Broker deployment inputs
	if input.setup.deployBroker {
		// Setup info
		if err := input.CollectBrokerDeployInput(ctx); err != nil {
			return err
		}

		// Whenever ssh key changes or broker was not deployed yet - collect and
		// run nginx+certbot setup for broker
		if input.runBrokerNginxCertbot {
			if err := input.CollectBrokerNginxInput(ctx); err != nil {
				return err
			}
		}
	}

	// Swarm deployment inputs
	if input.setup.deploySwarm {
		if err := input.CollectSwarmDeployInputs(ctx); err != nil {
			return err
		}

		if input.runSwarmNginxCertbot {
			if err := input.CollectSwarmNginxInputs(ctx); err != nil {
				return err
			}
		}

		deployMetrics, err := input.TUI.NewPrompt("Do you want to deploy metrics (prometheus/grafana) stack?", true)
		if err != nil {
			return err
		}
		input.setup.deployMetrics = deployMetrics
	}

	return nil
}

func (input *InputCollector) CollectProvisioningData(ctx *cli.Context) error {
	if input.provisioning.collected {
		return nil
	}

	cfg, err := input.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// We must ask about broker server before collecting information for the
	// actual provisioning step, since we need to know whether to ask for broker
	// server size on linode.
	createBrokerServer, err := input.TUI.NewPrompt("Do you want to provision a broker server?", true)
	if err != nil {
		return err
	}
	input.setup.deployBroker = createBrokerServer
	// Same for swarm servers
	deploySwarm, err := input.TUI.NewPrompt("Do you want to provision swarm servers?", true)
	if err != nil {
		return err
	}
	input.setup.deploySwarm = deploySwarm

	if !input.setup.deployBroker && !input.setup.deploySwarm {
		return fmt.Errorf("no servers selected for provisioning, quitting")
	}

	fmt.Println(styles.ItalicText.Render("Collecting provisioning information...\n"))

	// Select server provider from  a list of supported server providers
	fmt.Println("Select your server provider")
	selectedProvider, err := input.TUI.NewSelection([]string{
		string(ServerProviderLinode),
		string(ServerProviderAws),
	},
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)
	if err != nil {
		return err
	}
	input.provisioning.selectedServerProvider = SupportedServerProvider(selectedProvider[0])

	if err := input.EnsureSSHKeyPresent(input.SSHKeyPath, cfg); err != nil {
		return fmt.Errorf("ensuring ssh key is present: %w", err)
	} else {
		// Print a line because ssh-keygen output can be long
		fmt.Println("")
	}

	// Collect public ssh key for server provider configurers
	authorizedKey, err := getPublicKey(input.SSHKeyPath)
	if err != nil {
		return err
	}

	// Finalize server provider details
	switch input.provisioning.selectedServerProvider {
	case ServerProviderLinode:
		configurer, err := input.CollectLinodeProviderDetails(cfg)
		if err != nil {
			return err
		}
		configurer.authorizedKey = authorizedKey
		input.provisioning.collectedLinodeConfigurer = &configurer

	case ServerProviderAws:
		configurer, err := input.CollectAwProviderDetails(cfg)
		if err != nil {
			return err
		}
		configurer.authorizedKey = authorizedKey
		input.provisioning.collectedAwsConfigurer = &configurer
	}

	// Update cfg - it will be pre-populated with server provider details from
	// collector funcs
	if err := input.ConfigRWriter.Write(cfg); err != nil {
		return err
	}

	input.provisioning.collected = true

	return nil
}

// PostProvisioningHook runs after provisioning in order to collect any missing
// data. For example we can directly collect linode database DSN, but can't do
// that for AWS RDS, since we
func (input *InputCollector) PostProvisioningHook() error {
	cfg, err := input.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Attempt to parse aws_rds credentials file
	if err := collectAwsRdsDsnString(cfg); err != nil {
		return err
	}

	return input.ConfigRWriter.Write(cfg)
}

// CollectBrokerPrivateKey collects broker private key and stores it in input
// state
func (input *InputCollector) CollectBrokerPrivateKey() error {
	pk, _, err := input.CollectAndValidatePrivateKey("Enter your broker private key:")
	if err != nil {
		return err
	}
	input.brokerDeployInput.privateKey = pk

	return nil
}

// CollectReferralExecutorPrivateKey collects referral executor private key and
// stores it in input state
func (input *InputCollector) CollectReferralExecutorPrivateKey(cfg *configs.D8XConfig) error {
	pk, pkWalletAddress, err := input.CollectAndValidatePrivateKey("Enter your referral executor private key:")
	if err != nil {
		return err
	}
	input.swarmDeployInput.referralPaymentExecutorPrivateKey = pk
	input.swarmDeployInput.referralPaymentExecutorWalletAddress = pkWalletAddress

	// Store the referral executor wallet address for broker-deploy step
	cfg.BrokerServerConfig.ExecutorAddress = pkWalletAddress

	return input.ConfigRWriter.Write(cfg)
}

// CollectPrivateKeys collects broker and referral executor private keys. Only
// once per session
func (input *InputCollector) CollectPrivateKeys(ctx *cli.Context) error {
	if input.brokerDeployInput.privateKey != "" && input.swarmDeployInput.referralPaymentExecutorPrivateKey != "" {
		return nil
	}

	fmt.Println(styles.ItalicText.Render("Collecting private keys...\n"))

	cfg, err := input.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Broker private key must be collected only once per session. Do not
	// collect it for individual swarm-deploy or user chooses not to deploy
	// broker during the setup
	if input.brokerDeployInput.privateKey == "" && ctx.Command.Name != "swarm-deploy" {
		collect := true
		if ctx.Command.Name == "setup" && !input.setup.deployBroker {
			collect = false
		}

		if collect {
			if err := input.CollectBrokerPrivateKey(); err != nil {
				return err
			}
		}
	}

	// Collect referral executor private key
	if input.swarmDeployInput.referralPaymentExecutorPrivateKey == "" {
		if err := input.CollectReferralExecutorPrivateKey(cfg); err != nil {
			return err
		}
	}

	return nil
}

// CollectBrokerDeployInput collects all the input required for broker-deploy
// action. For broker deployment we do not collect chain id or rpc urls since
// broker is chain agnostic.
func (input *InputCollector) CollectBrokerDeployInput(ctx *cli.Context) error {
	if input.brokerDeployInput.collected {
		return nil
	}

	fmt.Println(styles.ItalicText.Render("Collecting broker-deploy information...\n"))

	// Collect private keys whenever they are not collected
	if err := input.CollectPrivateKeys(ctx); err != nil {
		return err
	}

	tbpsFromPercentage, err := input.CollectBrokerFee()
	if err != nil {
		return err
	}
	input.brokerDeployInput.feeTBPS = tbpsFromPercentage

	// Finalize the input collection for broker-deploy by marking it collected
	input.brokerDeployInput.collected = true
	return nil
}

func (input *InputCollector) CollectBrokerNginxInput(ctx *cli.Context) error {
	if input.brokerNginxInput.collected {
		return nil
	}

	fmt.Println(styles.ItalicText.Render("Collecting broker-nginx information...\n"))

	cfg, err := input.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	setupNginx, err := input.TUI.NewPrompt("Do you want to setup nginx for broker-server?", true)
	if err != nil {
		return err
	}
	input.brokerNginxInput.setupNginx = setupNginx
	setupCertbot, err := input.TUI.NewPrompt("Do you want to setup SSL with certbot for broker-server?", true)
	if err != nil {
		return err
	}
	input.brokerNginxInput.setupCertbot = setupCertbot

	// Collect domain name
	_, err = input.CollectSetupDomain(cfg)
	if err != nil {
		return err
	}

	// Collect certbot email
	if input.brokerNginxInput.setupCertbot {
		if _, err := input.CollectCertbotEmail(cfg); err != nil {
			return err
		}
	}

	// Collect broker server domain. Broker does not use chain id, so we will
	// not use SuggestSubdomain, but simply suggest broker.domain
	domainValue := "broker." + cfg.SetupDomain
	if v, ok := cfg.Services[configs.D8XServiceBrokerServer]; ok {
		if v.HostName != "" {
			domainValue = v.HostName
		}
	}
	brokerServerName, err := input.CollectInputWithConfirmation(
		"Enter Broker-server HTTP (sub)domain (e.g. broker.d8x.xyz):",
		"Is this correct?",
		components.TextInputOptPlaceholder("your-broker.domain.com"),
		components.TextInputOptValue(domainValue),
		components.TextInputOptDenyEmpty(),
	)
	if err != nil {
		return err
	}
	brokerServerName = TrimHttpsPrefix(brokerServerName)
	input.brokerNginxInput.domainName = brokerServerName

	input.brokerNginxInput.collected = true

	return nil
}

// CollectSwarmDeployInputs collects various information points which will be
// automatically added to the configuration files. Collected info is: chain ids,
// rpc urls.
func (input *InputCollector) CollectSwarmDeployInputs(ctx *cli.Context) error {
	if input.swarmDeployInput.collected {
		return nil
	}

	fmt.Println(styles.ItalicText.Render("Collecting swarm-deploy information...\n"))

	// Collect private keys whenever they are not collected
	if err := input.CollectPrivateKeys(ctx); err != nil {
		return err
	}

	guideUser, err := input.TUI.NewPrompt("Would you like the cli to guide you through swarm-deploy configuration?", true)
	if err != nil {
		return err
	}
	input.swarmDeployInput.guideConfig = guideUser
	if guideUser {

		cfg, err := input.ConfigRWriter.Read()
		if err != nil {
			return err
		}

		chainId, err := input.GetChainId(cfg, ctx)
		if err != nil {
			return err
		}

		chainIdStr := strconv.Itoa(int(chainId))
		// Collect HTTP rpc endpoints
		if err := input.CollectHTTPRPCUrls(cfg, chainIdStr); err != nil {
			return err
		}
		// Collect Websocket rpc endpoints
		if err := input.CollectWebsocketRPCUrls(cfg, chainIdStr); err != nil {
			return err
		}

		// Collect DB dsn
		if err := input.CollectDatabaseDSN(cfg); err != nil {
			return err
		}

		// Generate redis password
		if cfg.SwarmRedisPassword == "" {
			pwd, err := generatePassword(20)
			if err != nil {
				return fmt.Errorf("generating password for redis: %w", err)
			}
			if err != nil {
				return err
			}
			cfg.SwarmRedisPassword = pwd
		}

		// Collect broker payout address
		if err := input.CollecteBrokerPayoutAddress(cfg); err != nil {
			return err
		}

		// Collect broker http endpoint
		changeRemoteBrokerHTTP := true
		if cfg.SwarmRemoteBrokerHTTPUrl != "" {
			fmt.Printf("Found remote broker http url: %s\n", cfg.SwarmRemoteBrokerHTTPUrl)
			if keep, err := input.TUI.NewPrompt("Do you want to keep remote broker http endpoint?", true); err != nil {
				return err
			} else if keep {
				changeRemoteBrokerHTTP = false
			}
		}
		if changeRemoteBrokerHTTP {
			value := cfg.SwarmRemoteBrokerHTTPUrl
			// Prepopulate broker http address if deployment was done
			if v, ok := cfg.Services[configs.D8XServiceBrokerServer]; ok {
				value = v.HostName
			}
			// Lastly, prepopulate with available value from broker-nginx setup
			if value == "" && input.brokerNginxInput.domainName != "" {
				value = EnsureHttpsPrefixExists(input.brokerNginxInput.domainName)
			}

			fmt.Println("Enter remote broker http url:")
			brokerUrl, err := input.TUI.NewInput(
				components.TextInputOptPlaceholder("https://your-broker-domain.com"),
				components.TextInputOptValue(value),
				components.TextInputOptDenyEmpty(),
				components.TextInputOptValidation(ValidateHttp, "url must start with http:// or https://"),
			)
			if err != nil {
				return err
			}
			cfg.SwarmRemoteBrokerHTTPUrl = EnsureHttpsPrefixExists(brokerUrl)
		}

		// Collect hermes endpoint for candles/prices.config.json. Make sure the
		// default pyth.hermes entry is always the last one
		dontAddAnotherPythWss, err := input.TUI.NewPrompt("\nUse public Hermes Pyth Price Service endpoint only (entry in ./candles/prices.config.json)?", true)
		if err != nil {
			return err
		}
		priceServiceWSEndpoints := []string{input.ChainJson.getDefaultPythWSEndpoint(strconv.Itoa(int(cfg.ChainId)))}
		if !dontAddAnotherPythWss {
			fmt.Println("Enter additional Pyth priceServiceWSEndpoints entry")
			additioanalWsEndpoint, err := input.TUI.NewInput(
				components.TextInputOptPlaceholder("wss://hermes.pyth.network/ws"),
				components.TextInputOptDenyEmpty(),
				components.TextInputOptValidation(
					func(s string) bool {
						return strings.HasPrefix(s, "wss://") || strings.HasPrefix(s, "ws://")
					},
					"websockets url must start with wss:// or ws://",
				),
			)
			additioanalWsEndpoint = strings.TrimSpace(additioanalWsEndpoint)
			if err != nil {
				return err
			}
			priceServiceWSEndpoints = append([]string{additioanalWsEndpoint}, priceServiceWSEndpoints...)
		}
		input.swarmDeployInput.priceServiceWSEndpoints = priceServiceWSEndpoints

		// Update the config
		if err := input.ConfigRWriter.Write(cfg); err != nil {
			return err
		}
	}

	input.swarmDeployInput.collected = true

	return nil
}

func (input *InputCollector) CollectSwarmNginxInputs(ctx *cli.Context) error {
	if input.swarmNginxInput.collected {
		return nil
	}

	fmt.Println(styles.ItalicText.Render("Collecting swarm-nginx information...\n"))

	cfg, err := input.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	setupNginx, err := input.TUI.NewPrompt("Do you want to setup nginx for swarm-server?", true)
	if err != nil {
		return err
	}
	input.swarmNginxInput.setupNginx = setupNginx
	setupCertbot, err := input.TUI.NewPrompt("Do you want to setup SSL with certbot for manager server?", true)
	if err != nil {
		return err
	}
	input.swarmNginxInput.setupCertbot = setupCertbot

	// Collect domain name
	_, err = input.CollectSetupDomain(cfg)
	if err != nil {
		return err
	}

	// Collect certbot email
	if input.swarmNginxInput.setupCertbot {
		if _, err := input.CollectCertbotEmail(cfg); err != nil {
			return err
		}
	}

	// Collect swarm services domain
	if _, err := input.SwarmNginxCollectDomains(cfg); err != nil {
		return err
	}

	input.swarmNginxInput.collected = true

	return nil
}

// CollectBrokerFee requires user to enter broker fee in percentage and returns
// TBPS value
func (c *InputCollector) CollectBrokerFee() (string, error) {
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return "", err
	}

	// Pre-populate existing value whenever possible
	value := "0.06"
	if cfg.BrokerServerConfig.FeeInputPercent != "" {
		value = cfg.BrokerServerConfig.FeeInputPercent
	}

	fmt.Println("Enter your broker fee percentage (%) value:")
	feePercentage, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("0.06"),
		components.TextInputOptValue(value),
		components.TextInputOptEnding("%"),
	)
	if err != nil {
		return "", err
	}
	tbpsFromPercentage, err := convertPercentToTBPS(feePercentage)
	if err != nil {
		fmt.Println(styles.ErrorText.Render("invalid tbps value: " + err.Error()))
		return c.CollectBrokerFee()
	}

	// Persist user input
	cfg.BrokerServerConfig.FeeInputPercent = feePercentage
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return "", err
	}

	return tbpsFromPercentage, nil
}

// GetChainId attempts to retrieve the chain id from config, if that is not
// possible, prompts use to enter it and stores the value in config
func (c *InputCollector) GetChainId(cfg *configs.D8XConfig, ctx *cli.Context) (uint, error) {
	if cfg.ChainId != 0 {
		// Do not ask for chain id if it was already entered in current session
		if c.chainIdSelected {
			return cfg.ChainId, nil
		}

		c.chainIdSelected = true

		info := fmt.Sprintf("Currently using chain id: %d. Keep using this chain id?", cfg.ChainId)
		keep, err := c.TUI.NewPrompt(info, true)
		if err != nil {
			return 0, err
		}
		if keep {
			return cfg.ChainId, nil
		}
	}

	fmt.Println("Select chain id:")
	// Allow to input chain id if DEBUG_ALLOW_CHAINID_INPUT variable is set
	var chainId string
	if _, allowInput := os.LookupEnv("DEBUG_ALLOW_CHAINID_INPUT"); !allowInput {
		chains, err := c.TUI.NewSelection(ALLOWED_CHAINS_STRINGS, components.SelectionOptAllowOnlySingleItem(), components.SelectionOptRequireSelection())
		if err != nil {
			return 0, err
		}
		chainId = ALLOWED_CHAINS_MAP[chains[0]]
	} else {
		chain, err := c.TUI.NewInput(components.TextInputOptPlaceholder("1101"))
		if err != nil {
			return 0, err
		}
		chainId = chain
	}

	chainIdUint, err := strconv.Atoi(chainId)
	if err != nil {
		return 0, err
	}

	c.chainIdSelected = true
	cfg.ChainId = uint(chainIdUint)

	return cfg.ChainId, c.ConfigRWriter.Write(cfg)
}

func (c *InputCollector) CollectAndValidateWalletAddress(title, value string) (string, error) {
	fmt.Println(title)
	walletAddress, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("0x0000000000000000000000000000000000000000"),
		components.TextInputOptValue(value),
		components.TextInputOptDenyEmpty(),
	)
	walletAddress = strings.TrimSpace(walletAddress)
	if err != nil {
		return "", err
	}

	// Validate the address
	if !ValidWalletAddress(walletAddress) {
		fmt.Println(styles.ErrorText.Render("invalid address provided, please try again..."))
		return c.CollectAndValidateWalletAddress(title, value)
	}

	return walletAddress, nil
}

func (c *InputCollector) CollectDatabaseDSN(cfg *configs.D8XConfig) error {
	change := true
	if cfg.DatabaseDSN != "" {
		info := fmt.Sprintf("Found DATABASE_DSN=%s\nDo you want keep it?", cfg.DatabaseDSN)
		keep, err := c.TUI.NewPrompt(info, true)
		if err != nil {
			return err
		}
		change = !keep
	}

	if !change {
		return nil
	}

	// Validate database protocol prefix, and password if any special
	// characters are present
	dsnValidator := func(dbConnStr string) error {
		if !strings.HasPrefix(dbConnStr, "postgres://") && !strings.HasPrefix(dbConnStr, "postgresql://") {
			return fmt.Errorf("connection string must start with postgres:// or postgresql://")
		}

		connUrl, err := dburl.Parse(
			dbConnStr,
		)
		if err != nil {
			return err
		}

		if connUrl.User == nil {
			return fmt.Errorf("user:password part is missing")
		}

		password, set := connUrl.User.Password()
		if !set {
			return fmt.Errorf("password is missing")
		}
		specialCharacters := []byte{'*', '?', '$', '(', ')', '`', '\\', '\'', '"', '>', '<', '|', '&'}

		for _, char := range specialCharacters {
			if bytes.Contains([]byte(password), []byte{char}) {
				return fmt.Errorf("password contains special character %s, please use password without special characters", string(char))
			}
		}

		return nil
	}

	switch cfg.ServerProvider {
	// Linode users must enter their own database dns stirng manually
	case configs.D8XServerProviderLinode:
		for {
			fmt.Println("Enter your database dsn connection string:")
			dbDsn, err := c.TUI.NewInput(
				components.TextInputOptPlaceholder("postgresql://user:password@host:5432/postgres"),
				components.TextInputOptDenyEmpty(),
			)
			if err != nil {
				return err
			}
			dbDsn = strings.TrimSpace(dbDsn)

			if err := dsnValidator(dbDsn); err != nil {
				fmt.Println(styles.ErrorText.Render("Invalid database connection string, please try again: " + err.Error()))
			} else {
				cfg.DatabaseDSN = dbDsn
				break
			}
		}

	// For AWS - read it from rds credentials file
	case configs.D8XServerProviderAWS:
		if err := collectAwsRdsDsnString(cfg); err != nil {
			return err
		}
	}

	return c.ConfigRWriter.Write(cfg)
}

// collectAwsRdsDsnString collects database dsn string from AWS RDS credentials
// file and adds it to the given cfg. If RDS_CREDS_FILE does not exist yet it
// will not return an error and will silently fail.
func collectAwsRdsDsnString(cfg *configs.D8XConfig) error {
	creds, err := os.ReadFile(RDS_CREDS_FILE)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	credsMap := parseAwsRDSCredentialsFile(creds)
	cfg.DatabaseDSN = fmt.Sprintf("postgresql://%s:%s@%s:%s/postgres",
		credsMap["user"],
		credsMap["password"],
		credsMap["host"],
		credsMap["port"],
	)

	return nil
}

func (c *InputCollector) CollecteBrokerPayoutAddress(cfg *configs.D8XConfig) error {
	// Collect referrral broker payout address
	changeReferralPayoutAddress := true
	if cfg.ReferralConfig.BrokerPayoutAddress != "" {
		fmt.Printf("Found referralSettings.json broker payout address: %s\n", cfg.ReferralConfig.BrokerPayoutAddress)
		if keep, err := c.TUI.NewPrompt("Do you want to keep this broker payout address?", true); err != nil {
			return err
		} else if keep {
			changeReferralPayoutAddress = false
		}
	}
	if changeReferralPayoutAddress {
		info := "Enter broker payout address:\n"
		info = info + styles.GrayText.Render("See config README (live.referralSettings.json) for more info: \nhttps://github.com/D8-X/d8x-cli/blob/main/README_CONFIG.md\n")

		brokerPayoutAddress, err := c.CollectAndValidateWalletAddress(info, cfg.ReferralConfig.BrokerPayoutAddress)
		if err != nil {
			return err
		}
		cfg.ReferralConfig.BrokerPayoutAddress = brokerPayoutAddress

		return c.ConfigRWriter.Write(cfg)
	}
	return nil
}

// CollectAndValidatePrivateKey prompts user to enter a private key, validates
// it, displays the address of entered key and prompts user to confirm that
// entered key's address is correct. If any of the validation or
// confirmation steps fail, it will restart the collection process. Returned
// values are private key without 0x prefix and its address.
func (input *InputCollector) CollectAndValidatePrivateKey(title string) (string, string, error) {
	fmt.Println(title)
	pk, err := input.TUI.NewInput(
		components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
		components.TextInputOptMasked(),
		components.TextInputOptDenyEmpty(),
	)
	if err != nil {
		return "", "", err
	}
	pk = strings.TrimPrefix(pk, "0x")
	addr, err := PrivateKeyToAddress(pk)
	if err != nil {
		info := fmt.Sprintf("Invalid private key, please try again...\n - %s\n", err.Error())
		fmt.Println(styles.ErrorText.Render(info))
		return input.CollectAndValidatePrivateKey(title)
	}

	fmt.Printf("Wallet address of entered private key: %s\n", addr.Hex())

	ok, err := input.TUI.NewPrompt("Is this the correct address?", true)
	if err != nil {
		return "", "", err
	}

	if !ok {
		return input.CollectAndValidatePrivateKey(title)
	}

	return pk, addr.Hex(), nil
}

// CollectSetupDomain collects domain name and stores it in config
func (c *InputCollector) CollectSetupDomain(cfg *configs.D8XConfig) (string, error) {
	if c.setup.setupDomainEntered {
		return cfg.SetupDomain, nil
	}

	fmt.Println("Enter your domain name:")
	domain, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("your-domain.com"),
		components.TextInputOptValue(cfg.SetupDomain),
		components.TextInputOptDenyEmpty(),
	)
	if err != nil {
		return "", err
	}
	domain = TrimHttpsPrefix(domain)

	cfg.SetupDomain = domain
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return "", err
	}

	c.setup.setupDomainEntered = true

	return domain, nil
}

// CollectInputWithConfirmation shows an input field and when users fills it,
// shows a confirmation
func (c *InputCollector) CollectInputWithConfirmation(inputTitle, confirmationTitle string, inputOpts ...components.TextInputOpt) (string, error) {
	fmt.Println(inputTitle)
	input, err := c.TUI.NewInput(
		inputOpts...,
	)
	if err != nil {
		return "", err
	}

	fmt.Printf("You have entered: %s\n", input)

	correct, err := c.TUI.NewPrompt(confirmationTitle, true)
	if err != nil {
		return "", err
	}
	// Try again
	if !correct {
		return c.CollectInputWithConfirmation(inputTitle, confirmationTitle, inputOpts...)
	}

	return input, nil
}

func (c *InputCollector) CollectCertbotEmail(cfg *configs.D8XConfig) (string, error) {
	if c.setup.certbotEmailEntered {
		return cfg.CertbotEmail, nil
	}

	change := true
	if cfg.CertbotEmail != "" {
		fmt.Printf("Email for certbot notifications is set to %s\n", cfg.CertbotEmail)
		keep, err := c.TUI.NewPrompt("Do you want to keep it?", true)
		if err != nil {
			return "", err
		}
		if keep {
			change = false
		}
	}

	if !change {
		return cfg.CertbotEmail, nil
	}

	fmt.Println("Enter your email address for certbot notifications: ")
	email, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("my-email@domain.com"),
		components.TextInputOptDenyEmpty(),
	)
	if err != nil {
		return "", err
	}
	cfg.CertbotEmail = email

	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return "", err
	}

	c.setup.certbotEmailEntered = true

	return cfg.CertbotEmail, nil
}

// GetServerProviderConfigurer retrieves collected server provider configurer
func (input *InputCollector) GetServerProviderConfigurer() ServerProviderConfigurer {
	switch input.provisioning.selectedServerProvider {
	case ServerProviderLinode:
		return input.provisioning.collectedLinodeConfigurer
	case ServerProviderAws:
		return input.provisioning.collectedAwsConfigurer
	}

	return nil
}

// SwarmNginxCollectDomains collects hostnames information.
func (c *InputCollector) SwarmNginxCollectDomains(cfg *configs.D8XConfig) ([]hostnameTuple, error) {
	hosts := make([]string, len(hostsTpl))
	replacements := make([]files.ReplacementTuple, len(hostsTpl))
	for i, h := range hostsTpl {

		// When possible, find values from config for non-first time runs.
		// Provide some automatic subdomain suggestions by default
		value := cfg.SuggestSubdomain(h.serviceName, c.ChainJson.GetChainType(strconv.Itoa(int(cfg.ChainId))))
		if v, ok := cfg.Services[h.serviceName]; ok {
			if v.HostName != "" {
				value = v.HostName
			}
		}

		input, err := c.CollectInputWithConfirmation(
			h.prompt,
			"Is this the correct domain you want to use for "+string(h.serviceName)+"?",
			components.TextInputOptPlaceholder(h.placeholder),
			components.TextInputOptValue(value),
			components.TextInputOptDenyEmpty(),
		)
		if err != nil {
			return nil, err
		}
		input = TrimHttpsPrefix(input)
		hostsTpl[i].server = input
		replacements[i] = files.ReplacementTuple{
			Find:    h.find,
			Replace: input,
		}
		hosts[i] = input

		fmt.Printf("Using domain %s for %s\n\n", input, h.serviceName)
	}

	c.swarmNginxInput.collectedServiceDomains = hostsTpl

	return hostsTpl, nil
}

// EnsureSSHKeyPresent prompts user to create or override new ssh key pair in
// default provided sshKeyPath location. Config cfg is updated with changed
// SSHKeyMd5 hash on key change, but updates are not persisted to disk, only to
// cfg object.
func (c *InputCollector) EnsureSSHKeyPresent(sshKeyPath string, cfg *configs.D8XConfig) error {
	// By default, we assume key exists, if it doesn't - we will create it
	// without prompting for users's constent, otherwise we prompt for consent.
	createKey := false
	_, err := os.Stat(sshKeyPath)
	if err != nil {
		fmt.Printf("SSH key %s was not found, creating new one...\n", sshKeyPath)
		createKey = true
	} else {
		keep, err := c.TUI.NewPrompt(
			fmt.Sprintf("SSH key %s was found, do you want to keep it? (Changing keypair forces servers re-creation)", sshKeyPath),
			true,
		)
		if err != nil {
			return err
		}

		if !keep {
			createKey = true
		}
	}

	if createKey {
		fmt.Println(
			"Executing:",
			styles.ItalicText.Render(
				fmt.Sprintf("ssh-keygen -t ed25519 -f %s", sshKeyPath),
			),
		)
		keygenCmd := "yes | ssh-keygen -N \"\" -t ed25519 -C d8xtrader -f " + sshKeyPath
		cmd := exec.Command("bash", "-c", keygenCmd)
		connectCMDToCurrentTerm(cmd)
		if err := cmd.Run(); err != nil {
			return err
		}

		// Update md5 hash of private key
		h := md5.New()
		privateKey, err := os.ReadFile(sshKeyPath)
		if err != nil {
			return fmt.Errorf("reading private key: %w", err)
		}
		if _, err := h.Write(privateKey); err != nil {
			return err
		}
		md5Hash := fmt.Sprintf("%x", h.Sum(nil))
		if md5Hash != cfg.SSHKeyMD5 {
			c.sshKeyChanged = true
		}
		cfg.SSHKeyMD5 = md5Hash
	}
	return nil
}
