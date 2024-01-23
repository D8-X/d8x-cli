package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTBPS(t *testing.T) {

	tests := []struct {
		name         string
		input        string
		expectOutput string
		expectErr    string
	}{
		{
			name:         "ok 1",
			input:        "12.453",
			expectOutput: "12453",
		},
		{
			name:      "not ok 1",
			input:     "12.4953",
			expectErr: "invalid percent value, max 3 digits in the decimal fraction: 12.4953",
		},
		{
			name:         "ok 2",
			input:        "0.002",
			expectOutput: "2",
		},
		{
			name:      "not ok 2",
			input:     "0,002",
			expectErr: "invalid percent value, use dot '.' instead of comma ',': 0,002",
		},
		{
			name:      "not ok 3",
			input:     "0.00.2",
			expectErr: "invalid percent value: 0.00.2",
		},
		{
			name:         "ok 3",
			input:        "100",
			expectOutput: "100000",
			// expectErr: "invalid percent value, use dot '.' instead of comma ',': 0,002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := convertPercentToTBPS(tt.input)

			assert.Equal(t, tt.expectOutput, output)
			if err != nil {
				assert.EqualError(t, err, tt.expectErr)
			}
		})
	}
}

func TestUpdateBrokerChainConfigAllowedExecutors(t *testing.T) {
	chainConfig := &[]map[string]any{
		// Duplicates should be removed automatically
		{"chainId": float64(4001), "allowedExecutors": []any{"old-executor-address", "old-executor-address", "old-executor-address", "old-executor-address-123"}},
		{"chainId": float64(4401), "allowedExecutors": []any{"old-executor-address-4401"}},
	}
	err := UpdateBrokerChainConfigAllowedExecutors("new-executor-address")(chainConfig)
	assert.NoError(t, err)

	assert.Equal(t, []string{"new-executor-address", "old-executor-address", "old-executor-address-123"}, (*chainConfig)[0]["allowedExecutors"])
	assert.Equal(t, []string{"new-executor-address", "old-executor-address-4401"}, (*chainConfig)[1]["allowedExecutors"])
}
