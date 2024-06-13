package actions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/actions/contracts"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/urfave/cli/v2"
)

// Stack name that will be used when creating/destroying or managing swarm
// cluster deployment.
// TODO - store this in config and make this configurable via flags
var dockerStackName = "stack"

// NginxConfigSection defines a comment section that can be uncommented via
// processNginxConfigComments. Section starts with {NginxConfigSection} and ends
// with {/NginxConfigSection}. All lines starting with # will be trimmed and
// replaced in between these tags.
type NginxConfigSection string

const (
	RealIpCloudflare NginxConfigSection = "real_ip_cloudflare"
)

// EditSwarmEnv edits the .env file for swarm deployment with user provided and
// provisioning values.
func (c *Container) EditSwarmEnv(envPath string, cfg *configs.D8XConfig) error {
	// Edit .env file
	fmt.Println(styles.ItalicText.Render("Editing .env file..."))
	envFile, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("reading .env file: %w", err)
	}

	envFileLines := strings.Split(string(envFile), "\n")

	// We assume that all cfg values are present at this point
	findReplaceOrCreateEnvs := map[string]string{
		"NETWORK_NAME":       c.cachedChainJson.getChainPriceFeedName(strconv.Itoa(int(cfg.ChainId))),
		"SDK_CONFIG_NAME":    c.cachedChainJson.getChainSDKName(strconv.Itoa(int(cfg.ChainId))),
		"CHAIN_ID":           strconv.Itoa(int(cfg.ChainId)),
		"REDIS_PASSWORD":     cfg.SwarmRedisPassword,
		"REMOTE_BROKER_HTTP": cfg.SwarmRemoteBrokerHTTPUrl,
		"DATABASE_DSN":       cfg.DatabaseDSN,
	}

	// List of envs that were not found in .env but will be added to the output
	prependEnvs := []string{}

	// Process the env file and append collected .env values
	for env, value := range findReplaceOrCreateEnvs {
		envFound := false
		envVal := env + "=" + value
		fmt.Printf("Setting %s \n", envVal)
		for lineIndex, line := range envFileLines {
			if strings.HasPrefix(line, env) {
				envFound = true
				envFileLines[lineIndex] = envVal
				break
			}
		}
		if !envFound {
			prependEnvs = append(prependEnvs, envVal)
		}
	}
	if len(prependEnvs) > 0 {
		envFileLines = append(prependEnvs, envFileLines...)
	}

	// Write the env output
	return c.FS.WriteFile(envPath, []byte(strings.Join(envFileLines, "\n")))
}

// UpdateReferralSettings is an updateFn for UpdateConfig for referral settings
// json file
func UpdateReferralSettingsBrokerPayoutAddress(brokerPayoutAddress string, chainId int) func(referralSettings *[]map[string]any) error {
	return func(referralSettings *[]map[string]any) error {
		referralSettingsV := *referralSettings
		for i, refSetting := range referralSettingsV {
			if int(refSetting["chainId"].(float64)) == chainId {
				refSetting["brokerPayoutAddr"] = brokerPayoutAddress
				(*referralSettings)[i] = refSetting
				break
			}
		}
		return nil
	}
}

// UpdateCandlesPriceConfigPriceServices is an updateFn for UpdateConfig for
// candles prices config files
func UpdateCandlesPriceConfigPriceServices(priceServiceHTTPSEndpoints []string) func(pricesConf *map[string]any) error {
	// Delete empty values just in case
	priceServiceHTTPSEndpoints = slices.DeleteFunc(priceServiceHTTPSEndpoints, func(s string) bool {
		return s == ""
	})

	return func(pricesConf *map[string]any) error {
		(*pricesConf)["priceServiceHTTPSEndpoints"] = priceServiceHTTPSEndpoints
		return nil
	}
}

