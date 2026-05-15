package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

// Stack name that will be used when creating/destroying or managing swarm
// cluster deployment.
// TODO - store this in config and make this configurable via flags
var dockerStackName = "stack"

// EditSwarmEnv edits the .env file for swarm deployment with user provided and
// provisioning values.
func (c *Container) EditSwarmEnv(envPath string, cfg *configs.D8XConfig) error {
	fmt.Println(styles.ItalicText.Render("Editing .env file..."))
	envFile, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("reading .env file: %w", err)
	}
	out, err := c.EditSwarmEnvBytes(envFile, cfg)
	if err != nil {
		return err
	}
	return c.FS.WriteFile(envPath, out)
}

func (c *Container) EditSwarmEnvBytes(envContent []byte, cfg *configs.D8XConfig) ([]byte, error) {
	envFileLines := strings.Split(string(envContent), "\n")
	findReplaceOrCreateEnvs := map[string]string{
		"SDK_CONFIG_NAME":    c.cachedChainJson.getChainSDKName(strconv.Itoa(int(cfg.ChainId))),
		"CHAIN_ID":           strconv.Itoa(int(cfg.ChainId)),
		"REDIS_PASSWORD":     cfg.SwarmRedisPassword,
		"REMOTE_BROKER_HTTP": cfg.SwarmRemoteBrokerHTTPUrl,
		"DATABASE_DSN":       cfg.DatabaseDSN,
	}
	prependEnvs := []string{}
	for env, value := range findReplaceOrCreateEnvs {
		if value == "" {
			continue
		}
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
	return []byte(strings.Join(envFileLines, "\n")), nil
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

func (c *Container) CopySwarmDeployConfigs() error {
	stagings := []struct {
		envRelPath, embeddedSrc, localPath string
	}{
		{"trader-backend/rpc.main.json", "embedded/trader-backend/rpc.main.json", "./trader-backend/rpc.main.json"},
		{"trader-backend/rpc.history.json", "embedded/trader-backend/rpc.history.json", "./trader-backend/rpc.history.json"},
		{"candles/prices.config.json", "embedded/candles/prices.config.json", "./candles/prices.config.json"},
		{"candles/rpc_conf.json", "embedded/candles/rpc_conf.json", "./candles/rpc_conf.json"},
		{"docker-swarm-stack.yml", "embedded/docker-swarm-stack.yml", "./docker-swarm-stack.yml"},
	}
	envData, err := configs.EmbededConfigs.ReadFile("embedded/trader-backend/env.example")
	if err != nil {
		return fmt.Errorf("reading embedded env.example: %w", err)
	}
	if err := os.MkdirAll("./trader-backend", 0755); err != nil {
		return err
	}
	if err := os.WriteFile("./trader-backend/.env", envData, 0644); err != nil {
		return err
	}
	for _, s := range stagings {
		if err := c.stageInfraRepoFile(s.envRelPath, s.embeddedSrc, s.localPath); err != nil {
			return fmt.Errorf("staging %s: %w", s.envRelPath, err)
		}
	}
	return nil
}

func (c *Container) importRemoteSwarmDeployConfig(ctx *cli.Context, managerIp string) error {
	remoteCfg, err := c.fetchRemoteSwarmDeployConfig(managerIp)
	if err != nil {
		return err
	}
	if remoteCfg == nil {
		return nil
	}

	if c.Input == nil {
		return nil
	}

	fmt.Println(styles.ItalicText.Render("Found existing deployed swarm config on manager."))
	keep, err := c.TUI.NewPrompt("Load remote swarm config from manager and keep it as baseline?", true)
	if err != nil {
		return err
	}
	if !keep {
		return nil
	}

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	loadRemoteConfig(cfg, remoteCfg)
	if err := c.reconcileSecretsWithBitwarden(cfg, remoteCfg); err != nil {
		return err
	}
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return err
	}
	fmt.Println(styles.SuccessText.Render("Remote swarm config loaded into the in-memory session config."))
	return nil
}

func (c *Container) printDeploySummaryBytes(envContent []byte, cfg *configs.D8XConfig, managerIp string) {
	fmt.Println(styles.ItalicText.Render("Deployment summary:"))
	fmt.Printf("  environment       : %s\n", c.SelectedEnv)
	fmt.Printf("  manager IP        : %s\n", managerIp)
	fmt.Printf("  chain id          : %d\n", cfg.ChainId)
	fmt.Printf("  remote broker http: %s\n", cfg.SwarmRemoteBrokerHTTPUrl)
	keys := []string{
		"CHAIN_ID",
		"SDK_CONFIG_NAME",
		"REMOTE_BROKER_HTTP",
		"DATABASE_DSN",
		"REDIS_PASSWORD",
		"WS_SPORTSLINEINDEX",
		"NODE_AUTH_TOKEN",
	}
	values := parseEnvBytes(envContent)
	fmt.Println(styles.ItalicText.Render("Values that will be written to ./trader-backend/.env on the manager:"))
	for _, k := range keys {
		v, ok := values[k]
		if !ok {
			continue
		}
		fmt.Printf("  %-20s = %s\n", k, redactSecret(k, v))
	}
}

func parseEnvBytes(data []byte) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rest, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(strings.Trim(rest, `"'`))
	}
	return out
}


