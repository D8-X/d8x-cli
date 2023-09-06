package actions

import (
	"embed"
	"fmt"
	"io"
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
	if err := c.EmbedCopier.Copy(
		configs.TraderBackendConfigs,
		// Dest
		"./linode.tf",
		// Embed paths must be in this order: main.tf output.tf vars.tf
		"trader-backend/tf-linode/main.tf",
		"trader-backend/tf-linode/output.tf",
		"trader-backend/tf-linode/vars.tf",
	); err != nil {
		return nil, fmt.Errorf("generating lindode.tf file: %w", err)
	}
	// Cp inventory.tpl for hosts.cfg
	if err := c.EmbedCopier.Copy(
		configs.TraderBackendConfigs,
		"./inventory.tpl",
		"trader-backend/tf-linode/inventory.tpl",
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

// linodeServerConfigurer collects information for the linode cluster
// provisioning and creates linode ServerProviderConfigurer
func (c *Container) linodeServerConfigurer() (ServerProviderConfigurer, error) {
	l := linodeConfigurer{}

	// Token
	fmt.Println("Enter your Linode API token")
	token, err := components.NewInput(
		components.TextInputOptPlaceholder("<YOUR LINODE API TOKEN>"),
	)
	if err != nil {
		return nil, err
	}
	l.linodeToken = token

	// DB
	fmt.Println("Enter your Linode database cluster ID")
	dbId, err := components.NewInput(
		components.TextInputOptPlaceholder("12345678"),
	)
	if err != nil {
		return nil, err
	}
	l.linodeDbId = dbId

	// Region
	selected, err := components.NewList(linodeRegions, "Choose the Linode cluster region")
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
	label, err := components.NewInput(
		components.TextInputOptPlaceholder("my-d8x-cluster"),
		components.TextInputOptValue("d8x-cluster"),
	)
	if err != nil {
		return nil, err
	}
	l.linodeNodesLabelPrefix = label

	// Broker-server
	createBrokerServer, err := components.NewPrompt("Do you want to provision a broker-server server?", true)
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
