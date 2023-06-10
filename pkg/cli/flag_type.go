package cli

import (
	"flag"
	"fmt"
	"strconv"

	"github.com/gruntwork-io/terragrunt/errors"
)

type FlagType[T any] interface {
	flag.Getter
	Init(dest *T, negative bool) FlagType[T]
}

// -- generic Type
type flagType[T comparable] struct {
	dest     *T
	negative bool
}

func (val *flagType[T]) Init(dest *T, negative bool) FlagType[T] {
	return &flagType[T]{dest: dest, negative: negative}
}

func (val *flagType[T]) Set(str string) error {
	switch dest := (interface{})(val.dest).(type) {
	case *string:
		*dest = str

	case *bool:
		v, err := strconv.ParseBool(str)
		if err != nil {
			return errors.Errorf("error parse: %w", err)
		}
		if val.negative {
			v = !v
		}
		*dest = v

	case *int:
		v, err := strconv.ParseInt(str, 0, strconv.IntSize)
		if err != nil {
			return errors.Errorf("error parse: %w", err)
		}
		*dest = int(v)

	case *int64:
		v, err := strconv.ParseInt(str, 0, 64)
		if err != nil {
			return errors.Errorf("error parse: %w", err)
		}
		*dest = v
	}

	return nil
}

func (val *flagType[T]) Get() any { return *val.dest }

func (val *flagType[T]) String() string { return fmt.Sprintf("%v", *val.dest) }
