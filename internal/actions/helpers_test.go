package actions

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateConfig(t *testing.T) {
	filename := "test.json"
	testJson := `{"some-item":[1,2,3], "another-item": {"a": "b"}, "string-val": "hello"}`
	if err := os.WriteFile(filename, []byte(testJson), 0666); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filename)

	// Test the UpdateConfig function
	err := UpdateConfig[map[string]any](
		filename,
		func(m *map[string]any) error {
			(*m)["some-item"] = []int{4, 5, 6}
			return nil
		},
	)

	assert.NoError(t, err)

	// Inspect the updated file
	updatedJson, err := os.ReadFile(filename)
	assert.NoError(t, err)

	// Mind the MarshalIndent arguments (we use 2 spaces)!
	expectJson := `{
  "another-item": {
    "a": "b"
  },
  "some-item": [
    4,
    5,
    6
  ],
  "string-val": "hello"
}`

	assert.Equal(t, expectJson, string(updatedJson))
}
