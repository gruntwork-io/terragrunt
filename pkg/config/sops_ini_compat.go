package config

import (
	"bytes"
	"os"
	"strings"

	"github.com/getsops/sops/v3/decrypt"
)

// decrypt.File wrapper restoring DecodeNewLines for INI files (sops v3.13 dropped it).
func sopsDecryptFileWithINICompat(path, format string) ([]byte, error) {
	if format != "ini" {
		return decrypt.File(path, format)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return decrypt.Data(rewriteINISopsMetadataNewlines(data), format)
}

// Rewrites [sops] section values containing literal `\n` as triple-quoted multi-line.
func rewriteINISopsMetadataNewlines(data []byte) []byte {
	if !bytes.Contains(data, []byte(`\n`)) {
		return data
	}

	lines := strings.Split(string(data), "\n")
	inSops := false

	for i, line := range lines {
		trim := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]"):
			inSops = trim == "[sops]"
		case inSops && strings.Contains(line, `\n`):
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}

			key := strings.TrimRight(k, " \t")
			val := strings.ReplaceAll(strings.TrimLeft(v, " \t"), `\n`, "\n")
			lines[i] = key + ` = """` + val + `"""`
		}
	}

	return []byte(strings.Join(lines, "\n"))
}
