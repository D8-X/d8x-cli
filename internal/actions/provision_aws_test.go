package actions

import (
	"testing"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// expecter holds mocked things for expect func
type expecter struct {
	ConfigReadWriter *mocks.MockD8XConfigReadWriter
	Components       *mocks.MockComponentsRunner
}

func TestAWSServerConfigurer(t *testing.T) {
	tests := []struct {
		name    string
		expect  func(*expecter)
		wantErr string
	}{
		{
			name: "read d8x config - error",
			expect: func(e *expecter) {
				e.ConfigReadWriter.EXPECT().Read().Return(nil, assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "enter access token - error",
			expect: func(e *expecter) {
				cfg := configs.NewD8XConfig()

				e.ConfigReadWriter.EXPECT().Read().Return(cfg, nil)
				e.Components.EXPECT().
					NewInput(
						components.TextInputOptValue(""),
						components.TextInputOptPlaceholder("<AWS_ACCESS_KEY>"),
					).Return("", assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "enter access secret - error",
			expect: func(e *expecter) {
				cfg := configs.NewD8XConfig()

				e.ConfigReadWriter.EXPECT().Read().Return(cfg, nil)
				e.Components.EXPECT().
					NewInput(
						components.TextInputOptValue(""),
						components.TextInputOptPlaceholder("<AWS_ACCESS_KEY>"),
					).Return("aws-access-key", nil)
				e.Components.EXPECT().
					NewInput(
						components.TextInputOptValue(""),
						components.TextInputOptMasked(),
						components.TextInputOptPlaceholder("<AWS_SECRET_KEY>"),
					).Return("", assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "enter region - error",
			expect: func(e *expecter) {
				cfg := configs.NewD8XConfig()

				e.ConfigReadWriter.EXPECT().Read().Return(cfg, nil)
				e.Components.EXPECT().
					NewInput(
						components.TextInputOptValue(""),
						components.TextInputOptPlaceholder("<AWS_ACCESS_KEY>"),
					).Return("aws-access-key", nil)
				e.Components.EXPECT().
					NewInput(
						components.TextInputOptValue(""),
						components.TextInputOptMasked(),
						components.TextInputOptPlaceholder("<AWS_SECRET_KEY>"),
					).Return("aws-access-secret", nil)
				e.Components.EXPECT().
					NewInput(
						components.TextInputOptValue("eu-central-1"),
						components.TextInputOptPlaceholder("us-west-1"),
					).Return("", assert.AnError)
			},
			wantErr: assert.AnError.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctl := gomock.NewController(t)
			fakecfgRW := mocks.NewMockD8XConfigReadWriter(ctl)

			ctl1 := gomock.NewController(t)
			fakeTUI := mocks.NewMockComponentsRunner(ctl1)

			expect := &expecter{
				ConfigReadWriter: fakecfgRW,
				Components:       fakeTUI,
			}

			if tt.expect != nil {
				tt.expect(expect)
			}

			c := &Container{
				ConfigRWriter: expect.ConfigReadWriter,
				TUI:           fakeTUI,
			}

			_, err := c.awsServerConfigurer()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
