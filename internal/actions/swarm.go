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

// Stack name that will be used when creating/destroying or managing swarm
// cluster deployment.
// TODO - store this in config and make this configurable via flags
var dockerStackName = "stack"

func (c *Container) SwarmDeploy(ctx *cli.Context) error {
	styles.PrintCommandTitle("Starting swarm cluster deployment...")

	// Copy embed files before starting
	filesToCopy := []files.EmbedCopierOp{
		// Trader backend configs
		// Note that .env.example is not recognized in embed.FS
		{Src: "embedded/trader-backend/env.example", Dst: "./trader-backend/.env", Overwrite: false},
		{Src: "embedded/trader-backend/live.referralSettings.json", Dst: "./trader-backend/live.referralSettings.json", Overwrite: false},
		{Src: "embedded/trader-backend/live.rpc.json", Dst: "./trader-backend/live.rpc.json", Overwrite: false},
		// Candles configs
		{Src: "embedded/candles/live.config.json", Dst: "./candles/live.config.json", Overwrite: false},
		// Docker swarm file
		{Src: "embedded/docker-swarm-stack.yml", Dst: "./docker-swarm-stack.yml", Overwrite: true},
	}
	if err := c.EmbedCopier.Copy(configs.EmbededConfigs, filesToCopy...); err != nil {
		return fmt.Errorf("copying configs to local file system: %w", err)
	}
	fmt.Println(styles.AlertImportant.Render("Please edit your .env and configuration files before proceeding."))
	fmt.Println("The following configuration files will be copied to the 'manager node' for the d8x-trader-backend swarm deployment:")
	for _, f := range filesToCopy {
		fmt.Println(f.Dst)
	}
	c.TUI.NewConfirmation("Press enter to confirm that the configuration files listed above are adjusted...")

	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return fmt.Errorf("finding manager ip address: %w", err)
	}

	pwd, err := c.GetPassword(ctx)
	if err != nil {
		return err
	}

	sshConn, err := c.CreateSSHConn(
		managerIp,
		c.DefaultClusterUserName,
		c.SshKeyPath,
	)
	if err != nil {
		return err
	}

	// Stack might exist, prompt user to remove it
	if _, err := sshConn.ExecCommand(
		"echo '" + pwd + "'| sudo -S docker stack ls | grep " + dockerStackName + " >/dev/null 2>&1",
	); err == nil {
		ok, err := c.TUI.NewPrompt("\nThere seems to be an existing stack deployed. Do you want to remove it before redeploying?", true)
		if err != nil {
			return err
		}
		if ok {
			fmt.Println(styles.ItalicText.Render("Removing existing stack..."))
			out, err := sshConn.ExecCommand(
				fmt.Sprintf(`echo "%s"| sudo -S docker stack rm %s`, pwd, dockerStackName),
			)
			fmt.Println(string(out))
			if err != nil {
				return fmt.Errorf("removing existing stack: %w", err)
			}
		}
	}

	fmt.Println("Enter your referral payment executor private key:")
	pk, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
		components.TextInputOptMasked(),
	)
	if err != nil {
		return err
	}
	pk = strings.TrimPrefix(pk, "0x")
	keyfileLocal := "./trader-backend/keyfile.txt"
	// write keyfile
	if err := c.FS.WriteFile(keyfileLocal, []byte("0x"+pk)); err != nil {
		return fmt.Errorf("temp storage of keyfile failed: %w", err)
	}

	// Lines of docker config commands which we will concat into single
	// bash -c ssh call
	dockerConfigsCMD := []string{
		// "docker config rm cfg_rpc cfg_referral pg_ca cfg_candles",
		"docker config rm cfg_rpc cfg_referral cfg_candles",
		"docker config create cfg_rpc ./trader-backend/live.rpc.json >/dev/null 2>&1",
		"docker config create cfg_referral ./trader-backend/live.referralSettings.json >/dev/null 2>&1",
		"docker config create cfg_candles ./candles/live.config.json >/dev/null 2>&1",
	}

	// List of files to transfer to manager
	copyList := []conn.SftpCopySrcDest{
		{Src: "./trader-backend/.env", Dst: "./trader-backend/.env"},
		{Src: "./trader-backend/live.referralSettings.json", Dst: "./trader-backend/live.referralSettings.json"},
		{Src: "./trader-backend/live.rpc.json", Dst: "./trader-backend/live.rpc.json"},
		{Src: "./trader-backend/keyfile.txt", Dst: "./trader-backend/keyfile.txt"},
		{Src: "./candles/live.config.json", Dst: "./candles/live.config.json"},
		// Note we are renaming to docker-stack.yml on remote!
		{Src: "./docker-swarm-stack.yml", Dst: "./docker-stack.yml"},
	}
	// Include pg.cert
	// if _, err := c.FS.Stat(c.PgCrtPath); err == nil {
	// 	dockerConfigsCMD = append(
	// 		dockerConfigsCMD,
	// 		"docker config create pg_ca ./trader-backend/pg.crt >/dev/null 2>&1",
	// 	)
	// 	copyList = append(copyList,
	// 		conn.SftpCopySrcDest{Src: c.PgCrtPath, Dst: "./trader-backend/pg.crt"},
	// 	)
	// } else {
	// 	fmt.Println(
	// 		styles.ErrorText.Render(c.PgCrtPath + " was not found!"),
	// 	)
	// }
	// Copy files to remote
	fmt.Println(styles.ItalicText.Render("Copying configuration files to manager node " + managerIp))
	if err := sshConn.CopyFilesOverSftp(
		copyList...,
	); err != nil {
		os.Remove(keyfileLocal)
		return fmt.Errorf("copying configuration files to manager: %w", err)
	} else {
		os.Remove(keyfileLocal)
		fmt.Println(styles.SuccessText.Render("configuration files copied to manager"))
	}

	// Create configs
	fmt.Println(styles.ItalicText.Render("Creating docker configs..."))
	out, err := sshConn.ExecCommand(
		fmt.Sprintf(`echo '%s' | sudo -S bash -c "%s"`, pwd, strings.Join(dockerConfigsCMD, ";")),
	)
	fmt.Println(string(out))
	if err != nil {
		return fmt.Errorf("creating docker configs: %w", err)
	}
	fmt.Println(styles.SuccessText.Render("docker configs were created on manager node!"))

	// Deploy swarm stack
	fmt.Println(styles.ItalicText.Render("Deploying docker swarm via manager node..."))
	swarmDeployCMD := fmt.Sprintf(
		`echo '%s' | sudo -S bash -c "docker compose --env-file ./trader-backend/.env -f ./docker-stack.yml config | sed -E 's/published: \"([0-9]+)\"/published: \1/g' | sed -E 's/^name: .*$/ /'|  docker stack deploy -c - %s"`,
		pwd,
		dockerStackName,
	)
	out, err = sshConn.ExecCommand(swarmDeployCMD)
	fmt.Println(string(out))
	if err != nil {
		return fmt.Errorf("swarm deployment failed: %w", err)
	}
	fmt.Println(styles.SuccessText.Render("D8X-trader-backend swarm was deployed"))

	return nil
}

