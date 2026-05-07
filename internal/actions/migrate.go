package actions

import (
	"fmt"
	"os"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
)

func (c *Container) MigrateLegacyConfig(legacyPath string) error {
	st, err := os.Stat(legacyPath)
	if err != nil || st.IsDir() {
		return nil
	}

	legacy := configs.NewFileBasedD8XConfigRW(legacyPath)
	cfg, err := legacy.Read()
	if err != nil {
		fmt.Printf("%s could not read legacy %s (%s); leaving in place\n", notok, legacyPath, err)
		return nil
	}
	if cfg.IsEmpty() && !legacyHasAnyData(cfg) {
		fmt.Printf("%s legacy %s is empty; deleting\n", ok, legacyPath)
		return os.Remove(legacyPath)
	}

	fmt.Println(styles.AlertImportant.Render("Legacy d8x.conf.json detected"))
	fmt.Printf("File: %s\n\n", legacyPath)
	printLegacySummary(cfg)

	choice, err := c.TUI.NewSelection(
		[]string{
			"Walk through each field and migrate to Bitwarden / infra repo",
			"Keep the file as-is (skip migration for this run)",
			"Delete the file without migrating (its data has already been moved)",
		},
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)
	if err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(choice[0], "Walk"):
		return c.runMigrationWalkthrough(cfg, legacyPath)
	case strings.HasPrefix(choice[0], "Delete"):
		if err := os.Remove(legacyPath); err != nil {
			return fmt.Errorf("removing %s: %w", legacyPath, err)
		}
		fmt.Printf("%s removed %s\n", ok, legacyPath)
		return nil
	default:
		fmt.Println(styles.ItalicText.Render("Migration skipped. Run any d8x command later to revisit."))
		return nil
	}
}

func legacyHasAnyData(cfg *configs.D8XConfig) bool {
	if cfg == nil {
		return false
	}
	if cfg.ServerProvider != "" || cfg.ChainId != 0 || cfg.SetupDomain != "" || cfg.CertbotEmail != "" {
		return true
	}
	if cfg.SwarmRedisPassword != "" || cfg.DatabaseDSN != "" || cfg.SwarmRemoteBrokerHTTPUrl != "" {
		return true
	}
	if cfg.BrokerServerConfig.RedisPassword != "" || cfg.BrokerServerConfig.FeeTBPS != "" {
		return true
	}
	if cfg.LinodeConfig != nil && (cfg.LinodeConfig.Token != "" || cfg.LinodeConfig.LabelPrefix != "") {
		return true
	}
	if cfg.AWSConfig != nil && (cfg.AWSConfig.AccesKey != "" || cfg.AWSConfig.SecretKey != "" || cfg.AWSConfig.LabelPrefix != "") {
		return true
	}
	if len(cfg.HttpRpcList) > 0 || len(cfg.WsRpcList) > 0 {
		return true
	}
	if len(cfg.Services) > 0 {
		return true
	}
	if len(cfg.UserSuppliedPriceFeedEndpoints) > 0 {
		return true
	}
	return false
}