func redactSecret(key, value string) string {
	upper := strings.ToUpper(key)
	if value == "" {
		return "(empty)"
	}
	if strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") {
		return redactValue(value)
	}
	if upper == "DATABASE_DSN" {
		return redactDSN(value)
	}
	return value
}

func redactValue(v string) string {
	if len(v) <= 4 {
		return "****"
	}
	return v[:2] + strings.Repeat("*", len(v)-4) + v[len(v)-2:]
}

func redactDSN(dsn string) string {
	at := strings.LastIndex(dsn, "@")
	if at < 0 {
		return redactValue(dsn)
	}
	prefix := dsn[:at]
	suffix := dsn[at:]
	colon := strings.LastIndex(prefix, ":")
	if colon < 0 {
		return prefix + suffix
	}
	return prefix[:colon] + ":****" + suffix
}

func (c *Container) reconcileSecretsWithBitwarden(cfg *configs.D8XConfig, remoteCfg *configs.D8XConfig) error {
	upperEnv := strings.ToUpper(c.SelectedEnv)
	checks := []struct {
		displayName string
		bwField     string
		remoteVal   string
		target      *string
	}{
		{"DATABASE_DSN", "DATABASE_DSN_" + upperEnv, remoteCfg.DatabaseDSN, &cfg.DatabaseDSN},
		{"REDIS_PASSWORD", "SWARM_REDIS_PW_" + upperEnv, remoteCfg.SwarmRedisPassword, &cfg.SwarmRedisPassword},
	}
	for _, chk := range checks {
		bwVal := ""
		if c.BitwardenFields != nil {
			bwVal = c.BitwardenFields[chk.bwField]
		}
		if bwVal == "" && chk.remoteVal == "" {
			continue
		}
		if bwVal != "" && chk.remoteVal == "" {
			if *chk.target != "" && *chk.target != bwVal {
				fmt.Printf("%s %s in this session's config differs from Bitwarden (%s). Using Bitwarden value.\n", notok, chk.displayName, chk.bwField)
			}
			*chk.target = bwVal
			continue
		}
		if bwVal == "" && chk.remoteVal != "" {
			fmt.Printf("%s %s is set on the manager but missing in Bitwarden (%s).\n", notok, chk.displayName, chk.bwField)
			keep, err := c.TUI.NewPrompt(fmt.Sprintf("Keep manager value for %s and save it to Bitwarden as %s?", chk.displayName, chk.bwField), true)
			if err != nil {
				return err
			}
			if !keep {
				return fmt.Errorf("aborted: missing Bitwarden secret %s", chk.bwField)
			}
			*chk.target = chk.remoteVal
			if os.Getenv("BW_SESSION") != "" {
				if err := saveAndReport(chk.bwField, chk.remoteVal); err != nil {
					cont, perr := c.TUI.NewPrompt(fmt.Sprintf("Bitwarden save of %s failed. Proceed with deployment using the manager value (leaving Bitwarden unchanged)?", chk.bwField), false)
					if perr != nil {
						return perr
					}
					if !cont {
						return fmt.Errorf("aborted: %s could not be persisted to Bitwarden", chk.bwField)
					}
				}
				if c.BitwardenFields == nil {
					c.BitwardenFields = make(map[string]string)
				}
				c.BitwardenFields[chk.bwField] = chk.remoteVal
			}
			continue
		}
		if bwVal == chk.remoteVal {
			*chk.target = bwVal
			continue
		}
		fmt.Printf("%s %s MISMATCH between manager .env and Bitwarden (%s).\n", notok, chk.displayName, chk.bwField)
		choice, err := c.TUI.NewSelection(
			[]string{
				fmt.Sprintf("Use Bitwarden value for %s", chk.displayName),
				fmt.Sprintf("Use manager value for %s (and overwrite Bitwarden %s)", chk.displayName, chk.bwField),
				"Abort deployment",
			},
			components.SelectionOptAllowOnlySingleItem(),
			components.SelectionOptRequireSelection(),
		)
		if err != nil {
			return err
		}
		if len(choice) == 0 {
			return fmt.Errorf("aborted: no choice made for %s reconciliation", chk.displayName)
		}
		switch {
		case strings.HasPrefix(choice[0], "Use Bitwarden"):
			*chk.target = bwVal
		case strings.HasPrefix(choice[0], "Use manager"):
			*chk.target = chk.remoteVal
			if os.Getenv("BW_SESSION") != "" {
				if _, _, saveErr := ForceOverwriteBitwarden(chk.bwField, chk.remoteVal); saveErr != nil {
					fmt.Printf("  %s could not overwrite %s in Bitwarden: %s\n", notok, chk.bwField, saveErr)
					cont, perr := c.TUI.NewPrompt(fmt.Sprintf("Bitwarden overwrite of %s failed. Proceed with deployment anyway (Bitwarden will remain out of sync)?", chk.bwField), false)
					if perr != nil {
						return perr
					}
					if !cont {
						return fmt.Errorf("aborted: %s could not be overwritten in Bitwarden", chk.bwField)
					}
				} else {
					fmt.Printf("  %s %s overwritten in Bitwarden\n", ok, chk.bwField)
				}
				if c.BitwardenFields == nil {
					c.BitwardenFields = make(map[string]string)
				}
				c.BitwardenFields[chk.bwField] = chk.remoteVal
			}
		default:
			return fmt.Errorf("aborted: %s reconciliation", chk.displayName)
		}
	}
	return nil
}

