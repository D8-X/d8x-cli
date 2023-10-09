package actions

import (
	"testing"

	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/stretchr/testify/assert"
)

func TestGenerateArgs(t *testing.T) {

	tests := []struct {
		name    string
		l       linodeConfigurer
		wantOut []string
	}{
		{
			name: "ok",
			l: linodeConfigurer{
				D8XLinodeConfig: configs.D8XLinodeConfig{
					Token:              "token",
					DbId:               "123",
					Region:             "eu-north",
					LabelPrefix:        "prefix",
					CreateBrokerServer: false,
				},
				authorizedKey: "ssh-pub",
			},
			wantOut: []string{
				"apply", "-auto-approve",
				"-var", `authorized_keys=["ssh-pub"]`,
				"-var", `region=eu-north`,
				"-var", `server_label_prefix=prefix`,
				"-var", `create_broker_server=false`,
				"-var", `linode_db_cluster_id=123`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.l.generateArgs()
			assert.Equal(t, tt.wantOut, args)
		})
	}
}