func printLegacySummary(cfg *configs.D8XConfig) {
	fmt.Println(styles.ItalicText.Render("Contents found:"))
	if cfg.ServerProvider != "" {
		fmt.Printf("  provider             : %s\n", cfg.ServerProvider)
	}
	if cfg.ChainId != 0 {
		fmt.Printf("  chain_id             : %d\n", cfg.ChainId)
	}
	if cfg.SetupDomain != "" {
		fmt.Printf("  setup_domain         : %s\n", cfg.SetupDomain)
	}
	if cfg.CertbotEmail != "" {
		fmt.Printf("  certbot_email        : %s\n", cfg.CertbotEmail)
	}
	if cfg.SwarmRemoteBrokerHTTPUrl != "" {
		fmt.Printf("  remote_broker_http   : %s\n", cfg.SwarmRemoteBrokerHTTPUrl)
	}
	if cfg.LinodeConfig != nil {
		fmt.Printf("  linode label_prefix  : %s\n", cfg.LinodeConfig.LabelPrefix)
		fmt.Printf("  linode region        : %s\n", cfg.LinodeConfig.Region)
		fmt.Printf("  linode token         : %s\n", maskedSecret(cfg.LinodeConfig.Token))
	}
	if cfg.AWSConfig != nil {
		fmt.Printf("  aws label_prefix     : %s\n", cfg.AWSConfig.LabelPrefix)
		fmt.Printf("  aws region           : %s\n", cfg.AWSConfig.Region)
		fmt.Printf("  aws access_key       : %s\n", maskedSecret(cfg.AWSConfig.AccesKey))
		fmt.Printf("  aws secret_key       : %s\n", maskedSecret(cfg.AWSConfig.SecretKey))
	}
	if cfg.SwarmRedisPassword != "" {
		fmt.Printf("  swarm_redis_password : %s\n", maskedSecret(cfg.SwarmRedisPassword))
	}
	if cfg.DatabaseDSN != "" {
		fmt.Printf("  database_dsn         : %s\n", maskedSecret(cfg.DatabaseDSN))
	}
	if cfg.BrokerServerConfig.RedisPassword != "" {
		fmt.Printf("  broker_redis_password: %s\n", maskedSecret(cfg.BrokerServerConfig.RedisPassword))
	}
	if cfg.BrokerServerConfig.FeeTBPS != "" {
		fmt.Printf("  broker_fee_tbps      : %s\n", cfg.BrokerServerConfig.FeeTBPS)
	}
	for chainId, urls := range cfg.HttpRpcList {
		fmt.Printf("  http_rpcs[%s]    : %d entries\n", chainId, len(urls))
	}
	for chainId, urls := range cfg.WsRpcList {
		fmt.Printf("  ws_rpcs[%s]      : %d entries\n", chainId, len(urls))
	}
	fmt.Println()
}

func maskedSecret(v string) string {
	if v == "" {
		return "(empty)"
	}
	if len(v) <= 8 {
		return "***"
	}
	return v[:4] + "..." + v[len(v)-4:]
}

