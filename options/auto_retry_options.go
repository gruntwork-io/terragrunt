package options

import "time"

const DEFAULT_MAX_RETRY_ATTEMPTS = 3
const DEFAULT_SLEEP = 5 * time.Second

// List of recurring transient errors encountered when calling terraform
// If any of these match, we'll retry the command
var RETRYABLE_ERRORS = []string{
	"(?s).*Failed to load state.*tcp.*timeout.*",
	"(?s).*Failed to load backend.*TLS handshake timeout.*",
	"(?s).*Creating metric alarm failed.*request to update this alarm is in progress.*",
	"(?s).*Error installing provider.*TLS handshake timeout.*",
	"(?s).*Error configuring the backend.*TLS handshake timeout.*",
	"(?s).*Error installing provider.*tcp.*timeout.*",
	"(?s).*Error installing provider.*tcp.*connection reset by peer.*",
	"NoSuchBucket: The specified bucket does not exist",
	"(?s).*Error creating SSM parameter: TooManyUpdates:.*",
}