var swarmDeployConfigFilesToCopy = []files.EmbedCopierOp{
	// Trader backend configs
	// Note that .env.example is not recognized in embed.FS
	{Src: "embedded/trader-backend/env.example", Dst: "./trader-backend/.env", Overwrite: false},
	{Src: "embedded/trader-backend/live.referralSettings.json", Dst: "./trader-backend/live.referralSettings.json", Overwrite: false},
	{Src: "embedded/trader-backend/rpc.main.json", Dst: "./trader-backend/rpc.main.json", Overwrite: false},
	{Src: "embedded/trader-backend/rpc.referral.json", Dst: "./trader-backend/rpc.referral.json", Overwrite: false},
	{Src: "embedded/trader-backend/rpc.history.json", Dst: "./trader-backend/rpc.history.json", Overwrite: false},
	// Candles configs
	{Src: "embedded/candles/prices.config.json", Dst: "./candles/prices.config.json", Overwrite: false},

	// Docker swarm file - do not overwrite and allow user to modify the config
	// (for example choose specific image manually).
	{Src: "embedded/docker-swarm-stack.yml", Dst: "./docker-swarm-stack.yml", Overwrite: false},
}

func (c *Container) CopySwarmDeployConfigs() error {
	if err := c.EmbedCopier.Copy(configs.EmbededConfigs, swarmDeployConfigFilesToCopy...); err != nil {
		return fmt.Errorf("copying configs to local file system: %w", err)
	}
	return nil
}

func (c *Container) SwarmDeploy(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting swarm cluster deployment...")

	if err := c.swarmDeploy(ctx, true); err != nil {
		return err
	}

	// After swarm deployment is completed, check if ingress network is working
	// correctly on manager. Repeat for 2 times max
	ingressWorks := false
	for i := 0; i < 2; i++ {
		fmt.Printf("Checking ingress network on manager... (attempt %d/2)\n", i+1)
		err := c.CheckSwarmIngressIsCorrect(ctx)
		if err != nil {
			fmt.Println(styles.ErrorText.Render(err.Error()))
			if err := c.IngressFix(ctx); err != nil {
				fmt.Println(styles.SuccessText.Render(fmt.Sprintf("Ingress network fix failed: %s\n", err.Error())))
				continue
			}

			// Redeploy the swarm after ingress fix
			fmt.Println(styles.ItalicText.Render("Redeploying swarm services after ingress fix..."))
			if err := c.swarmDeploy(ctx, false); err != nil {
				return err
			}

		} else {
			fmt.Println(styles.SuccessText.Render("Ingress network is working correctly on manager"))
			ingressWorks = true
			break
		}
	}

	if !ingressWorks {
		fmt.Println(styles.ErrorText.Render("Ingress network is not working correctly on manager"))
		fmt.Println("Automatic fix failed, please try to run fix-ingress and setup swarm-deploy manually")
	}

	return nil
}

