package exec

// Options holds the exec command's configuration.
type Options struct {
	// InDownloadDir determines whether the command should execute in the download directory
	// rather than the working directory.
	InDownloadDir bool
}

// NewOptions returns Options that run the command in the working directory.
func NewOptions() *Options {
	return &Options{}
}
