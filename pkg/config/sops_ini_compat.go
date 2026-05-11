package config

import (
	"bufio"
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
	var out bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	inSops := false
	for scanner.Scan() {
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			inSops = trim == "[sops]"
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		if !inSops || !strings.Contains(line, `\n`) {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		key := strings.TrimRight(line[:eq], " \t")
		val := strings.TrimLeft(line[eq+1:], " \t")
		val = strings.ReplaceAll(val, `\n`, "\n")
		out.WriteString(key)
		out.WriteString(` = """`)
		out.WriteString(val)
		out.WriteString(`"""`)
		out.WriteByte('\n')
	}
	return out.Bytes()
}
