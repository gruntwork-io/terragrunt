package preset

import (
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
)

const (
	ArgOptRequired ArgOpt = iota
	ArgOptUpper
	ArgOptLower
	ArgOptTitle
)

var (
	// AllArgOpts exposes all arg opts
	AllArgOpts = ArgOpts{
		ArgOptRequired,
		ArgOptUpper,
		ArgOptLower,
		ArgOptTitle,
	}

	argOptNames = map[ArgOpt]string{
		ArgOptRequired: "required",
		ArgOptUpper:    "upper",
		ArgOptLower:    "lower",
		ArgOptTitle:    "title",
	}
)

type ArgOpts []ArgOpt

func (opts ArgOpts) String() string {
	return strings.Join(opts.Names(), ", ")
}

func (opts ArgOpts) Names() []string {
	strs := make([]string, len(opts))

	for i, opt := range opts {
		strs[i] = opt.String()
	}

	return strs
}

// ParseArgOpt takes a string and returns the arg opt constant.
func ParseArgOpts(names []string) (ArgOpts, error) {
	var opts ArgOpts

	for _, name := range names {
		if name == "" {
			continue
		}
		opt, err := ParseArgOpt(name)
		if err != nil {
			return nil, err
		}
		opts = append(opts, opt)
	}

	return opts, nil
}

type ArgOpt byte

// ParseArgOpt takes a string and returns the arg opt constant.
func ParseArgOpt(str string) (ArgOpt, error) {
	for opt, name := range argOptNames {
		if strings.EqualFold(name, str) {
			return opt, nil
		}
	}

	return ArgOpt(0), errors.Errorf("invalid argument opting %q, supported argument options: %s", str, AllArgOpts)
}

// String implements fmt.Stringer.
func (opt ArgOpt) String() string {
	if name, ok := argOptNames[opt]; ok {
		return name
	}

	return ""
}
