package actions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
)

// from linde-cli regions ls
var linodeRegions = []components.ListItem{
	{ItemTitle: "eu-central", ItemDesc: "Frankfurt, DE"},
	{ItemTitle: "ap-west", ItemDesc: "Mumbai, IN"},
	{ItemTitle: "ca-central", ItemDesc: "Toronto, CA"},
	{ItemTitle: "ap-southeast", ItemDesc: "Sydney, AU"},
	{ItemTitle: "us-iad", ItemDesc: "Washington, DC"},
	{ItemTitle: "us-ord", ItemDesc: "Chicago, IL"},
	{ItemTitle: "fr-par", ItemDesc: "Paris, FR"},
	{ItemTitle: "nl-ams", ItemDesc: "Amsterdam, NL"},
	{ItemTitle: "se-sto", ItemDesc: "Stockholm, SE"},
	{ItemTitle: "it-mil", ItemDesc: "Milan, IT"},
	{ItemTitle: "us-central", ItemDesc: "Dallas, TX"},
	{ItemTitle: "us-west", ItemDesc: "Fremont, CA"},
	{ItemTitle: "us-southeast", ItemDesc: "Atlanta, GA"},
	{ItemTitle: "us-east", ItemDesc: "Newark, NJ"},
	{ItemTitle: "eu-west", ItemDesc: "London, UK"},
	{ItemTitle: "ap-south", ItemDesc: "Singapore, SG"},
	{ItemTitle: "ap-northeast", ItemDesc: "Tokyo, JP"},
}

var _ ServerProviderConfigurer = (*linodeConfigurer)(nil)

type linodeConfigurer struct {
	configs.D8XLinodeConfig

	// public key that will be used as authorized_keys param
	authorizedKey string
}

// BuildTerraformCMD builds terraform configuration for linode cluster creation.
// It also creates a ssh key pair via ssh-keygen which is used in cluster
// servers for default user and does some other neccessary configuration.
func (l linodeConfigurer) BuildTerraformCMD(c *Container) (*exec.Cmd, error) {
	// Copy terraform files to cwd/linode.tf
	if err := c.EmbedCopier.CopyMultiToDest(
		configs.EmbededConfigs,
		// Dest
		"./linode.tf",
		// Embed paths must be in this order: main.tf output.tf vars.tf
		"embedded/trader-backend/tf-linode/main.tf",
		"embedded/trader-backend/tf-linode/output.tf",
		"embedded/trader-backend/tf-linode/vars.tf",
	); err != nil {
		return nil, fmt.Errorf("generating lindode.tf file: %w", err)
	}

	// Build the terraform apply command
	args := l.generateArgs()
	command := exec.Command("terraform", args...)
	// for $HOME
	command.Env = os.Environ()
	// Add linode tokens
	command.Env = append(command.Env,
		fmt.Sprintf("LINODE_TOKEN=%s", l.Token),
	)

	return command, nil
}

func (l linodeConfigurer) generateArgs() []string {
	args := []string{
		"apply", "-auto-approve",
		"-var", fmt.Sprintf(`authorized_keys=["%s"]`, strings.TrimSpace(l.authorizedKey)),
		"-var", fmt.Sprintf(`region=%s`, l.Region),
		"-var", fmt.Sprintf(`server_label_prefix=%s`, l.LabelPrefix),
		"-var", fmt.Sprintf(`create_broker_server=%t`, l.CreateBrokerServer),
	}

	if l.DbId != "" {
		args = append(
			args,
			"-var", fmt.Sprintf(`linode_db_cluster_id=%s`, l.DbId),
		)
	}
	if l.BrokerServerSize != "" {
		args = append(
			args,
			"-var", fmt.Sprintf(`broker_size=%s`, l.BrokerServerSize),
		)
	}
	if l.SwarmNodeSize != "" {
		args = append(
			args,
			"-var", fmt.Sprintf(`worker_size=%s`, l.SwarmNodeSize),
		)
	}

	return args
}

// pullPgCert downloads the database cluster ca certificate. Certifi
func (l linodeConfigurer) pullPgCert(c *http.Client, outFile string) error {
	if l.DbId == "" {
		return fmt.Errorf("linode db id was not provided")
	}
	endpoint := fmt.Sprintf("https://api.linode.com/v4/databases/postgresql/instances/%s/ssl", l.DbId)
	data, err := fetchLinodeAPIRequest(c, endpoint, l.Token)
	if err != nil {
		return err
	}

	jsonData := map[string]string{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return err
	}

	base64cert, ok := jsonData["ca_certificate"]
	if !ok {
		return fmt.Errorf("ca_certificate was not found in response")
	}

	pgCrtContent, err := base64.StdEncoding.DecodeString(base64cert)
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(outFile, pgCrtContent, 0666); err != nil {
		return fmt.Errorf("could not store %s: %w", outFile, err)
	}

	return nil
}

func getRegionItemByRegionId(regionId string) components.ListItem {
	for _, item := range linodeRegions {
		if item.ItemTitle == regionId {
			return item
		}
	}
	return components.ListItem{}
}

