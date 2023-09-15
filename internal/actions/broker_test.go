package actions

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/files"
	"github.com/D8-X/d8x-cli/internal/mocks"
	"github.com/D8-X/d8x-cli/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.uber.org/mock/gomock"
)

func TestBrokerDeploy(t *testing.T) {

	configsToCopy := []files.EmbedCopierOp{
		files.EmbedCopierOp{Src: "embedded/broker-server/chainConfig.json", Dst: "./broker-server/chainConfig.json", Overwrite: false},
		files.EmbedCopierOp{Src: "embedded/broker-server/docker-compose.yml", Dst: "./broker-server/docker-compose.yml", Overwrite: true},
	}

	copySftp := []conn.SftpCopySrcDest{
		{Src: "./broker-server/chainConfig.json", Dst: "./broker/chainConfig.json"},
		{Src: "./broker-server/docker-compose.yml", Dst: "./broker/docker-compose.yml"},
	}

	tests := []struct {
		name             string
		ctx              *cli.Context
		wantErr          string
		expectCopies     func(*mocks.MockEmbedFileCopier)
		expectComponents func(*mocks.MockComponentsRunner)
		expectHostsCfg   func(*mocks.MockHostsFileInteractor)
		expectSSH        func(*mocks.MockSSHConnection)
		expectFS         func(*mocks.MockFSInteractor)
	}{
		{
			name:    "copy configs - error",
			ctx:     &cli.Context{},
			wantErr: assert.AnError.Error(),
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(assert.AnError)
			},
		},
		{
			name:    "get broker ip - error",
			ctx:     &cli.Context{},
			wantErr: assert.AnError.Error(),
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation(
					testutils.MatchStringContains{
						Contains: "broker-server/chainConfig.json",
					},
				).Return(assert.AnError)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("", assert.AnError)
			},
		},
		{
			name:    "get broker private key - error",
			ctx:     &cli.Context{},
			wantErr: assert.AnError.Error(),
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation(
					testutils.MatchStringContains{
						Contains: "broker-server/chainConfig.json",
					},
				).Return(nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
					components.TextInputOptMasked(),
				).Return("", assert.AnError)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-host-127.0.0.1", nil)
			},
		},
		{
			name:    "get broker TBPS value - error",
			ctx:     &cli.Context{},
			wantErr: assert.AnError.Error(),
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation(
					testutils.MatchStringContains{
						Contains: "broker-server/chainConfig.json",
					},
				).Return(nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
					components.TextInputOptMasked(),
				).Return("broker-key", nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("60"),
					components.TextInputOptValue("60"),
				).Return("tbps-value", assert.AnError)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-host-127.0.0.1", nil)
			},
		},
		{
			name:    "copy configs to broker via ssh - error",
			ctx:     &cli.Context{},
			wantErr: assert.AnError.Error(),
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation(
					testutils.MatchStringContains{
						Contains: "broker-server/chainConfig.json",
					},
				).Return(nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
					components.TextInputOptMasked(),
				).Return("broker-key", nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("60"),
					components.TextInputOptValue("60"),
				).Return("tbps-value", nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-host-127.0.0.1", nil)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().CopyFilesOverSftp(
					copySftp,
				).Return(assert.AnError)
			},
		},
		{
			name:    "deploy broker-server - error",
			ctx:     &cli.Context{},
			wantErr: assert.AnError.Error(),
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation(
					testutils.MatchStringContains{
						Contains: "broker-server/chainConfig.json",
					},
				).Return(nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
					components.TextInputOptMasked(),
				).Return("broker-key", nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("60"),
					components.TextInputOptValue("60"),
				).Return("tbps-value", nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-host-127.0.0.1", nil)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().CopyFilesOverSftp(
					copySftp,
				).Return(nil)
				ms.EXPECT().ExecCommand(
					"cd ./broker && echo 'password' | sudo -S BROKER_KEY=broker-key BROKER_FEE_TBPS=tbps-value docker compose up -d",
				).Return([]byte{}, assert.AnError)
			},
		},
		{
			name: "deploy broker-server - ok",
			ctx:  &cli.Context{},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation(
					testutils.MatchStringContains{
						Contains: "broker-server/chainConfig.json",
					},
				).Return(nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("<YOUR PRIVATE KEY>"),
					components.TextInputOptMasked(),
				).Return("broker-key", nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("60"),
					components.TextInputOptValue("60"),
				).Return("tbps-value", nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-host-127.0.0.1", nil)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().CopyFilesOverSftp(
					copySftp,
				).Return(nil)
				ms.EXPECT().ExecCommand(
					"cd ./broker && echo 'password' | sudo -S BROKER_KEY=broker-key BROKER_FEE_TBPS=tbps-value docker compose up -d",
				).Return([]byte{}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctl := gomock.NewController(t)
			fakeEC := mocks.NewMockEmbedFileCopier(ctl)

			ctl2 := gomock.NewController(t)
			fakeCR := mocks.NewMockComponentsRunner(ctl2)

			ctl3 := gomock.NewController(t)
			fakeHC := mocks.NewMockHostsFileInteractor(ctl3)

			ctl4 := gomock.NewController(t)
			fakeSSH := mocks.NewMockSSHConnection(ctl4)

			if tt.expectCopies != nil {
				tt.expectCopies(fakeEC)
			}
			if tt.expectComponents != nil {
				tt.expectComponents(fakeCR)
			}
			if tt.expectHostsCfg != nil {
				tt.expectHostsCfg(fakeHC)
			}
			if tt.expectSSH != nil {
				tt.expectSSH(fakeSSH)
			}
			c := &Container{
				EmbedCopier: fakeEC,
				TUI:         fakeCR,
				HostsCfg:    fakeHC,
				GetPassword: func(ctx *cli.Context) (string, error) {
					return "password", nil
				},
				CreateSSHConn: func(serverIp, user, idFilePath string) (conn.SSHConnection, error) {
					return fakeSSH, nil
				},
			}

			err := c.BrokerDeploy(tt.ctx)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBrokerNginx(t *testing.T) {

	configsToCopy := []files.EmbedCopierOp{
		{Src: "embedded/nginx/nginx-broker.conf", Dst: "./nginx-broker.tpl.conf", Overwrite: true},
		{Src: "embedded/playbooks/broker.ansible.yaml", Dst: "./playbooks/broker.ansible.yaml", Overwrite: true},
	}

	tests := []struct {
		name    string
		ctx     *cli.Context
		wantErr string

		runCmd           func(*exec.Cmd) error
		expectCopies     func(*mocks.MockEmbedFileCopier)
		expectComponents func(*mocks.MockComponentsRunner)
		expectHostsCfg   func(*mocks.MockHostsFileInteractor)
		expectSSH        func(*mocks.MockSSHConnection)
		expectFS         func(*mocks.MockFSInteractor)
		expectCfgRW      func(*mocks.MockD8XConfigReadWriter)
	}{
		{
			name:    "read d8xconf - error",
			wantErr: assert.AnError.Error(),
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(nil, assert.AnError)
			},
		},
		{
			name:    "copy configs - error",
			wantErr: assert.AnError.Error(),
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).
					Return(assert.AnError)
			},
		},
		{
			name:    "get broker ip - error",
			wantErr: assert.AnError.Error(),
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).
					Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("", assert.AnError)
			},
		},
		{
			name:    "do setup nginx - error",
			wantErr: assert.AnError.Error(),
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).
					Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-ip", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewPrompt("Do you want to setup nginx for broker-server?", true).
					Return(false, assert.AnError)
			},
		},
		{
			name:    "do setup certbot - error",
			wantErr: assert.AnError.Error(),
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).
					Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-ip", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewPrompt("Do you want to setup nginx for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewPrompt("Do you want to setup SSL with certbot for broker-server?", true).
					Return(false, assert.AnError)
			},
		},
		{
			name:    "enter email for certbot - error",
			wantErr: assert.AnError.Error(),
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).
					Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-ip", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewPrompt("Do you want to setup nginx for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewPrompt("Do you want to setup SSL with certbot for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("email@domain.com"),
				).Return("", assert.AnError)
			},
		},
		{
			name:    "enter broker-server domain - error",
			wantErr: assert.AnError.Error(),
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).
					Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-ip", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewPrompt("Do you want to setup nginx for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewPrompt("Do you want to setup SSL with certbot for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("email@domain.com"),
				).Return("email@for.certbot", nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("your-broker.domain.com"),
				).Return("", assert.AnError)
			},
		},
		{
			name:    "replace nginx conf hostnames - error",
			wantErr: fmt.Errorf("could not create nginx configuration: %w", assert.AnError).Error(),
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).
					Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-ip", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewPrompt("Do you want to setup nginx for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewPrompt("Do you want to setup SSL with certbot for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("email@domain.com"),
				).Return("email@for.certbot", nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("your-broker.domain.com"),
				).Return("broker-hostname.com", nil)
				mcr.EXPECT().NewConfirmation("Press enter to continue...")
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				mf.EXPECT().
					ReplaceAndCopy(
						"./nginx-broker.tpl.conf",
						"./nginx-broker.configured.conf",
						[]files.ReplacementTuple{
							{
								Find:    `%broker_server%`,
								Replace: "broker-hostname.com",
							},
						},
					).Return(assert.AnError)
			},
		},
		{
			name:    "certbot setup - error",
			wantErr: assert.AnError.Error(),
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).
					Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-ip", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewPrompt("Do you want to setup nginx for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewPrompt("Do you want to setup SSL with certbot for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("email@domain.com"),
				).Return("email@for.certbot", nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("your-broker.domain.com"),
				).Return("broker-hostname.com", nil)
				mcr.EXPECT().NewConfirmation("Press enter to continue...")
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				mf.EXPECT().
					ReplaceAndCopy(
						"./nginx-broker.tpl.conf",
						"./nginx-broker.configured.conf",
						[]files.ReplacementTuple{
							{
								Find:    `%broker_server%`,
								Replace: "broker-hostname.com",
							},
						},
					).Return(nil)
			},
			runCmd: func(c *exec.Cmd) error {
				// Test ansible playbook command args
				return testutils.CheckCmdArgs(
					c,
					"ansible-playbook",
					[]string{
						"--extra-vars", "ansible_ssh_private_key_file='ssh-private-key'",
						"--extra-vars", "ansible_host_key_checking=false",
						"--extra-vars", "ansible_become_pass='password'",
						"-i", configs.DEFAULT_HOSTS_FILE,
						"-u", "test-user",
						"./playbooks/broker.ansible.yaml",
					},
				)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().ExecCommand("echo 'password' | sudo -S certbot --nginx -d broker-hostname.com -n  --agree-tos -m email@for.certbot").
					Return([]byte{}, assert.AnError)
			},
		},
		{
			name:    "write d8x config - error",
			wantErr: fmt.Errorf("could not update config: %w", assert.AnError).Error(),
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				cfg := configs.NewD8XConfig()
				mdxrw.EXPECT().Read().Return(cfg, nil)

				cfg.Services[configs.D8XServiceBrokerServer] = configs.D8XService{
					Name:      configs.D8XServiceBrokerServer,
					UsesHTTPS: true,
					HostName:  "broker-hostname.com",
				}

				mdxrw.EXPECT().Write(cfg).Return(assert.AnError)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).
					Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-ip", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewPrompt("Do you want to setup nginx for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewPrompt("Do you want to setup SSL with certbot for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("email@domain.com"),
				).Return("email@for.certbot", nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("your-broker.domain.com"),
				).Return("broker-hostname.com", nil)
				mcr.EXPECT().NewConfirmation("Press enter to continue...")
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				mf.EXPECT().
					ReplaceAndCopy(
						"./nginx-broker.tpl.conf",
						"./nginx-broker.configured.conf",
						[]files.ReplacementTuple{
							{
								Find:    `%broker_server%`,
								Replace: "broker-hostname.com",
							},
						},
					).Return(nil)
			},
			runCmd: func(c *exec.Cmd) error {
				// Test ansible playbook command args
				return testutils.CheckCmdArgs(
					c,
					"ansible-playbook",
					[]string{
						"--extra-vars", "ansible_ssh_private_key_file='ssh-private-key'",
						"--extra-vars", "ansible_host_key_checking=false",
						"--extra-vars", "ansible_become_pass='password'",
						"-i", configs.DEFAULT_HOSTS_FILE,
						"-u", "test-user",
						"./playbooks/broker.ansible.yaml",
					},
				)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().ExecCommand("echo 'password' | sudo -S certbot --nginx -d broker-hostname.com -n  --agree-tos -m email@for.certbot").
					Return([]byte{}, nil)
			},
		},
		{
			name: "ok",
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				cfg := configs.NewD8XConfig()
				mdxrw.EXPECT().Read().Return(cfg, nil)

				cfg.Services[configs.D8XServiceBrokerServer] = configs.D8XService{
					Name:      configs.D8XServiceBrokerServer,
					UsesHTTPS: true,
					HostName:  "broker-hostname.com",
				}

				mdxrw.EXPECT().Write(cfg).Return(nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).
					Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetBrokerPublicIp().Return("broker-ip", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewPrompt("Do you want to setup nginx for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewPrompt("Do you want to setup SSL with certbot for broker-server?", true).
					Return(true, nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("email@domain.com"),
				).Return("email@for.certbot", nil)
				mcr.EXPECT().NewInput(
					components.TextInputOptPlaceholder("your-broker.domain.com"),
				).Return("broker-hostname.com", nil)
				mcr.EXPECT().NewConfirmation("Press enter to continue...")
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				mf.EXPECT().
					ReplaceAndCopy(
						"./nginx-broker.tpl.conf",
						"./nginx-broker.configured.conf",
						[]files.ReplacementTuple{
							{
								Find:    `%broker_server%`,
								Replace: "broker-hostname.com",
							},
						},
					).Return(nil)
			},
			runCmd: func(c *exec.Cmd) error {
				// Test ansible playbook command args
				return testutils.CheckCmdArgs(
					c,
					"ansible-playbook",
					[]string{
						"--extra-vars", "ansible_ssh_private_key_file='ssh-private-key'",
						"--extra-vars", "ansible_host_key_checking=false",
						"--extra-vars", "ansible_become_pass='password'",
						"-i", configs.DEFAULT_HOSTS_FILE,
						"-u", "test-user",
						"./playbooks/broker.ansible.yaml",
					},
				)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().ExecCommand("echo 'password' | sudo -S certbot --nginx -d broker-hostname.com -n  --agree-tos -m email@for.certbot").
					Return([]byte{}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctl := gomock.NewController(t)
			fakeEC := mocks.NewMockEmbedFileCopier(ctl)

			ctl2 := gomock.NewController(t)
			fakeCR := mocks.NewMockComponentsRunner(ctl2)

			ctl3 := gomock.NewController(t)
			fakeHC := mocks.NewMockHostsFileInteractor(ctl3)

			ctl4 := gomock.NewController(t)
			fakeSSH := mocks.NewMockSSHConnection(ctl4)

			ctl5 := gomock.NewController(t)
			fakeFS := mocks.NewMockFSInteractor(ctl5)

			ctl6 := gomock.NewController(t)
			fakeCRW := mocks.NewMockD8XConfigReadWriter(ctl6)

			if tt.expectCfgRW != nil {
				tt.expectCfgRW(fakeCRW)
			}

			if tt.expectCopies != nil {
				tt.expectCopies(fakeEC)
			}
			if tt.expectComponents != nil {
				tt.expectComponents(fakeCR)
			}
			if tt.expectHostsCfg != nil {
				tt.expectHostsCfg(fakeHC)
			}
			if tt.expectSSH != nil {
				tt.expectSSH(fakeSSH)
			}
			if tt.expectFS != nil {
				tt.expectFS(fakeFS)
			}

			c := &Container{
				EmbedCopier: fakeEC,
				TUI:         fakeCR,
				HostsCfg:    fakeHC,
				GetPassword: func(ctx *cli.Context) (string, error) {
					return "password", nil
				},
				CreateSSHConn: func(serverIp, user, idFilePath string) (conn.SSHConnection, error) {
					return fakeSSH, nil
				},
				FS:                     fakeFS,
				ConfigRWriter:          fakeCRW,
				RunCmd:                 tt.runCmd,
				SshKeyPath:             "ssh-private-key",
				DefaultClusterUserName: "test-user",
			}

			err := c.BrokerServerNginxCertbotSetup(tt.ctx)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
