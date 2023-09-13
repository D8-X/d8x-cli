package actions

import (
	"embed"
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
	linodeToken            string
	linodeDbId             string
	linodeRegion           string
	linodeNodesLabelPrefix string

	// public key that will be used as authorized_keys param
	authorizedKey string

	createBroker bool
}

// copyEmbedFilesToDest copies embedFiles from embedFS into dest in the order that embedFS are provided
func copyEmbedFilesToDest(dest *os.File, embedFS embed.FS, embedFiles ...string) error {
	for _, embedFile := range embedFiles {
		f, err := embedFS.Open(embedFile)
		if err != nil {
			return err
		}

		_, err = io.Copy(dest, f)
		if err != nil {
			return err
		}
	}

	return nil
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
	// Cp inventory.tpl for hosts.cfg
	if err := c.EmbedCopier.CopyMultiToDest(
		configs.EmbededConfigs,
		"./inventory.tpl",
		"embedded/trader-backend/tf-linode/inventory.tpl",
	); err != nil {
		return nil, fmt.Errorf("generating inventory.tpl file: %w", err)
	}

	// Build the terraform apply command
	args := l.generateArgs()
	command := exec.Command("terraform", args...)
	// for $HOME
	command.Env = os.Environ()
	// Add linode tokens
	command.Env = append(command.Env,
		fmt.Sprintf("LINODE_TOKEN=%s", l.linodeToken),
	)

	return command, nil
}

func (l linodeConfigurer) generateArgs() []string {
	return []string{
		"apply", "-auto-approve",
		"-var", fmt.Sprintf(`authorized_keys=["%s"]`, strings.TrimSpace(l.authorizedKey)),
		"-var", fmt.Sprintf(`linode_db_cluster_id=%s`, l.linodeDbId),
		"-var", fmt.Sprintf(`region=%s`, l.linodeRegion),
		"-var", fmt.Sprintf(`server_label_prefix=%s`, l.linodeNodesLabelPrefix),
		"-var", fmt.Sprintf(`create_broker_server=%t`, l.createBroker),
	}
}

// pullPgCert downloads the database cluster ca certificate. Certifi
func (l linodeConfigurer) pullPgCert(c *http.Client, outFile string) error {
	if l.linodeDbId == "" {
		return fmt.Errorf("linodeDbId was not provided")
	}
	req, err := http.NewRequest(
		http.MethodGet,
		fmt.Sprintf("https://api.linode.com/v4/databases/postgresql/instances/%s/ssl", l.linodeDbId),
		nil,
	)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", l.linodeToken))
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
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

// linodeServerConfigurer collects information for the linode cluster
// provisioning and creates linode ServerProviderConfigurer
func (c *Container) linodeServerConfigurer() (ServerProviderConfigurer, error) {
	l := linodeConfigurer{}

	// Attempt to load defaults from config
	var (
		defaultToken              = ""
		defaultClusterLabelPrefix = "d8x-cluster"
		defaultDbId               = ""
		defaultRegion             = ""
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
	)
	if err != nil {
		return nil, err
	}
	l.linodeToken = token

	// DB
	fmt.Println("Enter your Linode database cluster ID")
	dbId, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("12345678"),
		components.TextInputOptValue(defaultDbId),
	)
	if err != nil {
		return nil, err
	}
	l.linodeDbId = dbId

	// Region
	selected, err := c.TUI.NewList(
		linodeRegions,
		"Choose the Linode cluster region",
		components.ListOptSelectedItem(defaultRegionItem),
	)
	if err != nil {
		return nil, err
	}
	l.linodeRegion = selected.ItemTitle
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
	l.linodeNodesLabelPrefix = label

	// Broker-server
	createBrokerServer, err := c.TUI.NewPrompt("Do you want to provision a broker-server server?", true)
	if err != nil {
		return nil, err
	}
	l.createBroker = createBrokerServer
	c.CreateBrokerServer = createBrokerServer

	// SSH key check
	if err := c.ensureSSHKeyPresent(); err != nil {
		return nil, err
	}
	pub, err := c.getPublicKey()
	if err != nil {
		return nil, err
	}
	l.authorizedKey = pub

	return l, nil
}
