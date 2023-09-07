package actions

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

func (c *Container) SwarmDeploy(ctx *cli.Context) error {
	// Copy embed files before starting
	if err := c.swarmCopyEmbedConfigs(); err != nil {
		return err
	}
	fmt.Println(styles.AlertImportant.Render("Please make sure you edit your .env and configuration files!"))
	fmt.Println("The following configuration files will be copied to manager node for d8x-trader-backend swarm deploymend:")
	fmt.Println("./trader-backend/.env")
	fmt.Println("./trader-backend/live.referralSettings.json")
	fmt.Println("./trader-backend/live.rpc.json")
	fmt.Println("./trader-backend/live.wsConfig.json")
	components.NewConfirmation("Confirm that your configs and .env are updated according your needs...")

	hosts, err := c.LoadHostsFile("./hosts.cfg")
	if err != nil {
		return err
	}

	managerIp, err := hosts.GetMangerPublicIp()
	if err != nil {
		return fmt.Errorf("finding manager ip address: %w", err)
	}

	pwd, err := c.getPassword(ctx)
	if err != nil {
		return err
	}

	client, err := conn.NewSSHClient(
		managerIp,
		c.DefaultClusterUserName,
		c.SshKeyPath,
	)
	if err != nil {
		return err
	}

	// Lines of docker config commands which we will concat into single
	// bash -c ssh call
	dockerConfigsCMD := []string{
		"docker config rm cfg_rpc cfg_referral cfg_wscfg pg_ca",
		"docker config create cfg_rpc ./trader-backend/live.rpc.json >/dev/null 2>&1",
		"docker config create cfg_referral ./trader-backend/live.referralSettings.json >/dev/null 2>&1",
		"docker config create cfg_wscfg ./trader-backend/live.wsConfig.json >/dev/null 2>&1",
	}

	// List of files to transfer to manager
	copyList := []conn.SftpCopySrcDest{
		{Src: "./trader-backend/.env", Dst: "./trader-backend/.env"},
		{Src: "./trader-backend/live.referralSettings.json", Dst: "./trader-backend/live.referralSettings.json"},
		{Src: "./trader-backend/live.rpc.json", Dst: "./trader-backend/live.rpc.json"},
		{Src: "./trader-backend/live.wsConfig.json", Dst: "./trader-backend/live.wsConfig.json"},
		{Src: "./trader-backend/docker-stack.yml", Dst: "./trader-backend/docker-stack.yml"},
	}

	// Include pg.cert
	if _, err := os.Stat(c.PgCrtPath); err == nil {
		dockerConfigsCMD = append(
			dockerConfigsCMD,
			"docker config create pg_ca ./deployment/pg.crt >/dev/null 2>&1",
		)
		copyList = append(copyList,
			conn.SftpCopySrcDest{Src: c.PgCrtPath, Dst: "./trader-backend/pg.crt"},
		)
	} else {
		return fmt.Errorf(c.PgCrtPath + " was not found!")
	}

	// Copy files to remote
	fmt.Println(styles.ItalicText.Render("Copying configuration files to manager node " + managerIp))
	if err := conn.CopyFilesOverSftp(
		client,
		copyList...,
	); err != nil {
		return fmt.Errorf("copying configuration files to manager: %w", err)
	} else {
		fmt.Println(styles.SuccessText.Render("configuration files copied to manager"))
	}

	// Create configs
	out, err := conn.SSHExecCommand(client,
		fmt.Sprintf(`echo '%s' | sudo -S bash -c "%s"`, pwd, strings.Join(dockerConfigsCMD, ";")),
	)
	fmt.Println(string(out))
	if err != nil {
		return err
	}
	fmt.Println(styles.SuccessText.Render("docker configs were created on manager node!"))

	// Deploy swarm stack
	fmt.Println(styles.ItalicText.Render("Deploying docker swarm via manager node..."))
	swarmDeployCMD := fmt.Sprintf(
		`cd ./trader-backend && echo '%s' | sudo -S bash -c ". .env && docker compose -f ./docker-stack.yml config | sed -E 's/published: \"([0-9]+)\"/published: \1/g' | sed -E 's/^name: .*$/ /'|  docker stack deploy -c - stack"`,
		pwd,
	)
	out, err = conn.SSHExecCommand(client, swarmDeployCMD)
	fmt.Println(string(out))
	if err != nil {
		return err
	}
	fmt.Println(styles.SuccessText.Render("D8X-trader-backend swarm was deployed"))

	return nil
}

// swarmCopyEmbedConfigs copies all configs required for trader-backend setup
// into ./trader-backend/ directory. Any existing configs are not overwritten.
func (c *Container) swarmCopyEmbedConfigs() error {

	dir := "./trader-backend/"
	// local destination - embed.FS source for copying
	localDestEmbedSrc := [][]string{
		{dir + ".env", "trader-backend/.env.example"},
		{dir + "live.referralSettings.json", "trader-backend/live.referralSettings.json"},
		{dir + "live.rpc.json", "trader-backend/live.rpc.json"},
		{dir + "live.wsConfig.json", "trader-backend/live.wsConfig.json"},
		{dir + "docker-stack.yml", "trader-backend/docker-stack.yml"},
	}

	for _, tuple := range localDestEmbedSrc {
		// Only copy if file does not exist
		if _, err := os.Stat(tuple[0]); err != nil {
			if err := c.EmbedCopier.Copy(
				configs.TraderBackendConfigs,
				tuple[0],
				tuple[1],
			); err != nil {
				return err
			}

			fmt.Printf("File created: %s\n", tuple[0])
		}
	}
	return nil
}