func (c *Container) runMigrationWalkthrough(cfg *configs.D8XConfig, legacyPath string) error {
	fmt.Println(styles.ItalicText.Render("\nMigrating field-by-field. Each prompt is independent — skip any you don't want to migrate."))
	fmt.Println(styles.ItalicText.Render("Secrets go to Bitwarden (env-suffixed). Metadata is pushed to the infra repo on your next deploy."))

	envName, err := c.askMigrationEnv()
	if err != nil {
		return err
	}
	if envName == "" {
		fmt.Println(styles.ItalicText.Render("No env selected for migration; secrets cannot be moved. Aborting walkthrough."))
		return nil
	}
	envUpper := strings.ToUpper(envName)

	type secretField struct {
		label    string
		bwField  string
		value    string
		personal bool
	}
	secrets := []secretField{}
	if cfg.LinodeConfig != nil && cfg.LinodeConfig.Token != "" {
		secrets = append(secrets, secretField{"Linode API token", "LINODE_TOKEN_" + envUpper, cfg.LinodeConfig.Token, false})
	}
	if cfg.AWSConfig != nil {
		if cfg.AWSConfig.AccesKey != "" {
			secrets = append(secrets, secretField{"AWS access key", "AWS_ACCESS_KEY_" + envUpper, cfg.AWSConfig.AccesKey, true})
		}
		if cfg.AWSConfig.SecretKey != "" {
			secrets = append(secrets, secretField{"AWS secret key", "AWS_SECRET_KEY_" + envUpper, cfg.AWSConfig.SecretKey, true})
		}
	}
	if cfg.SwarmRedisPassword != "" {
		secrets = append(secrets, secretField{"swarm Redis password", "SWARM_REDIS_PW_" + envUpper, cfg.SwarmRedisPassword, false})
	}
	if cfg.DatabaseDSN != "" {
		secrets = append(secrets, secretField{"database DSN", "DATABASE_DSN_" + envUpper, cfg.DatabaseDSN, false})
	}
	if cfg.BrokerServerConfig.RedisPassword != "" {
		secrets = append(secrets, secretField{"broker Redis password", "BROKER_REDIS_PW_" + envUpper, cfg.BrokerServerConfig.RedisPassword, false})
	}

	for _, s := range secrets {
		fmt.Printf("\n%s %s\n", arrow, s.label)
		fmt.Printf("  current value (masked): %s\n", maskedSecret(s.value))
		fmt.Printf("  destination Bitwarden field: %s (%s vault)\n", s.bwField, vaultLabel(s.personal))
		action, err := c.TUI.NewSelection(
			[]string{
				"Save to Bitwarden",
				"Edit value before saving",
				"Skip this field",
			},
			components.SelectionOptAllowOnlySingleItem(),
			components.SelectionOptRequireSelection(),
		)
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(action[0], "Save"):
			if err := saveMigrationSecret(s.bwField, s.value, s.personal); err != nil {
				fmt.Printf("  %s save failed: %s\n", notok, err)
			} else {
				fmt.Printf("  %s saved %s to Bitwarden\n", ok, s.bwField)
			}
		case strings.HasPrefix(action[0], "Edit"):
			edited, err := c.TUI.NewInput(
				components.TextInputOptValue(s.value),
				components.TextInputOptMasked(),
			)
			if err != nil {
				return err
			}
			if err := saveMigrationSecret(s.bwField, edited, s.personal); err != nil {
				fmt.Printf("  %s save failed: %s\n", notok, err)
			} else {
				fmt.Printf("  %s saved %s to Bitwarden\n", ok, s.bwField)
			}
		default:
			fmt.Printf("  %s skipped\n", notok)
		}
	}

	fmt.Println()
	fmt.Println(styles.ItalicText.Render("Metadata fields (chain_id, label_prefix, certbot_email, deployment flags, etc.) will roundtrip via the infra repo automatically — they're written when you next run a deploy command. Nothing to do for them now."))

	doDelete, err := c.TUI.NewPrompt("\nDelete the legacy d8x.conf.json file now?", true)
	if err != nil {
		return err
	}
	if doDelete {
		if err := os.Remove(legacyPath); err != nil {
			return fmt.Errorf("removing %s: %w", legacyPath, err)
		}
		fmt.Printf("%s removed %s\n", ok, legacyPath)
	} else {
		fmt.Println(styles.ItalicText.Render("Legacy file kept. The CLI no longer writes to it; it's safe to delete manually whenever."))
	}
	return nil
}

func vaultLabel(personal bool) string {
	if personal {
		return "personal"
	}
	return "shared"
}

func saveMigrationSecret(field, value string, personal bool) error {
	if os.Getenv("BW_SESSION") == "" {
		return fmt.Errorf("BW_SESSION not set; cannot save %s", field)
	}
	if personal {
		return saveAndReportPersonal(field, value)
	}
	return saveAndReport(field, value)
}

func (c *Container) askMigrationEnv() (string, error) {
	fmt.Println(styles.ItalicText.Render("Which environment should the secrets in this legacy config be associated with?"))
	token := os.Getenv("GITHUB_TOKEN")
	options := []string{}
	if token != "" {
		if dirs, err := ghListDirs(token); err == nil {
			options = append(options, dirs...)
		}
	}
	options = append(options, "Other (type a name)", "Skip secrets migration")
	sel, err := c.TUI.NewSelection(options,
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(sel[0], "Skip") {
		return "", nil
	}
	if strings.HasPrefix(sel[0], "Other") {
		name, err := c.TUI.NewInput(components.TextInputOptPlaceholder("env name (e.g. arbitrum)"))
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(name), nil
	}
	return sel[0], nil
}
