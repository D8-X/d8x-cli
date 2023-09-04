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
	linodeToken  string
	linodeDbId   string
	linodeRegion string
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
	outFile, err := os.OpenFile("./linode.tf", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return nil, err
	}
	// must be in this order: main.tf output.tf vars.tf
	err = copyEmbedFilesToDest(
		outFile,
		configs.TraderBackendConfigs,
		"trader-backend/tf-linode/main.tf",
		"trader-backend/tf-linode/output.tf",
		"trader-backend/tf-linode/vars.tf",
	)
	if err != nil {
		return nil, fmt.Errorf("generating linode.tf file: %w", err)
	}

	// Copy inventory.tpl

	createBrokerServer, err := components.NewPrompt("Do you want to provision a broker-server server?", true)
	if err != nil {
		return nil, err
	}

	// Generate ssh-key
	createKey := false
	_, err = os.Stat(c.SshKeyPath)
	if err != nil {
		fmt.Printf("SSH key %s was not found, creating new one...\n", c.SshKeyPath)
		createKey = true
	} else {
		ok, err := components.NewPrompt(
			fmt.Sprintf("SSH key %s was found, do you want to overwrite it with a new one?", c.SshKeyPath),
			true,
		)
		if err != nil {
			return nil, err
		}

		if ok {
			createKey = true
		}
	}

	if createKey {
		fmt.Println(
			"Executing:",
			styles.ItalicText.Render(
				fmt.Sprintf("ssh-keygen -t ed25519 -f %s", c.SshKeyPath),
			),
		)
		cmd := exec.Command("ssh-keygen", "-N", "", "-t", "ed25519", "-f", c.SshKeyPath, "-C", "")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, err
		}
	}

	// Get the public key contents
	pubkeyfile := fmt.Sprintf("%s.pub", c.SshKeyPath)
	pub, err := os.ReadFile(pubkeyfile)
	if err != nil {
		return nil, fmt.Errorf("reading public key %s: %w", pubkeyfile, err)
	}

	// Build the terraform apply command
	args := []string{
		"apply", "-auto-approve",
		"-var", fmt.Sprintf(`authorized_keys=["%s"]`, strings.TrimSpace(string(pub))),
		"-var", fmt.Sprintf(`linode_db_cluster_id=%s`, l.linodeDbId),
		"-var", fmt.Sprintf(`region=%s`, l.linodeRegion),
		"-var", fmt.Sprintf(`create_broker_server=%t`, createBrokerServer),
	}
	command := exec.Command("terraform", args...)
	// Add linode tokens
	command.Env = append(command.Env,
		fmt.Sprintf("LINODE_TOKEN=%s", l.linodeToken),
	)

	return command, nil
}

func (c *Container) linodeServerConfigurer() (ServerProviderConfigurer, error) {
	l := linodeConfigurer{}

	fmt.Println("Enter your Linode API token")
	token, err := components.NewInput(
		components.TextInputOptPlaceholder("<YOUR LINODE API TOKEN>"),
	)
	if err != nil {
		return nil, err
	}
	l.linodeToken = token

	fmt.Println("Enter your Linode database cluster ID")
	dbId, err := components.NewInput(
		components.TextInputOptPlaceholder("12345678"),
	)
	if err != nil {
		return nil, err
	}
	l.linodeDbId = dbId

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

	return l, nil
}
