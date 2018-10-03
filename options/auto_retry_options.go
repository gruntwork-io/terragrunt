package options

const DEFAULT_MAX_RETRY_ATTEMPTS = 3
const DEFAULT_SLEEP = 5

var RETRYABLE_ERRORS = []string{
	".*Failed to load state.*tcp.*timeout.*",
	".*Failed to load backend.*TLS handshake timeout.*",
	".*Creating metric alarm failed.*request to update this alarm is in progress.*",
	".*Error installing provider.*TLS handshake timeout.*",
}

var ERRORS_REQUIRING_INIT = []string{
	".*Failed to load state.*tcp.*timeout.*",
	".*Failed to load backend.*TLS handshake timeout.*",
	".*Creating metric alarm failed.*request to update this alarm is in progress.*",
	".*Error installing provider.*TLS handshake timeout.*",
}
