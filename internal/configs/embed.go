package configs

import (
	"embed"
	"io"
	"strings"

	"github.com/go-yaml/yaml"
)

//go:embed embedded/*
var EmbededConfigs embed.FS

// GetDockerStackFile reads the embedded docker-swarm-stack.yml file and returns
// it's contents
func GetDockerStackFile() ([]byte, error) {
	fs, err := EmbededConfigs.Open("embedded/docker-swarm-stack.yml")
	if err != nil {
		return nil, err
	}

	return io.ReadAll(fs)
}

type DockerService struct {
	// Image without the tag
	Image string `yaml:"image"`
}

// GetDockerStackServicesMap parses embedded docker swarm stack file and builds
// services map. If onlyD8X is true, only d8x services are returned. Images are returned without the tag
func GetDockerStackServicesMap(onlyD8X bool) (map[string]DockerService, error) {
	ret := map[string]DockerService{}

	stackYaml, err := GetDockerStackFile()
	if err != nil {
		return nil, err
	}
	mp := map[string]any{}
	if err := yaml.Unmarshal(stackYaml, &mp); err != nil {
		return nil, err
	}

	for service, values := range mp["services"].(map[any]any) {
		vals := values.(map[any]any)
		img := vals["image"].(string)
		svcName := service.(string)

		if onlyD8X {
			if !strings.Contains(strings.ToLower(img), "d8-x") {
				continue
			}
		}

		// Remove the tag
		img = strings.Split(img, ":")[0]

		ret[svcName] = DockerService{
			Image: img,
		}
	}

	return ret, nil
}