func (c *Container) fetchRemoteSwarmDeployConfig(managerIp string) (*configs.D8XConfig, error) {
	sshConn, err := c.CreateSSHConn(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return nil, fmt.Errorf("SSH to manager %s failed: %w", managerIp, err)
	}

	envOut, err := sshConn.ExecCommand(`if [ -f ./trader-backend/.env ]; then cat ./trader-backend/.env; fi`)
	if err != nil {
		return nil, fmt.Errorf("reading remote ./trader-backend/.env on %s: %w", managerIp, err)
	}
	remoteEnv := strings.TrimSpace(string(envOut))
	if remoteEnv == "" {
		fmt.Println(styles.ItalicText.Render("No existing ./trader-backend/.env on manager; treating as a fresh deployment."))
		return nil, nil
	}

	backupPath := workPath(fmt.Sprintf("trader-backend/.env.manager-backup-%s", time.Now().UTC().Format("20060102-150405")))
	if err := c.FS.WriteFile(backupPath, []byte(remoteEnv)); err != nil {
		fmt.Printf("%s failed to write remote .env backup to %s: %s\n", notok, backupPath, err)
		cont, perr := c.TUI.NewPrompt("Remote .env backup could not be written locally. Proceed without a safety copy?", false)
		if perr != nil {
			return nil, perr
		}
		if !cont {
			return nil, fmt.Errorf("aborted: no local backup of remote .env could be written")
		}
	} else {
		fmt.Printf("%s saved remote .env backup to %s\n", ok, backupPath)
		c.LastEnvBackupPath = backupPath
	}

	cfg := &configs.D8XConfig{}
	for _, line := range strings.Split(remoteEnv, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(strings.Trim(value, `"'`))
		switch key {
		case "CHAIN_ID":
			chainId, _ := strconv.Atoi(value)
			cfg.ChainId = uint(chainId)
		case "DATABASE_DSN":
			cfg.DatabaseDSN = value
		case "REMOTE_BROKER_HTTP":
			cfg.SwarmRemoteBrokerHTTPUrl = value
		case "REDIS_PASSWORD":
			cfg.SwarmRedisPassword = value
		}
	}

	for _, fname := range []string{"./trader-backend/rpc.main.json", "./trader-backend/rpc.history.json"} {
		rpcOut, err := sshConn.ExecCommand(fmt.Sprintf(`if [ -f %s ]; then cat %s; fi`, fname, fname))
		if err != nil {
			fmt.Printf("%s remote %s could not be read (%s); skipping\n", notok, fname, err)
			continue
		}
		if strings.TrimSpace(string(rpcOut)) == "" {
			fmt.Printf("%s remote %s is empty or missing; skipping\n", notok, fname)
			continue
		}
		if err := c.populateRemoteRpcConfig(cfg, rpcOut); err != nil {
			fmt.Printf("%s remote %s parse failed (%s); skipping\n", notok, fname, err)
			continue
		}
	}

	if cfg.ChainId == 0 && len(cfg.HttpRpcList) == 0 && len(cfg.WsRpcList) == 0 && cfg.DatabaseDSN == "" && cfg.SwarmRemoteBrokerHTTPUrl == "" && cfg.SwarmRedisPassword == "" {
		fmt.Println(styles.ItalicText.Render("Remote ./trader-backend/.env was parsed but contains no recognised fields; treating as a fresh deployment."))
		return nil, nil
	}
	return cfg, nil
}