// swarmDeploy performs the swarm deployment step
func (c *Container) swarmDeploy(ctx *cli.Context, showConfigConfirmation bool) error {

	// Find manager ip before we start collecting data in case manager is not
	// available.
	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return fmt.Errorf("finding manager ip address: %w", err)
	}

	if err := c.Input.CollectSwarmDeployInputs(ctx); err != nil {
		return err
	}

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Copy embed files before starting
	if err := c.CopySwarmDeployConfigs(); err != nil {
		return err
	}

	if c.Input.swarmDeployInput.guideConfig {
		// Update .env file
		if err := c.EditSwarmEnv("./trader-backend/.env", cfg); err != nil {
			return fmt.Errorf("editing .env file: %w", err)
		}

		// Update rpcconfigs
		for i, rpconfigFilePath := range []string{
			"./trader-backend/rpc.main.json",
			"./trader-backend/rpc.history.json",
			"./trader-backend/rpc.referral.json",
		} {
			httpRpcs, wsRpcs := DistributeRpcs(
				i,
				strconv.Itoa(int(cfg.ChainId)),
				cfg,
			)

			fmt.Printf("Updating %s config...\n", rpconfigFilePath)

			// No wsRPC for referral
			if i == 2 {
				wsRpcs = nil
			}

			if err := c.editRpcConfigUrls(rpconfigFilePath, cfg.ChainId, wsRpcs, httpRpcs); err != nil {
				fmt.Println(
					styles.ErrorText.Render(
						fmt.Sprintf("Could not update %s, please double check the config file: %+v", rpconfigFilePath, err),
					),
				)
			}
		}

		// Update referralSettings
		if err := UpdateConfig[[]map[string]any](
			"./trader-backend/live.referralSettings.json",
			UpdateReferralSettingsBrokerPayoutAddress(cfg.ReferralConfig.BrokerPayoutAddress, int(cfg.ChainId)),
		); err != nil {
			return fmt.Errorf("updating referralSettings.json: %w", err)
		}
		// Validate token X to be a valid erc-20 in live.referralSettings.json
		refSettings, err := os.Open("./trader-backend/live.referralSettings.json")
		if err != nil {
			return err
		}
		defer refSettings.Close()
		fmt.Println("Validating selected tokenX contract...")
		if err := c.validateReferralConfigTokenX(refSettings, cfg); err != nil {
			// This error is not critical and should not stop the deployment
			fmt.Println(styles.ErrorText.Render(fmt.Sprintf("validating tokenX contract: %s", err)))
		}

		// Update price configs with provided pyth https endpoints. Remove any
		// duplicates and ensure that the default pyth endpoint is appended
		// last.
		userProvidedHttpEndpoints := c.Input.swarmDeployInput.priceServiceHttpEndpoints
		slices.Sort(userProvidedHttpEndpoints)
		userProvidedHttpEndpoints = slices.Compact(userProvidedHttpEndpoints)
		defaultHttpEndpoint := c.cachedChainJson.getDefaultPythHTTPSEndpoint(strconv.Itoa(int(cfg.ChainId)))
		priceServiceHTTPSEndpoints := userProvidedHttpEndpoints
		if !slices.Contains(priceServiceHTTPSEndpoints, defaultHttpEndpoint) {
			priceServiceHTTPSEndpoints = append(priceServiceHTTPSEndpoints, defaultHttpEndpoint)
		}
		priceServiceHTTPSEndpoints = slices.Compact(priceServiceHTTPSEndpoints)

		if err := UpdateConfig(
			"./candles/prices.config.json",
			UpdateCandlesPriceConfigPriceServices(priceServiceHTTPSEndpoints),
		); err != nil {
			return err
		}
	}

	// Collected input data
	pk := c.Input.swarmDeployInput.referralPaymentExecutorPrivateKey
	pkWalletAddress := c.Input.swarmDeployInput.referralPaymentExecutorWalletAddress

	// Check if user provided broker allowed executor pk's address matches
	// with values in broker/chainConfig.json and report if not
	if cfg.BrokerDeployed {
		allowedExecutorAddrs, err := c.GetBrokerChainConfigJsonAllowedExecutors("./broker-server/chainConfig.json", cfg)
		if err != nil {
			return fmt.Errorf("reading ./broker-server/chainConfig.json: %w", err)
		}
		matchFound := false
		for _, allowedAddr := range allowedExecutorAddrs {
			if strings.EqualFold(strings.TrimSpace(pkWalletAddress), strings.TrimSpace(allowedAddr)) {
				matchFound = true
				break
			}
		}
		if !matchFound {
			// Allowed executor was either different or not found for selected chain
			fmt.Println(
				styles.ErrorText.Render(
					"provided referral executor address did not match any allowedExecutor address for chain id" + strconv.Itoa(int(cfg.ChainId)) + " in ./broker-server/chainConfig.json",
				),
			)
		}
	}

	keyfileLocal := "./trader-backend/keyfile.txt"
	if err := c.FS.WriteFile(keyfileLocal, []byte("0x"+pk)); err != nil {
		return fmt.Errorf("temp storage of keyfile failed: %w", err)
	}

	if showConfigConfirmation {
		fmt.Println(styles.AlertImportant.Render("Please verify your .env and configuration files are correct before proceeding."))
		fmt.Println("The following configuration files will be copied to the 'manager node' for the d8x-trader-backend swarm deployment:")
		for _, f := range swarmDeployConfigFilesToCopy[:6] {
			fmt.Println(f.Dst)
		}
		c.TUI.NewConfirmation("Press enter to confirm that the configuration files listed above are good to go...")
	}

	pwd, err := c.GetPassword(ctx)
	if err != nil {
		return err
	}

	managerSSHConn, err := c.CreateSSHConn(
		managerIp,
		c.DefaultClusterUserName,
		c.SshKeyPath,
	)
	if err != nil {
		return err
	}

	// Stack might exist, prompt user to remove it
	if _, err := managerSSHConn.ExecCommand(
		"echo '" + pwd + "'| sudo -S docker stack ls | grep " + dockerStackName + " >/dev/null 2>&1",
	); err == nil {
		ok, err := c.TUI.NewPrompt("\nThere seems to be an existing stack deployed. Do you want to remove it before redeploying?", true)
		if err != nil {
			return err
		}
		if ok {
			fmt.Println(styles.ItalicText.Render("Removing existing stack..."))
			out, err := managerSSHConn.ExecCommand(
				fmt.Sprintf(`docker stack rm %s`, dockerStackName),
			)
			fmt.Println(string(out))
			if err != nil {
				return fmt.Errorf("removing existing stack: %w", err)
			}
		}
	}

	ipWorkers, err := c.HostsCfg.GetWorkerIps()
	if err != nil {
		return fmt.Errorf("finding worker ip addresses: %w", err)
	}
	ipMgrPriv, err := c.HostsCfg.GetMangerPrivateIp()
	if err != nil {
		return err
	}
	ipWorkersPriv, err := c.HostsCfg.GetWorkerPrivateIps()
	if err != nil {
		return err
	}
	fmt.Println(styles.ItalicText.Render("Creating NFS Config..."))
	cmd := fmt.Sprintf(`echo '%s' | sudo -S bash -c "mkdir /var/nfs/general -p && chown nobody:nogroup /var/nfs/general" `, pwd)
	configEtcExports := "#"
	for _, ip := range ipWorkersPriv {
		cmdUfw := fmt.Sprintf(`&& echo '%s' | sudo -S bash -c "ufw allow from %s to any port nfs" `, pwd, ip)
		cmd = cmd + cmdUfw
		configEtcExports = configEtcExports + "\n" + fmt.Sprintf(`/var/nfs/general %s(rw,sync,no_subtree_check)`, ip)
	}
	_, err = managerSSHConn.ExecCommand(
		cmd,
	)
	if err != nil {
		return fmt.Errorf("NFS preparation on manager failed : %w", err)
	}
	if err := c.FS.WriteFile("./trader-backend/exports", []byte(configEtcExports)); err != nil {
		return fmt.Errorf("temp storage of /etc/exports file failed: %w", err)
	}

	managedConfigNames := []string{
		"cfg_rpc",
		"cfg_rpc_referral",
		"cfg_rpc_history",
		"cfg_referral",
		"cfg_prices",
	}
	// Lines of docker config commands which we will concat into single
	// bash -c ssh call
	dockerConfigsCMD := []string{
		`docker config create cfg_rpc ./trader-backend/rpc.main.json >/dev/null 2>&1`,
		`docker config create cfg_rpc_referral ./trader-backend/rpc.referral.json >/dev/null 2>&1`,
		`docker config create cfg_rpc_history ./trader-backend/rpc.history.json >/dev/null 2>&1`,
		`docker config create cfg_referral ./trader-backend/live.referralSettings.json >/dev/null 2>&1`,
		`docker config create cfg_prices ./candles/prices.config.json >/dev/null 2>&1`,

		// `docker config create prometheus_config ./prometheus.yml >/dev/null 2>&1`,
	}

	// List of files to transfer to manager
	copyList := []conn.SftpCopySrcDest{
		{Src: "./trader-backend/.env", Dst: "./trader-backend/.env"},
		{Src: "./trader-backend/live.referralSettings.json", Dst: "./trader-backend/live.referralSettings.json"},
		{Src: "./trader-backend/rpc.main.json", Dst: "./trader-backend/rpc.main.json"},
		{Src: "./trader-backend/rpc.referral.json", Dst: "./trader-backend/rpc.referral.json"},
		{Src: "./trader-backend/rpc.history.json", Dst: "./trader-backend/rpc.history.json"},
		// Keyfile contains unencrypted private key
		{Src: "./trader-backend/keyfile.txt", Dst: "./trader-backend/keyfile.txt"},
		{Src: "./trader-backend/exports", Dst: "./trader-backend/exports"},
		{Src: "./candles/prices.config.json", Dst: "./candles/prices.config.json"},
		// Note we are renaming to docker-stack.yml on remote!
		{Src: "./docker-swarm-stack.yml", Dst: "./docker-stack.yml"},
	}

	// Copy files to remote
	fmt.Println(styles.ItalicText.Render("Copying configuration files to manager node " + managerIp))
	defer os.Remove(keyfileLocal)
	if err := managerSSHConn.CopyFilesOverSftp(
		copyList...,
	); err != nil {
		return fmt.Errorf("copying configuration files to manager: %w", err)
	} else {
		fmt.Println(styles.SuccessText.Render("configuration files copied to manager"))
	}

	// enable nfs server
	fmt.Println(styles.ItalicText.Render("Starting NFS server..."))
	cmd = fmt.Sprintf(`echo '%s' | sudo -S bash -c "mv ./trader-backend/keyfile.txt /var/nfs/general/keyfile.txt && chown nobody:nogroup /var/nfs/general/keyfile.txt && chmod 775 /var/nfs/general/keyfile.txt" && `, pwd)
	cmd = cmd + fmt.Sprintf(`echo '%s' | sudo -S bash -c "cp ./trader-backend/exports /etc/exports \
		&& systemctl restart nfs-kernel-server" `, pwd)
	_, err = managerSSHConn.ExecCommand(
		cmd,
	)
	if err != nil {
		return fmt.Errorf("Error starting NFS server: %w", err)
	}

	fmt.Println(styles.ItalicText.Render("Mounting NFS directories on workers..."))
	cmd = fmt.Sprintf(`echo '%s' | sudo -S bash -c "mkdir -p /nfs/general && mount %s:/var/nfs/general /nfs/general" `, pwd, ipMgrPriv)
	for k, ip := range ipWorkersPriv {
		fmt.Println(styles.ItalicText.Render("worker "), ip)
		var (
			sshConnWorker conn.SSHConnection
			err           error
		)
		if cfg.ServerProvider == configs.D8XServerProviderAWS {
			sshConnWorker, err = conn.NewSSHConnectionWithBastion(
				managerSSHConn.GetClient(),
				ipWorkers[k],
				c.DefaultClusterUserName,
				c.SshKeyPath,
			)
		} else {
			sshConnWorker, err = c.CreateSSHConn(
				ipWorkers[k],
				c.DefaultClusterUserName,
				c.SshKeyPath,
			)
		}
		if err != nil {
			return err
		}
		_, err = sshConnWorker.ExecCommand(
			cmd,
		)
		if err != nil {
			return fmt.Errorf("failed to mount nfs dir on worker: %w", err)
		}
	}

	// Recreate configs
	fmt.Println(styles.ItalicText.Render("Creating docker configs..."))
	out, err := managerSSHConn.ExecCommand(
		"echo -e '" + strings.Join(managedConfigNames, "\n") + `' | while read -r configname; do docker config rm "$configname"; done;` + strings.Join(dockerConfigsCMD, ";"),
	)
	fmt.Println(string(out))
	if err != nil {
		return fmt.Errorf("creating docker configs: %w", err)
	}
	fmt.Println(styles.SuccessText.Render("docker configs were created on manager node!"))

	// docker volumes
	fmt.Println(styles.ItalicText.Render("Preparing Docker volumes..."))

	fmt.Printf("\nPrivate ip : %s\n", ipMgrPriv)
	cmd = fmt.Sprintf(`docker volume create --driver local --opt type=nfs4 --opt o=addr=%s,rw --opt device=:/var/nfs/general nfsvol`, ipMgrPriv)
	out, err = managerSSHConn.ExecCommand(
		cmd,
	)
	if err != nil {
		fmt.Println(string(out))
		return err
	}
	// create volume on worker nodes

	cmd = fmt.Sprintf(
		`docker volume create --driver local --opt type=nfs4 --opt o=addr=%s,rw --opt device=:/var/nfs/general nfsvol`,
		ipMgrPriv,
	)
	cmdDir := fmt.Sprintf(
		`echo '%s' | sudo -S bash -c "mkdir -p /nfs/general && mount %s:/var/nfs/general /nfs/general"`,
		pwd,
		ipMgrPriv,
	)
	for _, ip := range ipWorkers {
		var (
			sshConnWorker conn.SSHConnection
			err           error
		)
		if cfg.ServerProvider == configs.D8XServerProviderAWS {
			sshConnWorker, err = conn.NewSSHConnectionWithBastion(
				managerSSHConn.GetClient(),
				ip,
				c.DefaultClusterUserName,
				c.SshKeyPath,
			)
		} else {
			sshConnWorker, err = c.CreateSSHConn(
				ip,
				c.DefaultClusterUserName,
				c.SshKeyPath,
			)
		}
		if err != nil {
			return err
		}
		_, err = sshConnWorker.ExecCommand(
			cmdDir,
		)
		if err != nil {
			fmt.Println(string(out))
			return fmt.Errorf("failed to create nfs dir on worker: %w", err)
		}
		_, err = sshConnWorker.ExecCommand(
			cmd,
		)
		if err != nil {
			fmt.Println(string(out))
			return fmt.Errorf("creating volume on worker failed: %w", err)
		}
	}

	// Deploy swarm stack
	fmt.Println(styles.ItalicText.Render("Deploying docker swarm via manager node..."))
	swarmDeployCMD := fmt.Sprintf(
		`echo '%s' | sudo -S bash -c "docker compose --env-file ./trader-backend/.env -f ./docker-stack.yml config | sed -E 's/published: \"([0-9]+)\"/published: \1/g' | sed -E 's/^name: .*$/ /'|  docker stack deploy -c - %s"`,
		pwd,
		dockerStackName,
	)
	out, err = managerSSHConn.ExecCommand(swarmDeployCMD)
	fmt.Println(string(out))
	if err != nil {
		return fmt.Errorf("swarm deployment failed: %w", err)
	}
	fmt.Println(styles.SuccessText.Render("D8X-trader-backend swarm was deployed"))

	// Update config
	cfg.SwarmDeployed = true
	return c.ConfigRWriter.Write(cfg)
}

