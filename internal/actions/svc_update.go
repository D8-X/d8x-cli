package actions

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/containers/image/docker"
	"github.com/containers/image/types"
	"github.com/distribution/reference"
	"github.com/urfave/cli/v2"
)

type DokcerStackFileServices struct {
	Services []struct {
	} `yaml:"services"`
}

func (c *Container) ServiceUpdate(ctx *cli.Context) error {
	styles.PrintCommandTitle("Updating swarm services...")

	// Swarm services
	services, err := configs.GetSwarmDockerServices(true)
	if err != nil {
		return err
	}
	swarmSelection := []string{}
	for svc := range services {
		swarmSelection = append(swarmSelection, svc)
	}
	sort.Slice(swarmSelection, func(i, j int) bool {
		return swarmSelection[i] < swarmSelection[j]
	})

	// Broker-server services
	brokerServices, err := configs.GetBrokerServerComposeServices(true)
	if err != nil {
		return err
	}
	brokerServerSelection := []string{}
	for svc := range brokerServices {
		brokerServerSelection = append(brokerServerSelection, svc)
	}
	sort.Slice(brokerServerSelection, func(i, j int) bool {
		return brokerServerSelection[i] < brokerServerSelection[j]
	})

	fmt.Println("Select swarm services to update")
	selectedSwarmServicesToUpdate, err := c.TUI.NewSelection(swarmSelection)
	if err != nil {
		return err
	}

	fmt.Println("Select broker-server services to update")
	selectedBrokerServicesToUpdate, err := c.TUI.NewSelection(brokerServerSelection)
	if err != nil {
		return err
	}

	if err := c.updateSwarmServices(ctx, selectedSwarmServicesToUpdate, services); err != nil {
		return nil
	}
	if err := c.updateBrokerServerServices(selectedBrokerServicesToUpdate, brokerServices); err != nil {
		return nil
	}

	// Run the healthchecks
	return c.HealthCheck(ctx)
}

func (c *Container) updateSwarmServices(ctx *cli.Context, selectedSwarmServicesToUpdate []string, services map[string]configs.DockerService) error {

	password, err := c.GetPassword(ctx)
	if err != nil {
		return err
	}
	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return err
	}
	sshConn, err := conn.NewSSHConnection(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return err
	}

	for _, svcToUpdate := range selectedSwarmServicesToUpdate {
		fmt.Printf("Updating service %s\n", svcToUpdate)

		img := services[svcToUpdate].Image
		tags, err := getTags(img)
		if err != nil {
			fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Could not get tags for %s: %s\n", img, err.Error())))
			continue
		}

		fmt.Printf("Available tags: %s\n\n", strings.Join(tags, ", "))

		fmt.Printf("Provide a full path to image with tag (or optionally sha256 hash) to update to\n")
		info := "When providing a specific sha version, follow the following format:\nghcr.io/d8-x/<SERVICE>@sha256:<SHA256_HASH>\n"
		fmt.Println(styles.GrayText.Render(info))
		info = "When providing a tag only, follow the following format:\nghcr.io/d8-x/<SERVICE>:<TAG>\n"
		fmt.Println(styles.GrayText.Render(info))

		fmt.Println("Enter image to update to:")
		imgToUse, err := c.TUI.NewInput(
			components.TextInputOptValue(img),
			components.TextInputOptPlaceholder("ghcr.io/d8-x/image@sha256:hash"),
		)
		if err != nil {
			return err
		}

		// For referral system - we need to update the referral executor private
		// key, since the new version will have different encryption key and
		// keyfile.txt will be reencrypted
		// var oldKeyfile string = ""
		if svcToUpdate == "referral" {

			// Remove existing referral service
			fmt.Println("Scaling down referral service")
			if err := sshConn.ExecCommandPiped(
				fmt.Sprintf("docker service scale %s_%s=0", dockerStackName, svcToUpdate),
			); err != nil {
				fmt.Println(styles.ErrorText.Render(
					fmt.Sprintf("removing referral service: %v\n", err),
				))
				continue
			}

			// Store old key just in case
			_, err := sshConn.ExecCommand(fmt.Sprintf(`echo '%s' | sudo -S cat /var/nfs/general/keyfile.txt`, password))
			if err != nil {
				return err
			}

			fmt.Println("Enter your referral payment executor private key:")
			executorkey, err := c.TUI.NewInput(
				components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
				components.TextInputOptMasked(),
			)
			if err != nil {
				return err
			}
			executorkey = "0x" + strings.TrimPrefix(executorkey, "0x")

			// Write new keyfile
			out, err := sshConn.ExecCommand(fmt.Sprintf(`echo '%s' | sudo -S bash -c "echo -n '%s' > /var/nfs/general/keyfile.txt"`, password, executorkey))
			if err != nil {
				fmt.Println(string(out))
				return fmt.Errorf("updating executor private key file: %w", err)
			}
		}

		// Append stack name for service
		svcStackName := dockerStackName + "_" + svcToUpdate

		fmt.Printf("Updating %s to %s\n", svcToUpdate, imgToUse)

		t := time.NewTimer(time.Minute * 2)
		done := make(chan struct{})
		go func() {
			err := sshConn.ExecCommandPiped(
				fmt.Sprintf(`docker service update --image %s %s`, imgToUse, svcStackName),
			)
			// fmt.Println(string(out))
			if err != nil {
				fmt.Println(
					styles.ErrorText.Render(
						fmt.Sprintf("Could not update service %s: %s\n", svcToUpdate, err.Error()),
					),
				)
			} else {
				fmt.Println(
					styles.SuccessText.Render(
						fmt.Sprintf("Service %s updated successfully\n", svcToUpdate),
					),
				)
			}

			// Scale back the referral service
			if svcToUpdate == "referral" {
				if err := sshConn.ExecCommandPiped(
					fmt.Sprintf("docker service scale %s_%s=1", dockerStackName, svcToUpdate),
				); err != nil {
					fmt.Println(styles.ErrorText.Render(
						fmt.Sprintf("scaling referral service: %v\n", err),
					))
				}
			}

			done <- struct{}{}
		}()

		select {
		case <-t.C:
			fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Service %s update timed out\n", svcToUpdate)))
		case <-done:
		}

	}

	return nil
}

