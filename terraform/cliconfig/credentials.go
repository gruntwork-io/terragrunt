package cliconfig

import (
	"os"
	"strings"

	svchost "github.com/hashicorp/terraform-svchost"
	svcauth "github.com/hashicorp/terraform-svchost/auth"
)

type CredentialsSource struct {
	// configured describes the credentials explicitly configured in the CLI config via "credentials" blocks.
	configured map[svchost.Hostname]string
}

func (s *CredentialsSource) ForHost(host svchost.Hostname) svcauth.HostCredentials {
	// The first order of precedence for credentials is a host-specific environment variable
	if envCreds := hostCredentialsFromEnv(host); envCreds != nil {
		return envCreds
	}

	// Then, any credentials block present in the CLI config
	if token, ok := s.configured[host]; ok {
		return svcauth.HostCredentialsToken(token)
	}

	return nil
}

// hostCredentialsFromEnv returns a token credential by searching for a hostname-specific environment variable. The host parameter is expected to be in the "comparison" form, for example, hostnames containing non-ASCII characters like "café.fr" should be expressed as "xn--caf-dma.fr". If the variable based on the hostname is not defined, nil is returned.
//
// Hyphen and period characters are allowed in environment variable names, but are not valid POSIX variable names. However, it's still possible to set variable names with these characters using utilities like env or docker. Variable names may have periods translated to underscores and hyphens translated to double underscores in the variable name. For the example "café.fr", you may use the variable names "TF_TOKEN_xn____caf__dma_fr", "TF_TOKEN_xn--caf-dma_fr", or "TF_TOKEN_xn--caf-dma.fr"
func hostCredentialsFromEnv(host svchost.Hostname) svcauth.HostCredentials {
	token, ok := collectCredentialsFromEnv()[host]
	if !ok {
		return nil
	}
	return svcauth.HostCredentialsToken(token)
}

func collectCredentialsFromEnv() map[svchost.Hostname]string {
	const prefix = "TF_TOKEN_"

	ret := make(map[svchost.Hostname]string)
	for _, ev := range os.Environ() {
		eqIdx := strings.Index(ev, "=")
		if eqIdx < 0 {
			continue
		}
		name := ev[:eqIdx]
		value := ev[eqIdx+1:]
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rawHost := name[len(prefix):]

		// We accept double underscores in place of hyphens because hyphens are not valid identifiers in most shells and are therefore hard to set.
		rawHost = strings.ReplaceAll(rawHost, "__", "-")

		// We accept underscores in place of dots because dots are not valid identifiers in most shells and are therefore hard to set.
		// Underscores are not valid in hostnames, so this is unambiguous for valid hostnames.
		rawHost = strings.ReplaceAll(rawHost, "_", ".")

		// Because environment variables are often set indirectly by OS libraries that might interfere with how they are encoded, we'll be tolerant of them being given either directly as UTF-8 IDNs or in Punycode form, normalizing to Punycode form here because that is what the OpenTofu credentials helper protocol will use in its requests.
		dispHost := svchost.ForDisplay(rawHost)
		hostname, err := svchost.ForComparison(dispHost)
		if err != nil {
			// Ignore invalid hostnames
			continue
		}

		ret[hostname] = value
	}

	return ret
}