func (c *Container) SwarmNginx(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting swarm nginx and certbot setup...")

	if err := c.Input.CollectSwarmNginxInputs(ctx); err != nil {
		return err
	}

	// Load config which we will later use to write details about services.
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Copy nginx config and ansible playbook for swarm nginx setup
	if err := c.EmbedCopier.Copy(
		configs.EmbededConfigs,
		files.EmbedCopierOp{Src: "embedded/nginx/nginx.conf", Dst: "./nginx/nginx.conf", Overwrite: true},
		files.EmbedCopierOp{Src: "embedded/nginx/nginx.server.conf", Dst: "./nginx.server.conf", Overwrite: true},
		files.EmbedCopierOp{Src: "embedded/playbooks/nginx.ansible.yaml", Dst: "./playbooks/nginx.ansible.yaml", Overwrite: true},
	); err != nil {
		return err
	}

	password, err := c.GetPassword(ctx)
	if err != nil {
		return err
	}

	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}

	setupCertbot := c.Input.swarmNginxInput.setupCertbot
	emailForCertbot := cfg.CertbotEmail
	services := c.Input.swarmNginxInput.collectedServiceDomains

	replacements := make([]files.ReplacementTuple, len(services))
	for i, svc := range services {
		replacements[i] = files.ReplacementTuple{
			Find:    svc.find,
			Replace: svc.server,
		}
	}
	fmt.Println(styles.ItalicText.Render("Generating nginx.conf for swarm manager..."))
	// Replace server_name's in nginx.conf
	if err := c.FS.ReplaceAndCopy(
		"./nginx/nginx.conf",
		"./nginx.configured.conf",
		replacements,
	); err != nil {
		return err
	}

	fmt.Println(
		styles.AlertImportant.Render(
			"Please create the following DNS records on your domain provider's website now:",
		),
	)
	for _, svc := range services {
		fmt.Printf("Hostname: %s\tType: A\tIP: %s\n", svc.server, managerIp)
	}
	c.TUI.NewConfirmation("\nPress enter when done...")

	// Hostnames - domains list provided for certbot
	hostnames := make([]string, len(services))
	for i, svc := range services {
		hostnames[i] = svc.server
		// Store services in d8x config
		cfg.Services[svc.serviceName] = configs.D8XService{
			Name:      svc.serviceName,
			UsesHTTPS: setupCertbot,
			HostName:  svc.server,
		}
	}

	// Run ansible-playbook for nginx setup on broker server
	args := []string{
		"--extra-vars", fmt.Sprintf(`ansible_ssh_private_key_file='%s'`, c.SshKeyPath),
		"--extra-vars", "ansible_host_key_checking=false",
		"--extra-vars", fmt.Sprintf(`ansible_become_pass='%s'`, password),
		"-i", configs.DEFAULT_HOSTS_FILE,
		"-u", c.DefaultClusterUserName,
		"./playbooks/nginx.ansible.yaml",
	}
	cmd := exec.Command("ansible-playbook", args...)
	connectCMDToCurrentTerm(cmd)
	if err := c.RunCmd(cmd); err != nil {
		return err
	} else {
		fmt.Println(styles.SuccessText.Render("Manager node nginx setup done!"))

		// Update sate
		cfg.SwarmNginxDeployed = true
	}

	if setupCertbot {
		fmt.Println(styles.ItalicText.Render("Setting up ssl certificates with certbot..."))
		sshConn, err := c.CreateSSHConn(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
		if err != nil {
			return err
		}

		out, err := c.certbotNginxSetup(
			sshConn,
			password,
			emailForCertbot,
			hostnames,
		)
		fmt.Println(string(out))

		if err != nil {
			restart, err2 := c.TUI.NewPrompt("Certbot setup failed, do you want to restart the swarm-nginx setup?", true)
			if err2 != nil {
				return err2
			}
			if restart {
				return c.SwarmNginx(ctx)
			}
			return fmt.Errorf("certbot setup failed: %w", err)
		} else {
			fmt.Println(styles.SuccessText.Render("Manager server certificates setup done!"))

			cfg.SwarmCertbotDeployed = true
		}
	}

	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return fmt.Errorf("could not update config: %w", err)
	}

	return nil
}

