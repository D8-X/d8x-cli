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

func TestSwarmDeploy(t *testing.T) {

	configsToCopy := []files.EmbedCopierOp{
		{Src: "embedded/trader-backend/env.example", Dst: "./trader-backend/.env", Overwrite: false},
		{Src: "embedded/trader-backend/live.referralSettings.json", Dst: "./trader-backend/live.referralSettings.json", Overwrite: false},
		{Src: "embedded/trader-backend/live.rpc.json", Dst: "./trader-backend/live.rpc.json", Overwrite: false},
		{Src: "embedded/trader-backend/live.wsConfig.json", Dst: "./trader-backend/live.wsConfig.json", Overwrite: false},
		{Src: "embedded/candles/live.config.json", Dst: "./candles/live.config.json", Overwrite: false},
		{Src: "embedded/docker-swarm-stack.yml", Dst: "./docker-swarm-stack.yml", Overwrite: true},
	}

	configsToSendViaSFTP := []conn.SftpCopySrcDest{
		{Src: "./trader-backend/.env", Dst: "./trader-backend/.env"},
		{Src: "./trader-backend/live.referralSettings.json", Dst: "./trader-backend/live.referralSettings.json"},
		{Src: "./trader-backend/live.rpc.json", Dst: "./trader-backend/live.rpc.json"},
		{Src: "./trader-backend/live.wsConfig.json", Dst: "./trader-backend/live.wsConfig.json"},
		{Src: "./candles/live.config.json", Dst: "./candles/live.config.json"},
		// Note we are renaming to docker-stack.yml on remote!
		{Src: "./docker-swarm-stack.yml", Dst: "./docker-stack.yml"},

		// In tests we also include pg.crt
		{Src: "./pg.crt", Dst: "./trader-backend/pg.crt"},
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
			name: "copy embed configs - return err",
			ctx:  &cli.Context{},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(assert.AnError)
			},
			wantErr: fmt.Errorf("copying configs to local file system: %w", assert.AnError).Error(),
		},
		{
			name: "get manager ip - return err",
			ctx:  &cli.Context{},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation("Press enter to confirm that the configuration files listed above are adjusted...").Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("", assert.AnError)
			},
			wantErr: fmt.Errorf("finding manager ip address: %w", assert.AnError).Error(),
		},
		{
			name: "docker stack check - component error",
			ctx:  &cli.Context{},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation("Press enter to confirm that the configuration files listed above are adjusted...").Return(nil)
				mcr.EXPECT().
					NewPrompt("\nThere seems to be an existing stack deployed. Do you want to remove it before redeploying?", true).
					Return(false, assert.AnError)

			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().
					ExecCommand("echo 'password'| sudo -S docker stack ls | grep stack >/dev/null 2>&1").
					Return([]byte(""), nil)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "docker stack remove - ssh error",
			ctx:  &cli.Context{},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation("Press enter to confirm that the configuration files listed above are adjusted...").Return(nil)
				mcr.EXPECT().
					NewPrompt("\nThere seems to be an existing stack deployed. Do you want to remove it before redeploying?", true).
					Return(true, nil)

			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().
					ExecCommand("echo 'password'| sudo -S docker stack ls | grep stack >/dev/null 2>&1").
					Return([]byte(""), nil)
				ms.EXPECT().
					ExecCommand(`echo "password"| sudo -S docker stack rm stack`).
					Return([]byte(""), assert.AnError)

			},
			wantErr: fmt.Errorf("removing existing stack: %w", assert.AnError).Error(),
		},
		{
			name: "append pg.crt - error",
			ctx:  &cli.Context{},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation("Press enter to confirm that the configuration files listed above are adjusted...").Return(nil)
				mcr.EXPECT().
					NewPrompt("\nThere seems to be an existing stack deployed. Do you want to remove it before redeploying?", true).
					Return(true, nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().
					ExecCommand("echo 'password'| sudo -S docker stack ls | grep stack >/dev/null 2>&1").
					Return([]byte(""), nil)
				ms.EXPECT().
					ExecCommand(`echo "password"| sudo -S docker stack rm stack`).
					Return([]byte(""), nil)
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				mf.EXPECT().Stat("./pg.crt").Return(nil, assert.AnError)
			},
			wantErr: "./pg.crt was not found!",
		},
		{
			name: "copy configs over sftp - error",
			ctx:  &cli.Context{},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation("Press enter to confirm that the configuration files listed above are adjusted...").Return(nil)
				mcr.EXPECT().
					NewPrompt("\nThere seems to be an existing stack deployed. Do you want to remove it before redeploying?", true).
					Return(true, nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().
					ExecCommand("echo 'password'| sudo -S docker stack ls | grep stack >/dev/null 2>&1").
					Return([]byte(""), nil)
				ms.EXPECT().
					ExecCommand(`echo "password"| sudo -S docker stack rm stack`).
					Return([]byte(""), nil)
				ms.EXPECT().
					CopyFilesOverSftp(
						configsToSendViaSFTP,
					).
					Return(assert.AnError)
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				mf.EXPECT().Stat("./pg.crt").Return(nil, nil)
			},
			wantErr: fmt.Errorf("copying configuration files to manager: %w", assert.AnError).Error(),
		},

		{
			name: "create docker configs over ssh - error",
			ctx:  &cli.Context{},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation("Press enter to confirm that the configuration files listed above are adjusted...").Return(nil)
				mcr.EXPECT().
					NewPrompt("\nThere seems to be an existing stack deployed. Do you want to remove it before redeploying?", true).
					Return(true, nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().
					ExecCommand("echo 'password'| sudo -S docker stack ls | grep stack >/dev/null 2>&1").
					Return([]byte(""), nil)
				ms.EXPECT().
					ExecCommand(`echo "password"| sudo -S docker stack rm stack`).
					Return([]byte(""), nil)
				ms.EXPECT().
					CopyFilesOverSftp(
						configsToSendViaSFTP,
					).
					Return(nil)

					// Test if all required configs are created (including pg_cert)
				ms.EXPECT().
					ExecCommand(
						`echo 'password' | sudo -S bash -c "docker config rm cfg_rpc cfg_referral pg_ca cfg_candles;docker config create cfg_rpc ./trader-backend/live.rpc.json >/dev/null 2>&1;docker config create cfg_referral ./trader-backend/live.referralSettings.json >/dev/null 2>&1;docker config create cfg_candles ./candles/live.config.json >/dev/null 2>&1;docker config create pg_ca ./trader-backend/pg.crt >/dev/null 2>&1"`,
					).
					Return(nil, assert.AnError)

			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				mf.EXPECT().Stat("./pg.crt").Return(nil, nil)
			},
			wantErr: fmt.Errorf("creating docker configs: %w", assert.AnError).Error(),
		},
		{
			name: "deploy swarm cluster - error",
			ctx:  &cli.Context{},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation("Press enter to confirm that the configuration files listed above are adjusted...").Return(nil)
				mcr.EXPECT().
					NewPrompt("\nThere seems to be an existing stack deployed. Do you want to remove it before redeploying?", true).
					Return(true, nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().
					ExecCommand("echo 'password'| sudo -S docker stack ls | grep stack >/dev/null 2>&1").
					Return([]byte(""), nil)
				ms.EXPECT().
					ExecCommand(`echo "password"| sudo -S docker stack rm stack`).
					Return([]byte(""), nil)
				ms.EXPECT().
					CopyFilesOverSftp(
						configsToSendViaSFTP,
					).
					Return(nil)

					// Test if all required configs are created (including pg_cert)
				ms.EXPECT().
					ExecCommand(
						`echo 'password' | sudo -S bash -c "docker config rm cfg_rpc cfg_referral pg_ca cfg_candles;docker config create cfg_rpc ./trader-backend/live.rpc.json >/dev/null 2>&1;docker config create cfg_referral ./trader-backend/live.referralSettings.json >/dev/null 2>&1;docker config create cfg_candles ./candles/live.config.json >/dev/null 2>&1;docker config create pg_ca ./trader-backend/pg.crt >/dev/null 2>&1"`,
					).
					Return([]byte(""), nil)
					// Deploy swarm cmd test
				ms.EXPECT().
					ExecCommand(
						`echo 'password' | sudo -S bash -c "docker compose --env-file ./trader-backend/.env -f ./docker-stack.yml config | sed -E 's/published: \"([0-9]+)\"/published: \1/g' | sed -E 's/^name: .*$/ /'|  docker stack deploy -c - stack"`,
					).
					Return([]byte(""), assert.AnError)

			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				mf.EXPECT().Stat("./pg.crt").Return(nil, nil)
			},
			wantErr: fmt.Errorf("swarm deployment failed: %w", assert.AnError).Error(),
		},
		{
			name: "deploy swarm cluster - ok",
			ctx:  &cli.Context{},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().
					Copy(
						configs.EmbededConfigs,
						configsToCopy,
					).Return(nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().NewConfirmation("Press enter to confirm that the configuration files listed above are adjusted...").Return(nil)
				mcr.EXPECT().
					NewPrompt("\nThere seems to be an existing stack deployed. Do you want to remove it before redeploying?", true).
					Return(true, nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				ms.EXPECT().
					ExecCommand("echo 'password'| sudo -S docker stack ls | grep stack >/dev/null 2>&1").
					Return([]byte(""), nil)
				ms.EXPECT().
					ExecCommand(`echo "password"| sudo -S docker stack rm stack`).
					Return([]byte(""), nil)
				ms.EXPECT().
					CopyFilesOverSftp(
						configsToSendViaSFTP,
					).
					Return(nil)

					// Test if all required configs are created (including pg_cert)
				ms.EXPECT().
					ExecCommand(
						`echo 'password' | sudo -S bash -c "docker config rm cfg_rpc cfg_referral pg_ca cfg_candles;docker config create cfg_rpc ./trader-backend/live.rpc.json >/dev/null 2>&1;docker config create cfg_referral ./trader-backend/live.referralSettings.json >/dev/null 2>&1;docker config create cfg_candles ./candles/live.config.json >/dev/null 2>&1;docker config create pg_ca ./trader-backend/pg.crt >/dev/null 2>&1"`,
					).
					Return([]byte(""), nil)
					// Deploy swarm cmd test
				ms.EXPECT().
					ExecCommand(
						`echo 'password' | sudo -S bash -c "docker compose --env-file ./trader-backend/.env -f ./docker-stack.yml config | sed -E 's/published: \"([0-9]+)\"/published: \1/g' | sed -E 's/^name: .*$/ /'|  docker stack deploy -c - stack"`,
					).
					Return([]byte(""), nil)

			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				mf.EXPECT().Stat("./pg.crt").Return(nil, nil)
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
				FS:        fakeFS,
				PgCrtPath: "./pg.crt",
			}

			err := c.SwarmDeploy(tt.ctx)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSwarmNginx(t *testing.T) {

	configsToCopy := []files.EmbedCopierOp{
		{Src: "embedded/nginx/nginx.conf", Dst: "./nginx/nginx.conf", Overwrite: true},
		{Src: "embedded/playbooks/nginx.ansible.yaml", Dst: "./playbooks/nginx.ansible.yaml", Overwrite: true},
	}

	tests := []struct {
		name    string
		ctx     *cli.Context
		wantErr string
		runCmd  func(*exec.Cmd) error

		expectCopies     func(*mocks.MockEmbedFileCopier)
		expectCfgRW      func(*mocks.MockD8XConfigReadWriter)
		expectComponents func(*mocks.MockComponentsRunner)
		expectHostsCfg   func(*mocks.MockHostsFileInteractor)
		expectSSH        func(*mocks.MockSSHConnection)
		expectFS         func(*mocks.MockFSInteractor)
	}{
		{
			name: "get cfg read writer - error",
			ctx:  &cli.Context{},
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(nil, assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "get cfg read writer - error",
			ctx:  &cli.Context{},
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).Return(assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "get manager ip addr - error",
			ctx:  &cli.Context{},
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("", assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "prompt to setup certbot - error",
			ctx:  &cli.Context{},
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().
					NewPrompt("Do you want to setup SSL with certbot for manager server?", true).
					Return(false, assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "prompt to enter email for certbot setup - error",
			ctx:  &cli.Context{},
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().
					NewPrompt("Do you want to setup SSL with certbot for manager server?", true).
					Return(true, nil)
				mcr.EXPECT().
					NewInput(
						components.TextInputOptPlaceholder("email@domain.com"),
					).
					Return("", assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "configure nginx.conf replacements - error",
			ctx:  &cli.Context{},
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().
					NewPrompt("Do you want to setup SSL with certbot for manager server?", true).
					Return(true, nil)
				mcr.EXPECT().
					NewInput(
						components.TextInputOptPlaceholder("email@domain.com"),
					).
					Return("email@for.certbot", nil)
				// Setup fake inputs for nginx setup
				for _, h := range hostsTpl {
					mcr.EXPECT().
						NewInput(
							components.TextInputOptPlaceholder(h.placeholder),
						).
						// Return placeholder as server (domain) names
						Return(h.placeholder, nil)
				}

				// Confirm that values are correct
				mcr.EXPECT().NewPrompt("Are these values correct?", true).
					Return(true, nil)
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				// Generate replacements (Reaplce with h.placeholder as defined in expectComponents)
				replacements := make([]files.ReplacementTuple, len(hostsTpl))
				for i, h := range hostsTpl {
					replacements[i].Find = h.find
					replacements[i].Replace = h.placeholder
				}

				mf.EXPECT().
					ReplaceAndCopy(
						"./nginx/nginx.conf",
						"./nginx.configured.conf",
						replacements,
					).Return(assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "run ansible-playbook for nginx setup on manager - error",
			ctx:  &cli.Context{},
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().
					NewPrompt("Do you want to setup SSL with certbot for manager server?", true).
					Return(true, nil)
				mcr.EXPECT().
					NewInput(
						components.TextInputOptPlaceholder("email@domain.com"),
					).
					Return("email@for.certbot", nil)

				// Setup fake inputs for nginx setup
				for _, h := range hostsTpl {
					mcr.EXPECT().
						NewInput(
							components.TextInputOptPlaceholder(h.placeholder),
						).
						// Return placeholder as server (domain) names
						Return(h.placeholder, nil)
				}

				// Confirm that values are correct
				mcr.EXPECT().NewPrompt("Are these values correct?", true).
					Return(true, nil)

				mcr.EXPECT().NewConfirmation("Confirm that you have setup your DNS records to point to your manager's public IP address")
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				// Generate replacements (Reaplce with h.placeholder as defined in expectComponents)
				replacements := make([]files.ReplacementTuple, len(hostsTpl))
				for i, h := range hostsTpl {
					replacements[i].Find = h.find
					replacements[i].Replace = h.placeholder
				}

				mf.EXPECT().
					ReplaceAndCopy(
						"./nginx/nginx.conf",
						"./nginx.configured.conf",
						replacements,
					).Return(nil)
			},
			runCmd: func(c *exec.Cmd) error {
				return assert.AnError
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "run certbot nginx setup on manager - error",
			ctx:  &cli.Context{},
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {
				mdxrw.EXPECT().Read().Return(configs.NewD8XConfig(), nil)
			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().
					NewPrompt("Do you want to setup SSL with certbot for manager server?", true).
					Return(true, nil)
				mcr.EXPECT().
					NewInput(
						components.TextInputOptPlaceholder("email@domain.com"),
					).
					Return("email@for.certbot", nil)
				mcr.EXPECT().NewConfirmation("Confirm that you have setup your DNS records to point to your manager's public IP address")

				// Setup fake inputs for nginx setup
				for _, h := range hostsTpl {
					mcr.EXPECT().
						NewInput(
							components.TextInputOptPlaceholder(h.placeholder),
						).
						// Return placeholder as server (domain) names
						Return(h.placeholder, nil)
				}

				// Confirm that values are correct
				mcr.EXPECT().NewPrompt("Are these values correct?", true).
					Return(true, nil)
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				// Generate replacements (Reaplce with h.placeholder as defined in expectComponents)
				replacements := make([]files.ReplacementTuple, len(hostsTpl))
				for i, h := range hostsTpl {
					replacements[i].Find = h.find
					replacements[i].Replace = h.placeholder
				}

				mf.EXPECT().
					ReplaceAndCopy(
						"./nginx/nginx.conf",
						"./nginx.configured.conf",
						replacements,
					).Return(nil)
			},
			runCmd: func(c *exec.Cmd) error {
				return testutils.CheckCmdArgs(
					c,
					"ansible-playbook",
					[]string{
						"--extra-vars", `ansible_ssh_private_key_file='./private-key'`,
						"--extra-vars", "ansible_host_key_checking=false",
						"--extra-vars", `ansible_become_pass='password'`,
						"-i", configs.DEFAULT_HOSTS_FILE,
						"-u", "user-tester",
						"./playbooks/nginx.ansible.yaml",
					},
				)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				// -d domains are taken from global hostsTpl
				ms.EXPECT().
					ExecCommand(`echo 'password' | sudo -S certbot --nginx -d main.d8x.xyz,ws.d8x.xyz,history.d8x.xyz,referral.d8x.xyz,candles.d8x.xyz -n  --agree-tos -m email@for.certbot`).
					Return([]byte{}, assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "write d8x config - error",
			ctx:  &cli.Context{},
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {

				cfg := configs.NewD8XConfig()
				mdxrw.EXPECT().Read().Return(cfg, nil)

				// Write services details from global hostsTpl
				for _, h := range hostsTpl {
					cfg.Services[h.serviceName] = configs.D8XService{
						Name:      h.serviceName,
						UsesHTTPS: true,
						// We simply use the placholders in this test func
						HostName: h.placeholder,
					}
				}

				mdxrw.EXPECT().Write(cfg).Return(assert.AnError)

			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().
					NewPrompt("Do you want to setup SSL with certbot for manager server?", true).
					Return(true, nil)
				mcr.EXPECT().
					NewInput(
						components.TextInputOptPlaceholder("email@domain.com"),
					).
					Return("email@for.certbot", nil)
				mcr.EXPECT().NewConfirmation("Confirm that you have setup your DNS records to point to your manager's public IP address")

				// Setup fake inputs for nginx setup
				for _, h := range hostsTpl {
					mcr.EXPECT().
						NewInput(
							components.TextInputOptPlaceholder(h.placeholder),
						).
						// Return placeholder as server (domain) names
						Return(h.placeholder, nil)
				}

				// Confirm that values are correct
				mcr.EXPECT().NewPrompt("Are these values correct?", true).
					Return(true, nil)
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				// Generate replacements (Reaplce with h.placeholder as defined in expectComponents)
				replacements := make([]files.ReplacementTuple, len(hostsTpl))
				for i, h := range hostsTpl {
					replacements[i].Find = h.find
					replacements[i].Replace = h.placeholder
				}

				mf.EXPECT().
					ReplaceAndCopy(
						"./nginx/nginx.conf",
						"./nginx.configured.conf",
						replacements,
					).Return(nil)
			},
			runCmd: func(c *exec.Cmd) error {
				return testutils.CheckCmdArgs(
					c,
					"ansible-playbook",
					[]string{
						"--extra-vars", `ansible_ssh_private_key_file='./private-key'`,
						"--extra-vars", "ansible_host_key_checking=false",
						"--extra-vars", `ansible_become_pass='password'`,
						"-i", configs.DEFAULT_HOSTS_FILE,
						"-u", "user-tester",
						"./playbooks/nginx.ansible.yaml",
					},
				)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				// -d domains are taken from global hostsTpl
				ms.EXPECT().
					ExecCommand(`echo 'password' | sudo -S certbot --nginx -d main.d8x.xyz,ws.d8x.xyz,history.d8x.xyz,referral.d8x.xyz,candles.d8x.xyz -n  --agree-tos -m email@for.certbot`).
					Return([]byte{}, nil)
			},
			wantErr: fmt.Errorf("could not update config: %w", assert.AnError).Error(),
		},

		{
			name: "write d8x config - ok",
			ctx:  &cli.Context{},
			expectCfgRW: func(mdxrw *mocks.MockD8XConfigReadWriter) {

				cfg := configs.NewD8XConfig()
				mdxrw.EXPECT().Read().Return(cfg, nil)

				// Write services details from global hostsTpl
				for _, h := range hostsTpl {
					cfg.Services[h.serviceName] = configs.D8XService{
						Name:      h.serviceName,
						UsesHTTPS: true,
						// We simply use the placholders in this test func
						HostName: h.placeholder,
					}
				}

				mdxrw.EXPECT().Write(cfg).Return(nil)

			},
			expectCopies: func(mefc *mocks.MockEmbedFileCopier) {
				mefc.EXPECT().Copy(
					configs.EmbededConfigs,
					configsToCopy,
				).Return(nil)
			},
			expectHostsCfg: func(mhfi *mocks.MockHostsFileInteractor) {
				mhfi.EXPECT().GetMangerPublicIp().Return("127.0.0.1", nil)
			},
			expectComponents: func(mcr *mocks.MockComponentsRunner) {
				mcr.EXPECT().
					NewPrompt("Do you want to setup SSL with certbot for manager server?", true).
					Return(true, nil)
				mcr.EXPECT().
					NewInput(
						components.TextInputOptPlaceholder("email@domain.com"),
					).
					Return("email@for.certbot", nil)
				mcr.EXPECT().NewConfirmation("Confirm that you have setup your DNS records to point to your manager's public IP address")

				// Setup fake inputs for nginx setup
				for _, h := range hostsTpl {
					mcr.EXPECT().
						NewInput(
							components.TextInputOptPlaceholder(h.placeholder),
						).
						// Return placeholder as server (domain) names
						Return(h.placeholder, nil)
				}

				// Confirm that values are correct
				mcr.EXPECT().NewPrompt("Are these values correct?", true).
					Return(true, nil)
			},
			expectFS: func(mf *mocks.MockFSInteractor) {
				// Generate replacements (Reaplce with h.placeholder as defined in expectComponents)
				replacements := make([]files.ReplacementTuple, len(hostsTpl))
				for i, h := range hostsTpl {
					replacements[i].Find = h.find
					replacements[i].Replace = h.placeholder
				}

				mf.EXPECT().
					ReplaceAndCopy(
						"./nginx/nginx.conf",
						"./nginx.configured.conf",
						replacements,
					).Return(nil)
			},
			runCmd: func(c *exec.Cmd) error {
				return testutils.CheckCmdArgs(
					c,
					"ansible-playbook",
					[]string{
						"--extra-vars", `ansible_ssh_private_key_file='./private-key'`,
						"--extra-vars", "ansible_host_key_checking=false",
						"--extra-vars", `ansible_become_pass='password'`,
						"-i", configs.DEFAULT_HOSTS_FILE,
						"-u", "user-tester",
						"./playbooks/nginx.ansible.yaml",
					},
				)
			},
			expectSSH: func(ms *mocks.MockSSHConnection) {
				// -d domains are taken from global hostsTpl
				ms.EXPECT().
					ExecCommand(`echo 'password' | sudo -S certbot --nginx -d main.d8x.xyz,ws.d8x.xyz,history.d8x.xyz,referral.d8x.xyz,candles.d8x.xyz -n  --agree-tos -m email@for.certbot`).
					Return([]byte{}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctl := gomock.NewController(t)
			fakeCRW := mocks.NewMockD8XConfigReadWriter(ctl)

			if tt.expectCfgRW != nil {
				tt.expectCfgRW(fakeCRW)
			}

			ctl2 := gomock.NewController(t)
			fakeCR := mocks.NewMockComponentsRunner(ctl2)

			ctl3 := gomock.NewController(t)
			fakeHC := mocks.NewMockHostsFileInteractor(ctl3)

			ctl4 := gomock.NewController(t)
			fakeSSH := mocks.NewMockSSHConnection(ctl4)

			ctl5 := gomock.NewController(t)
			fakeFS := mocks.NewMockFSInteractor(ctl5)

			ctl6 := gomock.NewController(t)
			fakeEC := mocks.NewMockEmbedFileCopier(ctl6)

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
				PgCrtPath:              "./pg.crt",
				ConfigRWriter:          fakeCRW,
				SshKeyPath:             "./private-key",
				DefaultClusterUserName: "user-tester",
			}

			if tt.runCmd != nil {
				c.RunCmd = tt.runCmd
			}

			err := c.SwarmNginx(tt.ctx)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

		})
	}

}
