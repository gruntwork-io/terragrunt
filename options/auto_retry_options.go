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
	"(?s).*NoSuchBucket: The specified bucket does not exist.*",
	"(?s).*Error creating SSM parameter: TooManyUpdates:.*",
	"(?s).*app.terraform.io.*: 429 Too Many Requests.*",
	"(?s).*Get.*dial tcp.*i/o timeout.*",
	"(?s).*Get.*dial tcp.*connect: connection refused.*",
	"(?s).*Get.*remote error: tls: internal error.*",
	"(?s).*Post.*net/http: TLS handshake timeout.*",
	"(?s).*Error accessing remote module registry.*",
	"(?s).*Registry service unreachable.*",
}

// List of recurring transient errors encountered when calling Terraform.
// If one of these are encountered, we will re-run the init command.
var ERRORS_REQUIRING_INIT = []string{
	"(?s).*Please run \"terraform init\".*",
	"(?s).*Module version requirements have changed.*",
	"(?s).*Could not satisfy plugin requirements.*",
	"(?s).*provider.*new or changed plugin executable.*",
	"(?s).*Error installing provider.*error fetching checksums.*",
}
