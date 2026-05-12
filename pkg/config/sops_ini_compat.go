package config

import (
	"bytes"
	"os"
	"strings"

	"github.com/getsops/sops/v3/decrypt"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

const literalEscapedNewline = `\n`

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
// Preserves the `[sops]` section header matching rules and normalizes CRLF to LF for
// deterministic output.
// Assumption: SOPS metadata values never contain the literal sequence `"""` after `\n` decode
// (current SOPS metadata is base64 / PGP armor / timestamps, none can hold triple-quote).
func rewriteINISopsMetadataNewlines(data []byte) []byte {
	hasEscapedNewlines := bytes.Contains(data, []byte(literalEscapedNewline))
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	if !hasEscapedNewlines {
		return data
	}

	lines := strings.Split(string(data), "\n")
	inSopsSection := false

	for i, line := range lines {
		trim := strings.TrimSpace(line)

		if name, ok := sopsIniSectionName(trim); ok {
			inSopsSection = name == "sops"
			continue
		}

		if !inSopsSection || !strings.Contains(line, literalEscapedNewline) {
			continue
		}

		k, v, ok := strings.Cut(line, "=")
		if !ok {
			// Malformed [sops] line with no `=`; leave it for the INI parser to surface.
			continue
		}

		key := strings.TrimRight(k, " \t")
		val := strings.ReplaceAll(strings.TrimLeft(v, " \t"), literalEscapedNewline, "\n")
		lines[i] = key + ` = """` + val + `"""`
	}

	return []byte(strings.Join(lines, "\n"))
}

func sopsIniSectionName(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") {
		return "", false
	}

	closeIdx := strings.Index(line, "]")
	if closeIdx <= 1 {
		return "", false
	}

	name := strings.TrimSpace(line[1:closeIdx])
	if name == "" {
		return "", false
	}

	return strings.ToLower(name), true
}
