package cliconfig

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonHcl(t *testing.T) {
	jsonData := `{"name": "peter", "parties":["one", "two", ["three", "four"], {"five":"51", "six":61}]}`

	expectedHCL := `
  name = "peter"
  parties = [
    "one",
    "two",
    [
      "three",
      "four"
    ],
    {
      five = "51"
      six = 61
    }
  ]
`

	data := map[string]interface{}{}
	err := json.Unmarshal([]byte(jsonData), &data)
	require.NoError(t, err)

	hclBytes, err := Marshal(data)
	require.NoError(t, err)

	assert.Equal(t, string(hclBytes), expectedHCL)
}