// hostnames tuple for brevity (collecting data, prompts, replacements for
// nginx.conf)
type hostnameTuple struct {
	// server value is entered by user. It will be the domain or subdomain of
	// the service
	server      string
	prompt      string
	placeholder string
	// string pattern which will be replaced by server value
	find        string
	serviceName configs.D8XServiceName
}

// List of services which will be configured in nginx.conf
var hostsTpl = []hostnameTuple{
	{
		prompt:      "Enter Main HTTP (sub)domain: ",
		placeholder: "api.d8x.xyz",
		find:        "%main%",
		serviceName: configs.D8XServiceMainHTTP,
	},
	{
		prompt:      "Enter Main Websockets (sub)domain: ",
		placeholder: "ws.d8x.xyz",
		find:        "%main_ws%",
		serviceName: configs.D8XServiceMainWS,
	},
	{
		prompt:      "Enter History HTTP (sub)domain: ",
		placeholder: "history.d8x.xyz",
		find:        "%history%",
		serviceName: configs.D8XServiceHistory,
	},
	{
		prompt:      "Enter Referral HTTP (sub)domain: ",
		placeholder: "referral.d8x.xyz",
		find:        "%referral%",
		serviceName: configs.D8XServiceReferral,
	},
	{
		prompt:      "Enter Candlesticks Websockets (sub)domain: ",
		placeholder: "candles.d8x.xyz",
		find:        "%candles_ws%",
		serviceName: configs.D8XServiceCandlesWs,
	},
}

