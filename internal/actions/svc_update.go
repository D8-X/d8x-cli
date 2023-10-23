package actions

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
	styles.ItalicText.Render("Updating swarm services...")

	services, err := configs.GetDockerStackServicesMap(true)
	if err != nil {
		return err
	}

	selection := []string{}
	for svc := range services {
		selection = append(selection, svc)
	}
	sort.Slice(selection, func(i, j int) bool {
		return selection[i] < selection[j]
	})
	selectedServicesToUpdate, err := c.TUI.NewSelection(selection, components.SelectionOptRequireSelection())
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

	for _, svcToUpdate := range selectedServicesToUpdate {
		fmt.Printf("Updating service %s\n", svcToUpdate)
		img := services[svcToUpdate].Image
		tags, err := getTags(img)
		if err != nil {
			fmt.Println(styles.ErrorText.Render(fmt.Sprintf("Could not get tags for %s: %s\n", img, err.Error())))
			continue
		}

		fmt.Printf("Available tags: %s\n", strings.Join(tags, ", "))

		fmt.Printf("Provide a full path to image with tag (or optionally sha256 hash) to update to:\n")
		imgToUse, err := c.TUI.NewInput(
			components.TextInputOptValue(img),
			components.TextInputOptPlaceholder("ghcr.io/d8-x/image@sha256:hash"),
		)
		if err != nil {
			return err
		}

		// Append stack name for service
		svcStackName := dockerStackName + "_" + svcToUpdate

		fmt.Printf("Updating %s to %s\n", svcToUpdate, imgToUse)

		out, err := sshConn.ExecCommand(
			fmt.Sprintf(`docker service update --image %s %s`, imgToUse, svcStackName),
		)
		fmt.Println(string(out))
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
