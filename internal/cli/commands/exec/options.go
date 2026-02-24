package exec

type Options struct {
	// InDownloadDir determines whether the command should execute in the download directory
	// rather than the working directory.
	InDownloadDir bool
}

func NewOptions() *Options {
	return &Options{}
}