type NameIp struct {
	Name string `json:"name"`
	IP   string `json:"IP"`
}

// CheckSwarmIngressIsCorrect checks if swarm manager's ingress network
// configuration contains worker servers as peers and is in correct state
func (c *Container) CheckSwarmIngressIsCorrect(ctx *cli.Context) error {
	// Check if ingress's peers property contains all the workers on manager
	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}
	managerConn, err := conn.NewSSHConnection(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return err
	}

	out, err := managerConn.ExecCommand(`docker network inspect -f "{{json .Peers}}" ingress`)
	if err != nil {
		return err
	}
	peers := []NameIp{}
	if err := json.Unmarshal([]byte(out), &peers); err != nil {
		return fmt.Errorf("parsing docker network inspect output: %w", err)
	}

	// Check if all workers are present in peers list
	workerIps, err := c.HostsCfg.GetWorkerPrivateIps()
	if err != nil {
		return err
	}

	for _, workerIp := range workerIps {
		found := false
		for _, peer := range peers {
			if workerIp == peer.IP {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("worker with IP %s is not present in ingress network peers", workerIp)
		}
	}

	return nil
}

// validateReferralConfigTokenX validates if provided tokenX contract is a valid
// erc-20 contract for selected chain in cfg by checking its decimals.
func (c *Container) validateReferralConfigTokenX(liveReferralCfg io.Reader, cfg *configs.D8XConfig) error {
	selectedChain := strconv.Itoa(int(cfg.ChainId))

	cfgJson, err := io.ReadAll(liveReferralCfg)
	if err != nil {
		return fmt.Errorf("reading live.referralSettings.json: %w", err)
	}
	refCfg := []configs.ReferralSettingConfig{}
	if err := json.Unmarshal(cfgJson, &refCfg); err != nil {
		return fmt.Errorf("parsing live.referralSettings.json: %w", err)
	}
	var selectedChainRefCfg configs.ReferralSettingConfig
	for _, refCfg := range refCfg {
		if refCfg.ChainId == int(cfg.ChainId) {
			selectedChainRefCfg = refCfg
			break
		}
	}
	selectedTokenX := selectedChainRefCfg.TokenX.Address
	if selectedTokenX == "" {
		return fmt.Errorf("no tokenX address was provided in live.referralSettings.json")
	}

	httpRpcsList := cfg.HttpRpcList[selectedChain]
	if len(httpRpcsList) == 0 {
		return fmt.Errorf("no http rpcs were provided")
	}
	ec, err := ethclient.Dial(httpRpcsList[0])
	if err != nil {
		return fmt.Errorf("could not connect to rpc: %w", err)
	}

	erc20, err := contracts.NewERC20(common.HexToAddress(selectedTokenX), ec)
	if err != nil {
		return fmt.Errorf("could not initialize erc20 contract: %w", err)
	}
	decimals, err := erc20.Decimals(nil)
	if err != nil {
		return err
	}

	fmt.Println(
		styles.SuccessText.Render(
			fmt.Sprintf("TokenX contract %s is a valid erc-20 contract with %d decimals\n", selectedTokenX, decimals),
		),
	)

	return nil
}

