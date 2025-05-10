package cli

// List of sources. Order matters. Higher sources may overwrite flag values assigned by a lower source.
const (
	FlagValueSourceConfig FlagValueSourceType = iota
	FlagValueSourceEnvVar
	FlagValueSourceArg
)

type FlagValueSourceType byte

func (source FlagValueSourceType) String() string {
	switch source {
	case FlagValueSourceConfig:
		return "cli-config"
	case FlagValueSourceEnvVar:
		return "evn-var"
	case FlagValueSourceArg:
		return "cli-argument"
	}

	return "undefined source"
}
