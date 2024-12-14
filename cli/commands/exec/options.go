package exec

type Options struct {
	InDownloadDir bool
}

func NewOptions() *Options {
	return &Options{}
}