// processNginxConfigComments enables (uncomments) provided enableSection in
// given nginxConf if that section can be found. Section starts with comment
// line and enableSection wrapped in curly braces {enableSection} and ends with
// a comment line and enableSection wrapped in curly braces with forward slach
// after first brace {/enableSection}. Nested sections are not supported, but
// multiple seqeantial ones are.
func processNginxConfigComments(nginxConf io.Reader, enableSection NginxConfigSection) ([]byte, error) {
	config, err := io.ReadAll(nginxConf)
	if err != nil {
		return nil, err
	}

	opened := false

	result := bytes.NewBuffer(nil)
	sc := bufio.NewScanner(bytes.NewReader(config))
	for sc.Scan() {
		line := sc.Text()
		writeLine := line
		line = strings.TrimSpace(line)

		// Check for section open/close tags first
		isSectionTag := false
		if strings.HasPrefix(line, "#") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			// Open tag
			if line == "{"+string(enableSection)+"}" {
				opened = true
				isSectionTag = true
			}
			// Close tag
			if line == "{/"+string(enableSection)+"}" {
				opened = false
				isSectionTag = true
			}
		}

		if opened && !isSectionTag {
			// Keep any whitespace or identation in place, simply remove the
			// initial comment(s) chars, but leave any other comments in place
			temp := strings.Builder{}
			hashFound := false
			lastHash := false
			for _, ch := range writeLine {
				if ch == '#' && (!hashFound || lastHash) {
					hashFound = true
					lastHash = true
					continue
				}
				lastHash = false
				temp.WriteRune(ch)
			}

			writeLine = temp.String()
		}

		if _, err := result.Write([]byte(writeLine + "\n")); err != nil {
			return nil, err
		}
	}

	return result.Bytes(), nil
}