// createLinodeServerConfigurer collects information for the linode cluster
// provisioning and creates linode ServerProviderConfigurer
func (c *Container) createLinodeServerConfigurer() (ServerProviderConfigurer, error) {
	l := linodeConfigurer{}

	// Attempt to load defaults from config
	var (
		defaultToken              = ""
		defaultClusterLabelPrefix = "d8x-cluster"
		defaultDbId               = ""
		defaultRegion             = ""
		defaultSwarmNodeSize      = "g6-dedicated-2"
		defaultBrokerSize         = "g6-dedicated-2"
	)
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return nil, err
	}
	if cfg.ServerProvider == configs.D8XServerProviderLinode {
		if cfg.LinodeConfig != nil {
			defaultToken = cfg.LinodeConfig.Token
			defaultDbId = cfg.LinodeConfig.DbId
			defaultRegion = cfg.LinodeConfig.Region
			defaultClusterLabelPrefix = cfg.LinodeConfig.LabelPrefix
		}
	}

	var defaultRegionItem components.ListItem
	if len(defaultRegion) > 0 {
		defaultRegionItem = getRegionItemByRegionId(defaultRegion)
	}

	// Token
	fmt.Println("Enter your Linode API token")
	token, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("<YOUR LINODE API TOKEN>"),
		components.TextInputOptValue(defaultToken),
		components.TextInputOptMasked(),
	)

	if err != nil {
		return nil, err
	}
	l.Token = token

	// DB
	if ok, err := c.TUI.NewPrompt("Do you want to use a Linode database?", false); err == nil && ok {
		fmt.Println("Enter your Linode database cluster ID")
		dbId, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("12345678"),
			components.TextInputOptValue(defaultDbId),
		)
		if err != nil {
			return nil, err
		}
		l.DbId = dbId
	} else {
		fmt.Println("Not using Linode database. Make sure you provision your own external Postgres instances!")
	}

	// Region
	selected, err := c.TUI.NewList(
		linodeRegions,
		"Choose the Linode cluster region",
		components.ListOptSelectedItem(defaultRegionItem),
	)
	if err != nil {
		return nil, err
	}
	l.Region = selected.ItemTitle
	fmt.Printf(
		"Selected region: %s\n\n",
		styles.ItalicText.Render(
			fmt.Sprintf("%s (%s)",
				selected.ItemDesc,
				selected.ItemTitle,
			),
		),
	)

	// Label prefix
	fmt.Println("Enter your Linode nodes label prefix")
	label, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("my-d8x-cluster"),
		components.TextInputOptValue(defaultClusterLabelPrefix),
	)
	if err != nil {
		return nil, err
	}
	l.LabelPrefix = label

	// Broker-server
	createBrokerServer, err := c.TUI.NewPrompt("Do you want to provision a broker-server server?", true)
	if err != nil {
		return nil, err
	}
	l.CreateBrokerServer = createBrokerServer
	c.CreateBrokerServer = createBrokerServer

	// Servers sizes
	fmt.Println("Swarm linode node size")
	swarmNodeSize, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("g6-dedicated-2"),
		components.TextInputOptValue(defaultSwarmNodeSize),
	)
	if err != nil {
		return nil, err
	}
	l.SwarmNodeSize = swarmNodeSize
	fmt.Println("Broker linode node size")
	if l.CreateBrokerServer {
		brokerNodeSize, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("g6-dedicated-2"),
			components.TextInputOptValue(defaultBrokerSize),
		)
		if err != nil {
			return nil, err
		}
		l.BrokerServerSize = brokerNodeSize
	}

	// SSH key check
	if err := c.ensureSSHKeyPresent(); err != nil {
		return nil, err
	}
	pub, err := c.getPublicKey()
	if err != nil {
		return nil, err
	}
	l.authorizedKey = pub

	// Write linode config to cfg
	cfg.ServerProvider = configs.D8XServerProviderLinode
	cfg.LinodeConfig = &l.D8XLinodeConfig
	if err := c.ConfigRWriter.Write(cfg); err != nil {
		return nil, err
	}

	return l, nil
}

// noLinodeDbCheck displays some information to users when external db is used.
func (i linodeConfigurer) noLinodeDbCheck(c *Container) {
	if i.DbId == "" {
		fmt.Println(
			styles.AlertImportant.Render(
				"Make sure you configure external database to allow connections from Linode cluster!",
			),
		)

		fmt.Printf(`You should configure your external database to allow connection from provisioned cluster.
Make sure to refer to %s inventory file or visit your server provider's dashboard to 
find the public ip addresses of your servers.
`, configs.DEFAULT_HOSTS_FILE)

		workers, _ := c.HostsCfg.GetWorkerIps()
		manager, _ := c.HostsCfg.GetMangerPublicIp()
		broker, _ := c.HostsCfg.GetBrokerPublicIp()
		fmt.Println("Worker servers IPs:")
		for _, ip := range workers {
			fmt.Println(ip)
		}
		fmt.Println("Manager server IP:")
		fmt.Println(manager)
		fmt.Println("Broker server IP:")
		fmt.Println(broker)

		c.TUI.NewConfirmation("Press enter to confirm...")
	}
}

func (i linodeConfigurer) PostProvisioningAction(c *Container) error {
	// Pull the cert for database
	if err := i.pullPgCert(c.HttpClient, c.PgCrtPath); err != nil {
		fmt.Println(
			styles.ErrorText.Render(
				fmt.Sprintf("pulling postgres car cert: %s", err.Error()),
			),
		)
	}

	// Show external db messages
	i.noLinodeDbCheck(c)

	return nil
}

// fetchLinodeAPIRequest sends GET request to linode api endpoint and reads the
// response
func fetchLinodeAPIRequest(c *http.Client, endpoint, linodeToken string) ([]byte, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		endpoint,
		nil,
	)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", linodeToken))
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
