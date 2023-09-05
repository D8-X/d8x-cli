package actions

import (
	"testing"

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
				linodeToken:            "token",
				linodeDbId:             "123",
				linodeRegion:           "eu-north",
				linodeNodesLabelPrefix: "prefix",
				authorizedKey:          "ssh-pub",
				createBroker:           false,
			},
			wantOut: []string{
				"apply", "-auto-approve",
				"-var", `authorized_keys=["ssh-pub"]`,
				"-var", `linode_db_cluster_id=123`,
				"-var", `region=eu-north`,
				"-var", `server_label_prefix=prefix`,
				"-var", `create_broker_server=false`,
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
