package options

const DEFAULT_MAX_RETRY_ATTEMPTS = 3
const DEFAULT_SLEEP = 5

// List of recurring transient errors encountered when calling terraform
// If any of these match, we'll retry the command
var RETRYABLE_ERRORS = []string{
	"(?s).*Failed to load state.*tcp.*timeout.*",
	"(?s).*Failed to load backend.*TLS handshake timeout.*",
	"(?s).*Creating metric alarm failed.*request to update this alarm is in progress.*",
	"(?s).*Error installing provider.*TLS handshake timeout.*",
	"(?s).*Error configuring the backend.*TLS handshake timeout.*",
}

// List of recurring transient errors encountered when calling terraform
// If one of these are encountered, we will re-run the init command
var ERRORS_REQUIRING_INIT = []string{}
