package legacygzip_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/legacygzip"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	tffuncs "github.com/hashicorp/terraform/lang/funcs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestEncodeMatchesV113(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		input        string
		expected     string
		expectedHash string
	}{
		{
			name:     "empty",
			expected: "H4sIAAAAAAAA/wAAAP//AQAA//8AAAAAAAAAAA==",
		},
		{
			name:     "short",
			input:    "test",
			expected: "H4sIAAAAAAAA/ypJLS4BAAAA//8BAAD//wx+f9gEAAAA",
		},
		{
			name:     "sentence",
			input:    "some text",
			expected: "H4sIAAAAAAAA/yrOz01VKEmtKAEAAAD//wEAAP//ur26TwkAAAA=",
		},
		{
			name:  "cloud init",
			input: "#!/bin/bash\nset -euo pipefail\necho hello > /tmp/hello\n",
			expected: "H4sIAAAAAAAA/1JW1E/KzNNPSizO4CpOLVHQTS3NVyjILEhNS8zM4UpNzshXyEjNyclXsFPQL8kt0AdzuAAAAAD//wEAAP//" +
				"I7vvhTYAAAA=",
		},
		{
			name:  "repeated input",
			input: strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789\n", 200),
			expected: "H4sIAAAAAAAA/+zKtQHCAAAAsJ1rcDkHqLvb9X2jQ/Z8f/8gjOIkzfKirOqm7fphnOZl3c6X6+3+eL7enxMEQRAEQRAEQRAEQRAEQRAEQRAEQdCB" +
				"0A4AAP//AQAA//8JRK436BwAAA==",
		},
		{
			name:         "unicode",
			input:        "Hello, \u4e16\u754c \U0001f30d",
			expectedHash: "6ad54f9401db6fb2db87e3055dd3bdc09c595e75439f58a65e1a2b9cdb2a41a2",
		},
		{
			name:         "below history window",
			input:        strings.Repeat("abcdefghijklmnopqrstuvwxyz012345", 1024)[:32767],
			expectedHash: "071e2b7887a20474a41fe2a312d22087dd56c65eb2eb8f296934462fc32c5903",
		},
		{
			name:         "at history window",
			input:        strings.Repeat("abcdefghijklmnopqrstuvwxyz012345", 1024),
			expectedHash: "d1d6a30d6bce39d6dc97b2781785a2a4cb9c2c7fd34c53ea5b3c5b835490abc6",
		},
		{
			name:         "above history window",
			input:        strings.Repeat("abcdefghijklmnopqrstuvwxyz012345", 1025)[:32769],
			expectedHash: "11b5c89b161d938b68b238b0bb0a023380d0ca6cec8a85c64305374149e927dd",
		},
		{
			name:         "large incompressible input",
			input:        pseudoRandomASCII(65536),
			expectedHash: "d8affe2115a42410a460461494ae6c899eede3581eaac40b37eb88630c8f5800",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual, err := legacygzip.Encode(tc.input)
			require.NoError(t, err)

			if tc.expected != "" {
				assert.Equal(t, tc.expected, actual)
			} else {
				hash := sha256.Sum256([]byte(actual))
				assert.Equal(t, tc.expectedHash, hex.EncodeToString(hash[:]))
			}

			compressed, err := base64.StdEncoding.DecodeString(actual)
			require.NoError(t, err)

			reader, err := gzip.NewReader(bytes.NewReader(compressed))
			require.NoError(t, err)

			decoded, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, reader.Close())
			assert.Equal(t, tc.input, string(decoded))
		})
	}
}

func TestBase64GzipStrictControl(t *testing.T) {
	t.Parallel()

	const (
		input          = "some text"
		legacyExpected = "H4sIAAAAAAAA/yrOz01VKEmtKAEAAAD//wEAAP//ur26TwkAAAA="
	)

	terraformExpected, err := tffuncs.Base64Gzip(cty.StringVal(input))
	require.NoError(t, err)

	assert.Equal(t, legacyExpected, parseBase64Gzip(t, input, false), "legacy bytes by default")
	assert.Equal(t, terraformExpected.AsString(), parseBase64Gzip(t, input, true), "current encoder with the control")
}

func parseBase64Gzip(t *testing.T, input string, enableControl bool) any {
	t.Helper()

	l := logger.CreateLogger()
	cfgPath := filepath.Join(t.TempDir(), config.DefaultTerragruntConfigPath)

	opts, err := options.NewTerragruntOptionsForTest(cfgPath)
	require.NoError(t, err)

	if enableControl {
		require.NoError(t, opts.StrictControls.EnableControl(controls.LegacyBase64Gzip))
	}

	ctx, pctx := configbridge.NewParsingContext(t.Context(), l, venvtest.New(), opts)

	cfg, err := config.ParseConfigString(
		ctx,
		pctx,
		l,
		cfgPath,
		fmt.Sprintf("inputs = { test = base64gzip(%s) }", strconv.Quote(input)),
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	return cfg.Inputs["test"]
}

func pseudoRandomASCII(size int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}"

	data := make([]byte, size)
	state := uint32(0x6d2b79f5)

	for index := range data {
		state = state*1664525 + 1013904223
		data[index] = alphabet[int(state>>24)%len(alphabet)]
	}

	return string(data)
}
