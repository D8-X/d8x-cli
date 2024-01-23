package actions

import (
	"context"
	"fmt"
	"net/http"
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
	"golang.org/x/net/html"
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
		return err
	}
	if err := c.updateBrokerServerServices(selectedBrokerServicesToUpdate, brokerServices); err != nil {
		return err
	}

	// Run the healthchecks
	return c.HealthCheck(ctx)
}

func (c *Container) updateSwarmServices(ctx *cli.Context, selectedSwarmServicesToUpdate []string, services map[string]configs.DockerService) error {
	if len(selectedSwarmServicesToUpdate) == 0 {
		return nil
	}

	workerIps, err := c.HostsCfg.GetWorkerIps()
	if err != nil {
		return err
	}

	fmt.Println("Pruning unused resources on worker servers...")
	if err := c.PurgeWorkers(workerIps); err != nil {
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
	sshConn, err := conn.NewSSHConnection(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return err
	}

	for _, svcToUpdate := range selectedSwarmServicesToUpdate {
		fmt.Printf("Updating service %s\n", svcToUpdate)

		img := services[svcToUpdate].Image
		tags, err := getTagsWithHashes(svcToUpdate, img)
		// If tags cannot be fetched - show error, but do not exit the update
		// process, since this is only local error on users' machine
		if err != nil {
			fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Could not get tags for %s: %s\n", img, err.Error())))
			tags = []string{}
		}

		// Show selection of available tags (with latest sha hashes whenever
		// possible) to update to
		enterManuallyOption := "Enter image reference manually"
		imagesSelection := make([]string, len(tags))
		for i, tag := range tags {
			imagesSelection[i] = img + ":" + tag
		}
		// Manual entry option
		imagesSelection = append(imagesSelection, enterManuallyOption)
		fmt.Println("Choose which image reference to update to")
		selectedImageToUpdate, err := c.TUI.NewSelection(
			imagesSelection,
			components.SelectionOptAllowOnlySingleItem(),
			components.SelectionOptRequireSelection(),
		)
		if err != nil {
			return err
		}

		imgToUse := selectedImageToUpdate[0]

		if imgToUse != enterManuallyOption {
			fmt.Printf("Using image: %s\n", selectedImageToUpdate[0])
		} else {
			fmt.Printf("Provide a full path to image with tag (or optionally sha256 hash) to update to\n")
			info := "When providing a specific sha version, follow the following format:\nghcr.io/d8-x/<SERVICE>@sha256:<SHA256_HASH>\n"
			fmt.Println(styles.GrayText.Render(info))
			info = "When providing a tag only, follow the following format:\nghcr.io/d8-x/<SERVICE>:<TAG>\n"
			fmt.Println(styles.GrayText.Render(info))

			fmt.Println("Enter image to update to:")
			enteredImage, err := c.TUI.NewInput(
				components.TextInputOptValue(img),
				components.TextInputOptPlaceholder("ghcr.io/d8-x/image@sha256:hash"),
			)
			if err != nil {
				return err
			}
			imgToUse = enteredImage
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

	// See broker.go for broker server setup directory
	brokerDir := "./broker"

	ipBroker, err := c.HostsCfg.GetBrokerPublicIp()
	if err != nil {
		return err
	}
	sshConn, err := conn.NewSSHConnection(ipBroker, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return err
	}

	fmt.Println("Pruning unused resources on broker server...")
	out, err := dockerPrune(sshConn)
	fmt.Println(string(out))
	if err != nil {
		return fmt.Errorf("docker prune on broker server failed: %w", err)
	}
	fmt.Println(styles.SuccessText.Render("Docker prune on broker server completed successfully"))

	// Ask for private key
	pk, _, err := c.CollectAndValidatePrivateKey("Enter your broker private key:")
	if err != nil {
		return err
	}

	// Check if we have broker services configuration information and prompt
	// user to enter it otherwise
	redisPassword := ""
	feeTBPS := ""
	if !cfg.BrokerDeployed {
		fmt.Println(styles.ErrorText.Render("Broker server configuration not found, make sure you have deployed the broker server first (d8x setup broker-deploy), otherwise the update might fail."))
		fmt.Println("Enter your broker redis password:")
		pwd, err := c.TUI.NewInput(
			components.TextInputOptPlaceholder("<YOUR REDIS PASSWORD>"),
		)
		if err != nil {
			return err
		}
		redisPassword = pwd

		fee, err := c.Input.CollectBrokerFee()
		if err != nil {
			return err
		}
		feeTBPS = fee

		// Store these in the config
		cfg.BrokerServerConfig = configs.D8XBrokerServerConfig{
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

		if err := sshConn.ExecCommandPiped(
			fmt.Sprintf(
				`cd %s && docker compose down --rmi all %[2]s && BROKER_FEE_TBPS=%s REDIS_PW=%s docker compose up %[2]s -d`,
				brokerDir,
				svcToUpdate,

				feeTBPS,
				redisPassword,
			),
		); err != nil {
			return err
		} else {
			fmt.Println(styles.SuccessText.Render(fmt.Sprintf("Broker-server service %s updated to latest version", svcToUpdate)))
		}
	}

	return nil
}

// See docker-swarm-stack.yml and github packages for urls
var githubPackageVersionsPage = map[string]string{
	"api":                 "https://github.com/D8-X/d8x-trader-backend/pkgs/container/d8x-trader-main/versions",
	"history":             "https://github.com/D8-X/d8x-trader-backend/pkgs/container/d8x-trader-history/versions",
	"referral":            "https://github.com/D8-X/referral-system/pkgs/container/d8x-referral-system/versions",
	"candles-pyth-client": "https://github.com/D8-X/d8x-candles/pkgs/container/d8x-candles-pyth-client/versions",
	"candles-ws-server":   "https://github.com/D8-X/d8x-candles/pkgs/container/d8x-candles-ws-server/versions",
}

// getTagsWithHashes is a hacky way for us to gather the sha digest hashes of
// the given image from github packages version page. If version page url is
// found - we try to parse the html and find ul li items with tag button <a>.
// The common parent node (<li>) of <a> with tag name contains another child
// with sha256 hash. Returned fullTags slice will contain the sha256 hash
// appended to the tag name.
func getTagsWithHashes(svcName, imgUrl string) (fullTags []string, err error) {
	tags, err := getTags(imgUrl)
	if err != nil {
		return nil, err
	}

	packageUrl, ok := githubPackageVersionsPage[svcName]
	if !ok {
		fmt.Println(
			styles.ErrorText.Render("Could not find package url for service " + svcName + ". Image "),
		)
		return tags, nil
	}

	resp, err := http.DefaultClient.Get(packageUrl)
	if err != nil {
		return tags, nil
	}
	defer resp.Body.Close()

	htmlTree, err := html.Parse(resp.Body)
	if err != nil {
		return tags, nil
	}

	// Inspect the github version page html to see the structure. First we'll
	// find <li> with Box-row class. intialTree := htmlTree
	liItems := []*html.Node{}
	findHtmlNodes(htmlTree, ghTagLiFinder, &liItems)

	// Modify tags slice in place and append sha hashes found from the github
	// packages version pages
	for _, li := range liItems {
		for i, tag := range tags {
			foundHash := ghLiTagShaHashFinder(li, tag)
			if foundHash != "" {
				tags[i] = tag + "@" + foundHash
			}
		}
	}

	return tags, nil
}

// htmlFinderFunc is a function that finds matching  node and a bool indicating
// that node was found.
type htmlFinderFunc func(*html.Node) (*html.Node, bool)

// ghTagLiFinder finds li.Box-row elements which contain the sha hashes of image
// versions.
func ghTagLiFinder(n *html.Node) (*html.Node, bool) {
	if n.Type == html.ElementNode && n.Data == "li" {
		for _, attr := range n.Attr {
			if attr.Key == "class" && strings.Contains(attr.Val, "Box-row") {
				return n, true
			}
		}
	}
	return nil, false
}

// ghLiTagShaHashFinder finds html element which contains link and also text of
// given gitTag
func ghLiTagShaHashFinder(li *html.Node, gitTags string) string {
	// Find the <a> tag with href containing the gitTag
	res := []*html.Node{}
	findHtmlNodes(li, func(n *html.Node) (*html.Node, bool) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" && strings.Contains(strings.ToLower(attr.Val), "?tag="+gitTags) {
					return n, true
				}
			}
		}
		return nil, false
	}, &res)

	// Find any element inside our li node which hash the sha256 hash text
	if len(res) > 0 {
		hashNode := []*html.Node{}
		lastSavedHahs := ""
		findHtmlNodes(li, func(n *html.Node) (*html.Node, bool) {
			if n.Type == html.TextNode {
				innerText := strings.TrimSpace(n.Data)
				if strings.HasPrefix(innerText, "sha256:") {
					lastSavedHahs = innerText
					return n, true
				}
			}
			return nil, false
		}, &hashNode)

		if len(hashNode) > 0 {
			return lastSavedHahs
		}
	}

	return ""
}

func findHtmlNodes(parent *html.Node, f htmlFinderFunc, result *[]*html.Node) {
	foundNode, found := f(parent)

	if found {
		*result = append(*result, foundNode)
	}

	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		findHtmlNodes(c, f, result)
	}
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

// PurgeWorkers removes all all docker artifacts on each worker
func (c *Container) PurgeWorkers(workersIps []string) error {
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	for workerIndex, workerIp := range workersIps {
		fmt.Printf("Running docker prune on worker-%d:\n", workerIndex+1)
		worker, err := c.GetWorkerConnection(workerIp, cfg)
		if err != nil {
			return err
		}

		output, err := dockerPrune(worker)
		fmt.Println(string(output))
		if err != nil {
			return fmt.Errorf("docker prune on worker %d failed: %w", workerIndex+1, err)
		} else {
			fmt.Println(styles.SuccessText.Render(fmt.Sprintf("Docker prune on worker %d completed successfully", workerIndex+1)))
		}
	}

	return nil
}

func dockerPrune(server conn.SSHConnection) ([]byte, error) {
	return server.ExecCommand("docker system prune -a -f --volumes && docker volume prune -f")
}
