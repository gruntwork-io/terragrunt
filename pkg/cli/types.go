package cli

import (
	"flag"
	"strconv"

	"github.com/gruntwork-io/terragrunt/errors"
)

type Type[T any] interface {
	flag.Getter
	Init(p *T) Type[T]
}

// -- bool Type
type boolType bool

func (val *boolType) Init(p *bool) Type[bool] {
	return (*boolType)(p)
}

func (b *boolType) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return errors.Errorf("error parse: %w", err)
	}
	*b = boolType(v)
	return nil
}

func (b *boolType) Get() any { return bool(*b) }

func (b *boolType) String() string { return strconv.FormatBool(bool(*b)) }

// -- string Type
type stringType string

func (val *stringType) Init(p *string) Type[string] {
	return (*stringType)(p)
}

func (val *stringType) Set(str string) error {
	*val = stringType(str)
	return nil
}

func (val *stringType) Get() any { return string(*val) }

func (val *stringType) String() string { return string(*val) }

// -- int Type
type intType int

func (val *intType) Init(p *int) Type[int] {
	return (*intType)(p)
}

func (val *intType) Set(str string) error {
	v, err := strconv.ParseInt(str, 0, strconv.IntSize)
	if err != nil {
		return errors.Errorf("error parse: %w", err)
	}
	*val = intType(v)
	return nil
}

func (val *intType) Get() any { return int(*val) }

func (val *intType) String() string { return strconv.Itoa(int(*val)) }

// -- int64 Type
type int64Type int64

func (val *int64Type) Init(p *int64) Type[int64] {
	return (*int64Type)(p)
}

func (i *int64Type) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		return errors.Errorf("error parse: %w", err)
	}
	*i = int64Type(v)
	return nil
}

func (i *int64Type) Get() any { return int64(*i) }

func (i *int64Type) String() string { return strconv.FormatInt(int64(*i), 10) }
