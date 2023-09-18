package actions

import "testing"

// expecter holds mocked things for expect func
type expecter struct {
}

func TestAwsConfigurer(t *testing.T) {
	tests := []struct {
		name   string
		expect func(*expecter)
	}{
		{
			name: "enter access token - error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

		})
	}
}