func (c *Container) SwarmNginx(ctx *cli.Context) error {
	// Load config which we will later use to write details about services.
	d8xCfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Copy nginx config and ansible playbook for swarm nginx setup
	if err := c.EmbedCopier.Copy(
		configs.EmbededConfigs,
		files.EmbedCopierOp{Src: "embedded/nginx/nginx.conf", Dst: "./nginx/nginx.conf", Overwrite: true},
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

	setupCertbot, err := c.TUI.NewPrompt("Do you want to setup SSL with certbot for manager server?", true)
	if err != nil {
		return err
	}
	emailForCertbot := ""
	if setupCertbot {
		fmt.Println("Enter your email address for certbot notifications: ")
		email, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("email@domain.com"),
		)
		if err != nil {
			return err
		}
		emailForCertbot = email
	}

	services, err := c.swarmNginxCollectData()
	if err != nil {
		return err
	}

	fmt.Println(
		styles.AlertImportant.Render(
			"Setup DNS A records with your manager IP address",
		),
	)
	fmt.Println("Manager IP address: " + managerIp)
	for _, svc := range services {
		fmt.Printf("Service: %s Domain: %s\n", svc.serviceName, svc.server)
	}
	c.TUI.NewConfirmation("Confirm that you have setup your DNS records to point to your manager's public IP address")

	// Hostnames - domains list provided for certbot
	hostnames := make([]string, len(services))
	for i, svc := range services {
		hostnames[i] = svc.server

		// Store services in d8x config
		d8xCfg.Services[svc.serviceName] = configs.D8XService{
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
			return err
		} else {
			fmt.Println(styles.SuccessText.Render("Manager server certificates setup done!"))
		}
	}

	if err := c.ConfigRWriter.Write(d8xCfg); err != nil {
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
		prompt:      "Enter Main HTTP (sub)domain (e.g. main.d8x.xyz): ",
		placeholder: "main.d8x.xyz",
		find:        "%main%",
		serviceName: configs.D8XServiceMainHTTP,
	},
	{
		prompt:      "Enter Main Websockets (sub)domain (e.g. ws.d8x.xyz): ",
		placeholder: "ws.d8x.xyz",
		find:        "%main_ws%",
		serviceName: configs.D8XServiceMainWS,
	},
	{
		prompt:      "Enter History HTTP (sub)domain (e.g. history.d8x.xyz): ",
		placeholder: "history.d8x.xyz",
		find:        "%history%",
		serviceName: configs.D8XServiceHistory,
	},
	{
		prompt:      "Enter Referral HTTP (sub)domain (e.g. referral.d8x.xyz): ",
		placeholder: "referral.d8x.xyz",
		find:        "%referral%",
		serviceName: configs.D8XServiceReferral,
	},
	{
		prompt:      "Enter Candlesticks Websockets (sub)domain (e.g. candles.d8x.xyz): ",
		placeholder: "candles.d8x.xyz",
		find:        "%candles_ws%",
		serviceName: configs.D8XServiceCandlesWs,
	},
}

// swarmNginxCollectData collects hostnames information and prepares
// nginx.configured.conf file. Returns list of hostnames provided by user
func (c *Container) swarmNginxCollectData() ([]hostnameTuple, error) {

	hosts := make([]string, len(hostsTpl))
	replacements := make([]files.ReplacementTuple, len(hostsTpl))
	for i, h := range hostsTpl {
		fmt.Println(h.prompt)
		input, err := c.TUI.NewInput(
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
	correct, err := c.TUI.NewPrompt("Are these values correct?", true)
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
		"./nginx/nginx.conf",
		"./nginx.configured.conf",
		replacements,
	); err != nil {
		return nil, err
	}

	return hostsTpl, nil
}