func (c *Container) populateRemoteRpcConfig(cfg *configs.D8XConfig, content []byte) error {
	entries := []RPCConfigEntry{}
	if err := json.Unmarshal(content, &entries); err != nil {
		return err
	}
	if cfg.HttpRpcList == nil {
		cfg.HttpRpcList = make(map[string][]string)
	}
	if cfg.WsRpcList == nil {
		cfg.WsRpcList = make(map[string][]string)
	}

	for _, entry := range entries {
		chainIdStr := strconv.Itoa(int(entry.ChainId))
		if len(entry.HttpRpcs) > 0 {
			cfg.HttpRpcList[chainIdStr] = append(cfg.HttpRpcList[chainIdStr], entry.HttpRpcs...)
		}
		if entry.WsRpcs != nil {
			cfg.WsRpcList[chainIdStr] = append(cfg.WsRpcList[chainIdStr], *entry.WsRpcs...)
		}
	}

	for chainID, rpcs := range cfg.HttpRpcList {
		cfg.HttpRpcList[chainID] = slices.Compact(rpcs)
	}
	for chainID, rpcs := range cfg.WsRpcList {
		cfg.WsRpcList[chainID] = slices.Compact(rpcs)
	}

	return nil
}

func (c *Container) SwarmDeploy(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting swarm cluster deployment...")

	defer func() {
		if c.LastEnvBackupPath == "" {
			return
		}
		if err := os.Remove(c.LastEnvBackupPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("%s failed to remove .env backup %s: %s\n", notok, c.LastEnvBackupPath, err)
		}
		c.LastEnvBackupPath = ""
	}()

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if _, err := c.EnsureEnvironment(cfg); err != nil {
		return err
	}
	if cfg.ServerProvider == "" {
		return fmt.Errorf("server_provider is empty in this env's config.json on the infra repo; set it to \"linode\" or \"aws\" there, or run \"d8x setup provision\" first")
	}

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

	if err := c.importRemoteSwarmDeployConfig(ctx, managerIp); err != nil {
		fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Could not load remote swarm config: %v", err)))
		cont, perr := c.TUI.NewPrompt("Proceed without remote config? (this will deploy as if it were a fresh cluster and may overwrite live values)", false)
		if perr != nil {
			return perr
		}
		if !cont {
			return fmt.Errorf("aborted: remote config unreachable")
		}
	}

	if err := c.Input.CollectSwarmDeployInputs(ctx); err != nil {
		return err
	}

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	envContent, err := configs.EmbededConfigs.ReadFile("embedded/trader-backend/env.example")
	if err != nil {
		return fmt.Errorf("loading embedded trader-backend env template: %w", err)
	}
	rpcMain, err := c.loadInfraRepoFile("trader-backend/rpc.main.json", "embedded/trader-backend/rpc.main.json")
	if err != nil {
		return err
	}
	rpcHist, err := c.loadInfraRepoFile("trader-backend/rpc.history.json", "embedded/trader-backend/rpc.history.json")
	if err != nil {
		return err
	}
	prices, err := c.loadInfraRepoFile("candles/prices.config.json", "embedded/candles/prices.config.json")
	if err != nil {
		return err
	}
	rpcConf, err := c.loadInfraRepoFile("candles/rpc_conf.json", "embedded/candles/rpc_conf.json")
	if err != nil {
		return err
	}
	swarmStack, err := c.loadInfraRepoFile("docker-swarm-stack.yml", "embedded/docker-swarm-stack.yml")
	if err != nil {
		return err
	}

	chainIdStr := strconv.Itoa(int(cfg.ChainId))
	shouldUpdateConfigs := cfg.ChainId != 0 && (len(cfg.HttpRpcList[chainIdStr]) > 0 || len(cfg.WsRpcList[chainIdStr]) > 0 || cfg.DatabaseDSN != "" || cfg.SwarmRemoteBrokerHTTPUrl != "" || cfg.SwarmRedisPassword != "" || len(cfg.UserSuppliedPriceFeedEndpoints) > 0)

	if c.Input.swarmDeployInput.guideConfig || shouldUpdateConfigs {
		envContent, err = c.EditSwarmEnvBytes(envContent, cfg)
		if err != nil {
			return fmt.Errorf("editing .env content: %w", err)
		}

		if len(cfg.HttpRpcList[chainIdStr]) > 0 || len(cfg.WsRpcList[chainIdStr]) > 0 {
			for i, slot := range []struct {
				name    string
				content *[]byte
			}{
				{"trader-backend/rpc.main.json", &rpcMain},
				{"trader-backend/rpc.history.json", &rpcHist},
			} {
				httpRpcs, wsRpcs := DistributeRpcs(i, chainIdStr, cfg)
				fmt.Printf("Updating %s config...\n", slot.name)
				updated, uerr := c.editRpcConfigUrlsBytes(*slot.content, cfg.ChainId, wsRpcs, httpRpcs)
				if uerr != nil {
					fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Could not update %s: %+v", slot.name, uerr)))
					continue
				}
				*slot.content = updated
			}
		}

		userProvidedHttpEndpoints := cfg.UserSuppliedPriceFeedEndpoints
		slices.Sort(userProvidedHttpEndpoints)
		userProvidedHttpEndpoints = slices.Compact(userProvidedHttpEndpoints)
		defaultHttpEndpoint := c.cachedChainJson.getDefaultPythHTTPSEndpoint(chainIdStr)
		priceServiceHTTPSEndpoints := userProvidedHttpEndpoints
		if !slices.Contains(priceServiceHTTPSEndpoints, defaultHttpEndpoint) {
			priceServiceHTTPSEndpoints = append(priceServiceHTTPSEndpoints, defaultHttpEndpoint)
		}

		if len(priceServiceHTTPSEndpoints) > 0 {
			updated, uerr := UpdateConfigBytes(prices, UpdateCandlesPriceConfigPriceServices(priceServiceHTTPSEndpoints))
			if uerr != nil {
				return fmt.Errorf("updating candles prices config: %w", uerr)
			}
			prices = updated
		}
	}

	if showConfigConfirmation {
		fmt.Println(styles.AlertImportant.Render("Review the configuration below before deploying."))
		c.printDeploySummaryBytes(envContent, cfg, managerIp)
		fmt.Println("The following configuration files will be copied to the 'manager node':")
		for _, dst := range []string{
			"./trader-backend/.env",
			"./trader-backend/rpc.main.json",
			"./trader-backend/rpc.history.json",
			"./candles/prices.config.json",
			"./candles/rpc_conf.json",
			"./docker-stack.yml",
		} {
			fmt.Println("  " + dst)
		}
		proceed, err := c.TUI.NewPrompt("Proceed with deployment using the values above?", false)
		if err != nil {
			return err
		}
		if !proceed {
			return fmt.Errorf("aborted: deployment declined at confirmation step")
		}
	}

	pwd, err := c.ResolvePassword(ctx)
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
	cmd := fmt.Sprintf(
		`echo '%s' | sudo -S bash -c 'mkdir -p /var/nfs/general && chown nobody:nogroup /var/nfs/general'`,
		pwd,
	)

	configEtcExports := "#"
	for _, ip := range ipWorkersPriv {
		// Essentially ufw allow from %s to any port nfs (tcp/udp)
		iptables := fmt.Sprintf(`iptables -A INPUT -s %[1]s -p tcp --dport 2049 -j ACCEPT && iptables -A INPUT -s %[1]s -p udp --dport 2049 -j ACCEPT`, ip)
		cmdUfw := fmt.Sprintf(`&& echo '%s' | sudo -S bash -c "%s" `, pwd, iptables)
		cmd = cmd + cmdUfw
		configEtcExports = configEtcExports + "\n" + fmt.Sprintf(`/var/nfs/general %s(rw,sync,no_subtree_check)`, ip)
	}
	// Persist rules
	cmd = cmd + fmt.Sprintf(`&& echo '%s' | sudo -S bash -c "mkdir -p /etc/iptables && iptables-save > /etc/iptables/rules.v4" `, pwd)

	_, err = managerSSHConn.ExecCommand(
		cmd,
	)
	if err != nil {
		return fmt.Errorf("NFS preparation on manager failed : %w", err)
	}
	exportsContent := []byte(configEtcExports)

	managedConfigNames := []string{
		"cfg_rpc",
		"cfg_rpc_history",
		"cfg_prices",
		"cfg_rpc_candles",
	}
	// Lines of docker config commands which we will concat into single
	// bash -c ssh call
	dockerConfigsCMD := []string{
		`docker config create cfg_rpc ./trader-backend/rpc.main.json >/dev/null 2>&1`,
		`docker config create cfg_rpc_history ./trader-backend/rpc.history.json >/dev/null 2>&1`,
		`docker config create cfg_prices ./candles/prices.config.json >/dev/null 2>&1`,
		`docker config create cfg_rpc_candles ./candles/rpc_conf.json >/dev/null 2>&1`,
		// `docker config create prometheus_config ./prometheus.yml >/dev/null 2>&1`,
	}

	copyList := []conn.SftpCopySrcDest{
		{Content: envContent, Dst: "./trader-backend/.env"},
		{Content: rpcMain, Dst: "./trader-backend/rpc.main.json"},
		{Content: rpcHist, Dst: "./trader-backend/rpc.history.json"},
		{Content: exportsContent, Dst: "./trader-backend/exports"},
		{Content: prices, Dst: "./candles/prices.config.json"},
		{Content: rpcConf, Dst: "./candles/rpc_conf.json"},
		{Content: swarmStack, Dst: "./docker-stack.yml"},
	}

	// Copy files to remote
	fmt.Println(styles.ItalicText.Render("Copying configuration files to manager node " + managerIp))
	if err := managerSSHConn.CopyFilesOverSftp(
		copyList...,
	); err != nil {
		return fmt.Errorf("copying configuration files to manager: %w", err)
	} else {
		fmt.Println(styles.SuccessText.Render("configuration files copied to manager"))
	}

	// enable nfs server
	fmt.Println(styles.ItalicText.Render("Starting NFS server..."))
	cmd = fmt.Sprintf(`echo '%s' | sudo -S bash -c "cp ./trader-backend/exports /etc/exports && systemctl restart nfs-kernel-server"`, pwd)
	_, err = managerSSHConn.ExecCommand(
		cmd,
	)
	if err != nil {
		return fmt.Errorf("starting NFS server: %w", err)
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
				ip,
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
	for k, ip := range ipWorkers {
		var (
			sshConnWorker conn.SSHConnection
			err           error
		)
		if cfg.ServerProvider == configs.D8XServerProviderAWS {
			sshConnWorker, err = conn.NewSSHConnectionWithBastion(
				managerSSHConn.GetClient(),
				ipWorkersPriv[k],
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
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return err
	}
	if err := c.PublishRemoteConfig(cfg); err != nil {
		fmt.Printf("  %s failed to sync remote config: %s\n", notok, err)
	}
	return nil
}

func (c *Container) SwarmNginx(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting swarm nginx setup...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	env, err := c.EnsureEnvironment(cfg)
	if err != nil {
		return err
	}

	if err := c.RequireBitwardenField("GITHUB_TOKEN"); err != nil {
		return err
	}
	token := os.Getenv("GITHUB_TOKEN")
	if err := c.RequireBitwardenField("NGINX_API_KEY"); err != nil {
		return err
	}
	apiKey := os.Getenv("NGINX_API_KEY")

	password, err := c.ResolvePassword(ctx)
	if err != nil {
		return err
	}

	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}

	sshConn, err := c.CreateSSHConn(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return fmt.Errorf("SSH connection: %w", err)
	}

	fmt.Println(styles.ItalicText.Render("Fetching nginx configs from GitHub..."))
	deployCfg, err := fetchAndBuildNginxConfig(token, env)
	if err != nil {
		return err
	}
	deployCfg.sshConn = sshConn
	deployCfg.password = password
	deployCfg.apiKey = apiKey

	fmt.Println("Installing certbot...")
	sshExecSudo(sshConn, password, "apt-get remove -y certbot 2>/dev/null; true")
	sshExecSudo(sshConn, password, "snap install --classic certbot 2>/dev/null; true")
	sshExecSudo(sshConn, password, "ln -sf /snap/bin/certbot /usr/bin/certbot")

	if err := deployNginxFull(*deployCfg); err != nil {
		return err
	}

	setupCertbot, err := c.TUI.NewPrompt("Setup SSL certificates with certbot?", true)
	if err != nil {
		return err
	}
	if setupCertbot {
		fmt.Println("Enter email for certbot:")
		email := cfg.CertbotEmail
		if email == "" {
			email, err = c.TUI.NewInput(components.TextInputOptPlaceholder("admin@example.com"))
			if err != nil {
				return err
			}
			cfg.CertbotEmail = email
		}

		// Issue certs for all server_names in sites.conf
		hostnames := extractAllServerNames(deployCfg.sitesConfContent)
		for _, host := range hostnames {
			fmt.Printf("  Issuing cert for %s...\n", host)
			cmd := fmt.Sprintf("echo '%s' | sudo -S certbot --nginx -d %s --non-interactive --agree-tos -m %s 2>&1", password, host, email)
			out, err := sshConn.ExecCommand(cmd)
			if err != nil {
				fmt.Printf("  %s certbot failed for %s: %s\n", notok, host, strings.TrimSpace(string(out)))
			} else {
				fmt.Printf("  %s %s\n", ok, host)
			}
		}

		// Enable certbot renewal timer
		sshExecSudo(sshConn, password, "systemctl enable snap.certbot.renew.timer && systemctl start snap.certbot.renew.timer")

		cfg.SwarmCertbotDeployed = true
	}

	cfg.SwarmNginxDeployed = true
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return fmt.Errorf("could not update config: %w", err)
	}
	if err := c.PublishRemoteConfig(cfg); err != nil {
		fmt.Printf("  %s failed to sync remote config: %s\n", notok, err)
	}

	fmt.Println(styles.SuccessText.Render("Nginx deployment complete."))
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

