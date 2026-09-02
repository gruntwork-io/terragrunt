// Package portal talks to the Gruntwork Developer Portal, the authorization
// server and catalog API that back `terragrunt login` and a portal-defined
// `terragrunt catalog`. It also owns the CLI's half of the login hand-off,
// where the user is shown a code and sent to the portal to approve it.
//
// Login follows the OAuth 2.0 Device Authorization Grant ([RFC 8628]). The CLI
// is a public client: it holds no client secret and registers no redirect URI,
// so the device code the portal issues is the only credential in the exchange,
// and [Secret] keeps it out of terminal output and log lines.
//
// The device-authorization request is built here rather than through
// golang.org/x/oauth2. Its Config.DeviceAuth sends a request carrying no
// context, so a cancelled login cannot abandon a call already in flight.
//
// [RFC 8628]: https://datatracker.ietf.org/doc/html/rfc8628
package portal

import (
	"math"
	"time"
)

const (
	// DefaultBaseURL addresses the production Gruntwork Developer Portal. A
	// preview or local deployment answers on its own host, so every URL in a
	// response is read from that response rather than assembled from this one.
	DefaultBaseURL = "https://app.gruntwork.io"

	// ClientID identifies the Terragrunt CLI to the portal's authorization
	// server, which matches it against a registration in the portal's source.
	// The string travels verbatim, so changing it here breaks every released
	// CLI until the portal's registration changes with it.
	ClientID = "terragrunt-cli"

	// ScopeCatalogRead requests read access to the org's portal-defined
	// catalog. The portal rejects any scope it does not recognize, and rejects
	// a request that names none at all.
	ScopeCatalogRead = "catalog:read"
)

// secretPlaceholder stands in for a credential wherever one would be rendered.
const secretPlaceholder = "[REDACTED]"

// Secret carries a credential that must never reach a terminal or a log file.
// Formatting one yields a placeholder, so the value it holds leaves only
// through [Secret.Reveal].
type Secret string

// String renders the placeholder instead of the credential.
func (s Secret) String() string {
	return secretPlaceholder
}

// GoString renders the placeholder instead of the credential, covering the %#v
// verb, which does not consult [Secret.String].
func (s Secret) GoString() string {
	return secretPlaceholder
}

// Reveal returns the credential, for the request that has to carry it.
func (s Secret) Reveal() string {
	return string(s)
}

// maxDurationSeconds is the largest whole-second count that fits in a
// [time.Duration].
const maxDurationSeconds = math.MaxInt64 / int64(time.Second)

// secondsToDuration reads a wait the portal reported in whole seconds. A count
// that overflows a [time.Duration] wraps to an unrelated wait, so it is
// rejected along with the counts that are zero or negative.
func secondsToDuration(n int64) (time.Duration, bool) {
	if n <= 0 || n > maxDurationSeconds {
		return 0, false
	}

	return time.Duration(n) * time.Second, true
}
