package config

import (
	"bytes"
	"os"
	"strings"

	"github.com/getsops/sops/v3/decrypt"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// decrypt.File wrapper restoring DecodeNewLines for INI files dropped in sops v3.13's
// stores/ini/store.go refactor (https://github.com/getsops/sops/blob/v3.13.0/stores/ini/store.go).
func sopsDecryptFileWithINICompat(path, format string) ([]byte, error) {
	if format != "ini" {
		return decrypt.File(path, format)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Errorf("reading sops file %s: %w", path, err)
	}

	return decrypt.Data(rewriteINISopsMetadataNewlines(data), format)
}

// Rewrites [sops] section values containing literal `\n` as gopkg.in/ini.v1 triple-quoted
// multi-line values (the syntax that section accepts for multi-line content).
// Assumption: SOPS metadata values never contain the literal sequence `"""` after `\n` decode
// (current SOPS metadata is base64 / PGP armor / timestamps, none can hold triple-quote).
func rewriteINISopsMetadataNewlines(data []byte) []byte {
	if !bytes.Contains(data, []byte(`\n`)) {
		return data
	}

	lines := strings.Split(string(data), "\n")
	inSops := false

	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		lines[i] = line
		trim := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]"):
			inSops = trim == "[sops]"
		case inSops && strings.Contains(line, `\n`):
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				// Malformed [sops] line with no `=`; leave it for the INI parser to surface.
				continue
			}

			key := strings.TrimRight(k, " \t")
			val := strings.ReplaceAll(strings.TrimLeft(v, " \t"), `\n`, "\n")
			lines[i] = key + ` = """` + val + `"""`
		}
	}

	return []byte(strings.Join(lines, "\n"))
}