func (c *Container) SwarmNginx(ctx *cli.Context) error {
	// Copy required configs
	if err := c.EmbedCopier.Copy(
		configs.NginxConfigs,
		"./trader-backend/nginx-swarm.tpl.conf",
		"nginx/nginx.conf",
	); err != nil {
		return err
	}
	if err := c.EmbedCopier.Copy(
		configs.AnsiblePlaybooks,
		"./playbooks/nginx.ansible.yml",
		"playbooks/nginx.ansible.yml",
	); err != nil {
		return err
	}

	password, err := c.getPassword(ctx)
	if err != nil {
		return err
	}

	cfg, err := c.LoadHostsFile("./hosts.cfg")
	if err != nil {
		return err
	}
	managerIp, err := cfg.GetMangerPublicIp()
	if err != nil {
		return err
	}

	setupCertbot, err := components.NewPrompt("Do you want to setup SSL with certbot for manager server?", true)
	if err != nil {
		return err
	}
	emailForCertbot := ""
	if setupCertbot {
		fmt.Println("Enter your email address for certbot notifications: ")
		email, err := components.NewInput(
			components.TextInputOptPlaceholder("email@domain.com"),
		)
		if err != nil {
			return err
		}
		emailForCertbot = email
	}

	fmt.Println(
		styles.AlertImportant.Render(
			fmt.Sprintf("Make sure you correctly setup DNS A records with your manager IP address (%s)", managerIp),
		),
	)
	components.NewConfirmation("Confirm that you have setup your DNS records to point to your manager's public IP address")

	hostnames, err := c.swarmNginxCollectData()
	if err != nil {
		return err
	}

	// Run ansible-playbook for nginx setup on broker server
	args := []string{
		"--extra-vars", fmt.Sprintf(`ansible_ssh_private_key_file='%s'`, c.SshKeyPath),
		"--extra-vars", "ansible_host_key_checking=false",
		"--extra-vars", fmt.Sprintf(`ansible_become_pass='%s'`, password),
		"-i", "./hosts.cfg",
		"-u", c.DefaultClusterUserName,
		"./playbooks/nginx.ansible.yaml",
	}
	cmd := exec.Command("ansible-playbook", args...)
	connectCMDToCurrentTerm(cmd)
	if err := cmd.Run(); err != nil {
		return err
	} else {
		fmt.Println(styles.SuccessText.Render("Broker server nginx setup done!"))
	}

	if setupCertbot {
		fmt.Println(styles.ItalicText.Render("Setting up ssl certificates with certbot..."))
		sshClient, err := conn.NewSSHClient(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
		if err != nil {
			return err
		}

		out, err := c.certbotNginxSetup(
			sshClient,
			password,
			emailForCertbot,
			hostnames,
		)

		fmt.Println(string(out))
		if err != nil {
			return err
		} else {
			fmt.Println(styles.SuccessText.Render("Manager server certificates setup done!"))
		}
	}

	return nil
}

// swarmNginxCollectData collects hostnames information and prepares
// nginx.configured.conf file. Returns list of hostnames provided by user
func (c *Container) swarmNginxCollectData() ([]string, error) {

	// hostnames tuple for brevity (collecting data, prompts, replacements for
	// nginx.conf)
	type hostnameTuple struct {
		// server value is entered by user
		server      string
		prompt      string
		placeholder string
		// string pattern which will be replaced by server value
		find string
	}

	hostsTpl := []hostnameTuple{
		{
			prompt:      "Enter Main HTTP (sub)domain (e.g. main.d8x.xyz): ",
			placeholder: "main.d8x.xyz",
			find:        "%main%",
		},
		{
			prompt:      "Enter Main Websockets (sub)domain (e.g. ws.d8x.xyz): ",
			placeholder: "ws.d8x.xyz",
			find:        "%main_ws%",
		},
		{
			prompt:      "Enter History HTTP (sub)domain (e.g. history.d8x.xyz): ",
			placeholder: "history.d8x.xyz",
			find:        "%history%",
		},
		{
			prompt:      "Enter Referral HTTP (sub)domain (e.g. referral.d8x.xyz): ",
			placeholder: "referral.d8x.xyz",
			find:        "%referral%",
		},
		{
			prompt:      "Enter PXWS HTTP (sub)domain (e.g. pxws-rest.d8x.xyz): ",
			placeholder: "rest.d8x.xyz",
			find:        "%pxws%",
		},
		{
			prompt:      "Enter PXWS Websockets (sub)domain (e.g. pxws-ws.d8x.xyz): ",
			placeholder: "ws.d8x.xyz",
			find:        "%pxws_ws%",
		},
	}

	hosts := make([]string, len(hostsTpl))
	replacements := make([]files.ReplacementTuple, len(hostsTpl))
	for i, h := range hostsTpl {
		input, err := components.NewInput(
			components.TextInputOptPlaceholder(h.placeholder),
		)
		if err != nil {
			return nil, err
		}
		hostsTpl[i].server = input
		replacements[i] = files.ReplacementTuple{
			Find:    h.find,
			Replace: input,
		}

		hosts[i] = input
	}

	// Confirm choices
	for _, h := range hostsTpl {
		fmt.Printf(
			"(sub)domain for %s: %s\n",
			strings.ReplaceAll(h.find, "%", ""),
			h.server,
		)
	}
	correct, err := components.NewPrompt("Are these value correct?", true)
	if err != nil {
		return nil, err
	}
	// Restart this func
	if !correct {
		return c.swarmNginxCollectData()
	}

	fmt.Println(styles.ItalicText.Render("Generating nginx.conf for swarm manager..."))
	// Replace server_name's in nginx.conf
	if err := c.FS.ReplaceAndCopy(
		"./nginx.configured.conf",
		"./trader-backend/nginx-swarm.tpl.conf",
		replacements,
	); err != nil {
		return nil, err
	}

	return hosts, nil
}
