package actions

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/urfave/cli/v2"
)

// MigrateHelper helps to migrate from old version of cli to newer one. It will
// look for old terraform state file and
func (c *Container) MigrateHelper(ctx *cli.Context) error {
	styles.PrintCommandTitle("Running migrate helper")

	// Attempt to rename old d8x.conf to d8x.conf.json in config directory
	fmt.Println("checking for old d8x.conf file")
	if _, err := os.Stat(c.ConfigDir + "/d8x.conf"); err == nil {
		err := os.Rename(c.ConfigDir+"/d8x.conf", c.ConfigDir+"/d8x.conf.json")
		if err != nil {
			return err
		}
		fmt.Println(
			styles.SuccessText.Render("d8x.conf renamed to d8x.conf.json"),
		)
	} else if err != nil && os.IsNotExist(err) {
		fmt.Println("d8x.conf was not found - ok")
	}

	// If server provider is aws. Attempt to move swarm manager's eip to swarm
	// module, attempt to move the postgres instance and broker instance too.
	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	// Copy terraform files for provider
	fmt.Printf("found server provider: %s\n", cfg.ServerProvider)
	switch cfg.ServerProvider {
	case configs.D8XServerProviderAWS:
		if err := c.CopyAWSTFFiles(); err != nil {
			return err
		}
	}

	// Run terraform init
	cmd := exec.Command("terraform", "init")
	cmd.Dir = c.ProvisioningTfDir
	if err := cmd.Run(); err != nil {
		fmt.Println("failed to run terraform init: " + err.Error())
	} else {
		fmt.Println("terraform init ran successfully")
	}

	// Copy old terraform state file to c.ProvisioningTfDir where new terraform
	// files will reside
	oldTfState, err := filepath.Abs("terraform.tfstate")
	if err != nil {
		return err
	}
	newTfState, err := filepath.Abs(c.ProvisioningTfDir + "/terraform.tfstate")
	if err != nil {
		return err
	}

	if _, err := os.Stat(oldTfState); err == nil {
		if err := exec.Command("cp", "terraform.tfstate", c.ProvisioningTfDir).Run(); err != nil {
			return fmt.Errorf("failed to copy %s to %s: %w", oldTfState, newTfState, err)
		} else {
			fmt.Printf("old state file %s was copied to %s\n", oldTfState, newTfState)
		}
	}

	// Move most important state items: manager elastic ip, postgres instance,
	// broker instance
	switch cfg.ServerProvider {
	case configs.D8XServerProviderAWS:

		// terraform state mv aws_eip.manager_ip module.swarm_servers.aws_eip.manager_ip
		// terraform state mv aws_instance.manager module.swarm_servers.aws_instance.manager

		// Pairs of src and dst of terraform state objects to move
		mvList := [][2]string{
			{"aws_eip.manager_ip", "module.swarm_servers[0].aws_eip.manager_ip"},
			{"aws_instance.manager", "module.swarm_servers[0].aws_instance.manager"},

			{"aws_db_instance.pg", "module.swarm_servers[0].aws_db_instance.pg"},
		}

		for _, srcDst := range mvList {
			cmd := exec.Command("terraform", "state", "mv", srcDst[0], srcDst[1])
			cmd.Dir = c.ProvisioningTfDir

			connectCMDToCurrentTerm(cmd)
			err := cmd.Run()
			if err != nil {
				fmt.Println(err.Error())
				fmt.Printf("failed to move state object %s to %s\n", srcDst[0], srcDst[1])
			} else {
				info := fmt.Sprintf("state object %s moved to %s\n", srcDst[0], srcDst[1])
				fmt.Println(styles.SuccessText.Render(info))
			}
		}

		// Very hacky way of modifying terraform state. But we need to get the
		// ssh key to not get overwritten. We'll modify the name of key directly
		// in the terraform state file

		newTfStateContents, err := os.ReadFile(newTfState)
		if err != nil {
			return err
		}

		state := map[string]any{}
		if err := json.Unmarshal(newTfStateContents, &state); err != nil {
			return err
		}

		// For ssh key name see the aws main.tf file
		updatedState := migrateOldAWSKeyPair(state, cfg.AWSConfig.LabelPrefix+"-cluster-ssh-key")
		marshalledNewState, err := json.MarshalIndent(updatedState, "", "\t")
		if err != nil {
			return err
		}
		if err := os.WriteFile(newTfState, marshalledNewState, 0644); err != nil {
			return err
		}
	}

	return nil
}

type shallowTfState struct {
	Resources []struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		Instances []any  `json:"instances"`
	} `json:"resources"`
}

func migrateOldAWSKeyPair(tfState map[string]any, sshKeyNameAttributeWithoutMd5 string) map[string]any {

	for i, resource := range tfState["resources"].([]any) {
		res, ok := resource.(map[string]any)
		if !ok {
			continue
		}
		if res["name"] == "d8x_cluster_ssh_key" && res["type"] == "aws_key_pair" {
			instances, ok := res["instances"].([]any)
			if ok && len(instances) > 0 {
				keyInstance, ok := instances[0].(map[string]any)
				if ok {
					attrs, ok2 := keyInstance["attributes"].(map[string]any)
					if ok2 {

						// Get the md5 hash of public_key and append it to the
						// name

						if publicKey, ok3 := attrs["public_key"].(string); ok3 {
							md5Sum := md5.Sum([]byte(publicKey))
							md5Str := hex.EncodeToString(md5Sum[:])
							attrs["key_name"] = sshKeyNameAttributeWithoutMd5 + "-" + md5Str

							// Save the data
							keyInstance["attributes"] = attrs
							instances[0] = keyInstance
							res["instances"] = instances
							tfState["resources"].([]any)[i] = res
						}
					}
				}
			}
		}
	}

	return tfState
}
