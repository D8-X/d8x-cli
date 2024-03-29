package actions

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/files"
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

func (c *Container) CopyLinodeTFFiles() error {
	return c.EmbedCopier.Copy(configs.EmbededConfigs,
		files.EmbedCopierOp{
			Src:       "embedded/trader-backend/tf-linode",
			Dst:       c.ProvisioningTfDir,
			Dir:       true,
			Overwrite: true,
		},
	)

}

// BuildTerraformCMD builds terraform configuration for linode cluster creation.
// It also creates a ssh key pair via ssh-keygen which is used in cluster
// servers for default user and does some other neccessary configuration.
func (l linodeConfigurer) BuildTerraformCMD(c *Container) (*exec.Cmd, error) {
	// Copy tf configs
	if err := c.CopyLinodeTFFiles(); err != nil {
		return nil, fmt.Errorf("generating lindode.tf file: %w", err)
	}

	// Build the terraform apply command
	args := l.generateArgs()
	command := exec.Command("terraform", args...)
	// for $HOME
	command.Env = os.Environ()
	command.Dir = c.ProvisioningTfDir
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
		"-var", fmt.Sprintf(`create_swarm=%t`, l.DeploySwarm),
		"-var", fmt.Sprintf(`num_workers=%d`, l.NumWorker),
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

func getRegionItemByRegionId(regionId string) components.ListItem {
	for _, item := range linodeRegions {
		if item.ItemTitle == regionId {
			return item
		}
	}
	return components.ListItem{}
}

// CollectLinodeProviderDetails collects linode provider details from user
// input, creates a new linodeConfigurer and fills in configuration details to
// cfg.
func (c *InputCollector) CollectLinodeProviderDetails(cfg *configs.D8XConfig) (linodeConfigurer, error) {
	if c.provisioning.collectedLinodeConfigurer != nil {
		return *c.provisioning.collectedLinodeConfigurer, nil
	}

	l := linodeConfigurer{}

	// Attempt to load defaults from config
	var (
		defaultToken              = ""
		defaultClusterLabelPrefix = "d8x-cluster"
		defaultDbId               = ""
		defaultRegion             = ""
		defaultSwarmNodeSize      = "g6-dedicated-2"
		defaultBrokerSize         = "g6-dedicated-2"
		defaultNumberOfWokers     = "4"
	)

	if cfg.ServerProvider == configs.D8XServerProviderLinode {
		if cfg.LinodeConfig != nil {
			defaultToken = cfg.LinodeConfig.Token
			defaultDbId = cfg.LinodeConfig.DbId
			defaultRegion = cfg.LinodeConfig.Region
			defaultClusterLabelPrefix = cfg.LinodeConfig.LabelPrefix
			defaultNumberOfWokers = strconv.Itoa(cfg.LinodeConfig.NumWorker)
			if cfg.LinodeConfig.NumWorker <= 0 {
				defaultNumberOfWokers = "4"
			}
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
		return l, err
	}
	l.Token = token

	// DB for swarm
	if c.setup.deploySwarm {
		if ok, err := c.TUI.NewPrompt("Do you want to use an external database? (choose no if you have a legacy Linode DB cluster)", true); err == nil && !ok {
			fmt.Println("Enter your Linode database cluster ID")
			dbId, err := c.TUI.NewInput(
				components.TextInputOptPlaceholder("12345678"),
				components.TextInputOptValue(defaultDbId),
			)
			if err != nil {
				return l, err
			}
			l.DbId = dbId
		} else {
			fmt.Println("Not using Linode database. Make sure you provision your own external Postgres instances!")
		}
	}

	// Region
	selected, err := c.TUI.NewList(
		linodeRegions,
		"Choose the Linode cluster region",
		components.ListOptSelectedItem(defaultRegionItem),
	)
	if err != nil {
		return l, err
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
		return l, err
	}
	l.LabelPrefix = label

	// Broker-server
	l.CreateBrokerServer = c.setup.deployBroker
	if c.setup.deployBroker {
		fmt.Println("Broker linode node size")
		brokerNodeSize, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("g6-dedicated-2"),
			components.TextInputOptValue(defaultBrokerSize),
		)
		if err != nil {
			return l, err
		}
		l.BrokerServerSize = brokerNodeSize
	}

	// Swarm details
	if c.setup.deploySwarm {
		l.DeploySwarm = true

		// Servers sizes
		fmt.Println("Swarm linode node size")
		swarmNodeSize, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("g6-dedicated-2"),
			components.TextInputOptValue(defaultSwarmNodeSize),
		)
		if err != nil {
			return l, err
		}
		l.SwarmNodeSize = swarmNodeSize

		// Number of workers
		numWorkers, err := c.CollectNumberOfWorkers(defaultNumberOfWokers)
		if err != nil {
			return l, fmt.Errorf("incorrect number of workers: %w", err)
		}
		l.NumWorker = numWorkers

	}

	c.provisioning.collectedLinodeConfigurer = &l

	// Update the cfg
	cfg.ServerProvider = configs.D8XServerProviderLinode
	cfg.LinodeConfig = &l.D8XLinodeConfig

	return l, nil
}

// noLinodeDbCheck displays some information to users when external db is used.
func (i linodeConfigurer) noLinodeDbCheck(c *Container) {
	if i.DbId == "" && !c.Input.BrokerOnly() {
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
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Show external db messages
	i.noLinodeDbCheck(c)

	// Whenever this is not the first time provisioning (after at least 1
	// configuration is done) Update linode inventory hosts.cfg file to use
	// cluster user name for ssh login before configuration is started, since
	// terraform always overwrites the hosts.cfg
	if cfg.ConfigDetails.Done && len(cfg.ConfigDetails.ConfiguredServers) > 0 {
		if err := c.LinodeInventorySetUserVar(cfg.ConfigDetails.ConfiguredServers, c.DefaultClusterUserName); err != nil {
			return fmt.Errorf("updating linode inventory file: %w", err)
		}
	}

	return nil
}

// Set ansible_user variable to linode hosts.cfg inventory file for all servers
// which public ip address is within the provided ipAddresses list
func (c *Container) LinodeInventorySetUserVar(ipAddresses []string, sshUser string) error {
	hostLines, err := c.HostsCfg.GetLines()
	if err != nil {
		return fmt.Errorf("retrieving hosts contents: %w", err)
	}

	ipv4Pattern := regexp.MustCompile(`^(25[0-5]|2[0-4][0-9]|[0-1]?[0-9]?[0-9])\.(25[0-5]|2[0-4][0-9]|[0-1]?[0-9]?[0-9])\.(25[0-5]|2[0-4][0-9]|[0-1]?[0-9]?[0-9])\.(25[0-5]|2[0-4][0-9]|[0-1]?[0-9]?[0-9])`)

	// Update existing server lines and append ansible_user to them
	for i, line := range hostLines {
		if ipv4Pattern.MatchString(line) && !strings.Contains(line, "ansible_user=") {
			publicIp := ipv4Pattern.FindString(line)
			if slices.Contains(ipAddresses, publicIp) {
				hostLines[i] = fmt.Sprintf("%s ansible_user=%s", line, sshUser)
			}
		}
	}

	return c.HostsCfg.WriteLines(hostLines)
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
