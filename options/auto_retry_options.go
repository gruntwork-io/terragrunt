package options

import "time"

const DefaultRetryMaxAttempts = 3
const DefaultRetrySleepInterval = 5 * time.Second

// DefaultRetryableErrors is a list of errors that are considered transient and
// should be retried.
//
// It's a list of recurring transient errors encountered when calling terraform
// If any of these match, we'll retry the command.
var DefaultRetryableErrors = []string{
	"(?s).*Failed to load state.*tcp.*timeout.*",
	"(?s).*Failed to load backend.*TLS handshake timeout.*",
	"(?s).*Creating metric alarm failed.*request to update this alarm is in progress.*",
	"(?s).*Error installing provider.*TLS handshake timeout.*",
	"(?s).*Error configuring the backend.*TLS handshake timeout.*",
	"(?s).*Error installing provider.*tcp.*timeout.*",
	"(?s).*Error installing provider.*tcp.*connection reset by peer.*",
	"NoSuchBucket: The specified bucket does not exist",
	"(?s).*Error creating SSM parameter: TooManyUpdates:.*",
	"(?s).*app.terraform.io.*: 429 Too Many Requests.*",
	"(?s).*ssh_exchange_identification.*Connection closed by remote host.*",
	"(?s).*Client\\.Timeout exceeded while awaiting headers.*",
	"(?s).*Could not download module.*The requested URL returned error: 429.*",
	"(?s).*net/http: TLS.*handshake timeout.*",
}