// updateBrokerServerServices performs broker-server services update on broker
// server. Broker-server update involves  uploading the key to a new volume.
func (c *Container) updateBrokerServerServices(selectedSwarmServicesToUpdate []string, services map[string]configs.DockerService) error {
	if len(selectedSwarmServicesToUpdate) == 0 {
		return nil
	}

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// See broker.go for broker server setup
	brokerDir := "./broker"

	ipBroker, err := c.HostsCfg.GetBrokerPublicIp()
	if err != nil {
		return err
	}
	sshConn, err := conn.NewSSHConnection(ipBroker, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return err
	}

	// Ask for private key
	fmt.Println("Enter your broker private key:")
	pk, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
		components.TextInputOptMasked(),
	)
	if err != nil {
		return err
	}

	// Check if we have broker services configuration information and prompt
	// user to enter it otherwise
	redisPassword := ""
	feeTBPS := ""
	if cfg.BrokerServerConfig == nil {
		fmt.Println(styles.ErrorText.Render("Broker server configuration not found"))
		fmt.Println("Enter your broker redis password:")
		pwd, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("<YOUR REDIS PASSWORD>"),
		)
		if err != nil {
			return err
		}
		redisPassword = pwd

		fmt.Println("Enter your broker fee tbps value:")
		fee, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("60"),
		)
		if err != nil {
			return err
		}
		feeTBPS = fee

		// Store these in the config
		cfg.BrokerServerConfig = &configs.D8XBrokerServerConfig{
			FeeTBPS:       feeTBPS,
			RedisPassword: redisPassword,
		}
		if err := c.ConfigRWriter.Write(cfg); err != nil {
			return err
		}
	} else {
		redisPassword = cfg.BrokerServerConfig.RedisPassword
		feeTBPS = cfg.BrokerServerConfig.FeeTBPS
	}

	fmt.Printf("Using BROKER_FEE_TBPS=%s REDIS_PW=%s\n", feeTBPS, redisPassword)

	for _, svcToUpdate := range selectedSwarmServicesToUpdate {
		fmt.Printf("Updating service %s to latest version\n", svcToUpdate)

		// For service broker - we need to recreate the volume with keyfile
		if svcToUpdate == "broker" {
			// Stop broker service
			// Remove the keyfile volume
			cmd := "cd %s && docker compose down --rmi all %s && docker volume rm %s"
			cmd = fmt.Sprintf(cmd, brokerDir, svcToUpdate, BROKER_KEY_VOL_NAME)
			out, err := sshConn.ExecCommand(cmd)
			if err != nil {
				fmt.Println(string(out))
				return fmt.Errorf("preparing docker volume for keyfile: %w", err)
			}
			out, err = c.brokerServerKeyVolSetup(sshConn, pk)
			if err != nil {
				fmt.Println(string(out))
				return fmt.Errorf("creating docker volume with keyfile: %w", err)
			}
		}

		out, err := sshConn.ExecCommand(
			fmt.Sprintf(
				`cd %s && docker compose down --rmi all %[2]s && BROKER_FEE_TBPS=%s REDIS_PW=%s docker compose up %[2]s -d`,
				brokerDir,
				svcToUpdate,

				feeTBPS,
				redisPassword,
			),
		)

		if err != nil {
			fmt.Println(string(out))
			return err
		} else {
			fmt.Println(styles.SuccessText.Render(fmt.Sprintf("Broker-server service %s updated to latest version", svcToUpdate)))
		}
	}

	return nil
}

// getTags retrieves available tags for given image
func getTags(imgUrl string) ([]string, error) {
	ref, err := reference.ParseNormalizedNamed(imgUrl)
	if err != nil {
		return nil, err
	}

	imgRef, err := docker.NewReference(reference.TagNameOnly(ref))
	if err != nil {
		return nil, err
	}

	return docker.GetRepositoryTags(
		context.Background(),
		&types.SystemContext{},
		imgRef,
	)
}
