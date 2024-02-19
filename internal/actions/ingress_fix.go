package actions

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

// IngressFix fixes non-working ingress network on manager. Steps to fix ingress
// (in order) are: remove any existing stacks; remove ingress on manager;
// recreate ingress on manager; reboot manager's docker; reboot all workers'
// docker.
func (c *Container) IngressFix(ctx *cli.Context) error {
	pwd, err := c.GetPassword(ctx)
	if err != nil {
		return err
	}

	// Remove the ingress on manager
	ip, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}
	managerConn, err := conn.NewSSHConnection(ip, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return err
	}

	// Remove the stack and ingress network
	fmt.Println("Removing stack and ingress network")
	if _, err := managerConn.ExecCommand(
		fmt.Sprintf("docker stack rm %s && yes | docker network rm ingress -f", dockerStackName),
	); err != nil {
		return fmt.Errorf("removing stack and ingress network: %w", err)
	} else {
		fmt.Println(styles.SuccessText.Render("Successfully removed stack and ingress network"))
	}

	fmt.Println("Recreating ingress network")
	time.Sleep(5 * time.Second)
	// Recreate ingress. Make sure subnet is the same as in setup playbook
	if _, err := managerConn.ExecCommand("docker network create -d overlay --subnet 172.16.1.0/24 --ingress ingress"); err != nil {
		return fmt.Errorf("recreating ingress network: %w", err)
	} else {
		fmt.Println(styles.SuccessText.Render("Successfully recreated ingress network"))
	}

	fmt.Println("Restarting docker daemons")

	// Restart the manager's docker
	if _, err := managerConn.ExecCommand(
		fmt.Sprintf(`echo "%s"| sudo -S systemctl restart docker`, pwd),
	); err != nil {
		return fmt.Errorf("restarting manager's docker: %w", err)
	} else {
		fmt.Println(styles.SuccessText.Render("Successfully restarted docker on manager"))
	}

	workerIps, err := c.HostsCfg.GetWorkerIps()
	if err != nil {
		return err
	}

	// Reboot all workers
	wg := sync.WaitGroup{}
	for n, ip := range workerIps {
		n := n
		wg.Add(1)
		go func(ip string) {
			workerNum := n + 1
			defer wg.Done()
			workerConn, err := conn.NewSSHConnectionWithBastion(managerConn.GetClient(), ip, c.DefaultClusterUserName, c.SshKeyPath)
			if err != nil {
				info := fmt.Sprintf("creating ssh connection to worker-%d %s: %s", workerNum, ip, err.Error())
				fmt.Println(styles.ErrorText.Render(info))
			}

			if _, err := workerConn.ExecCommand(
				fmt.Sprintf(`echo "%s"| sudo -S systemctl restart docker`, pwd),
			); err != nil {
				return
			} else {
				fmt.Println(styles.SuccessText.Render("Successfully restarted docker on worker-" + strconv.Itoa(workerNum)))
			}
		}(ip)
	}
	wg.Wait()

	if ctx.Command.Name == "fix-ingress" {
		fmt.Println("Make sure you re-run d8x setup swarm-deploy to re-deploy the services")
	}

	return nil
}
