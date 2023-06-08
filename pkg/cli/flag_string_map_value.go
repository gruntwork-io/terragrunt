package cli

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/pkg/maps"
)

// -- string map Value
type stringMapValue struct {
	value              *map[string]string
	hasBeenSet         bool
	listSep, keyValSep string
	splitter           SplitterFunc
}

func newStringMapValue(val map[string]string, p *map[string]string, listSep, keyValSep string, splitter SplitterFunc) *stringMapValue {
	*p = val

	return &stringMapValue{
		value:     p,
		splitter:  splitter,
		listSep:   listSep,
		keyValSep: keyValSep,
	}
}

func (val *stringMapValue) Set(str string) error {
	if !val.hasBeenSet {
		val.hasBeenSet = true
		*val.value = make(map[string]string)
	}

	parts := val.splitter(str, val.keyValSep)
	if len(parts) != 2 {
		return errors.Errorf("valid format: key%svalue", val.keyValSep)
	}

	(*val.value)[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	return nil
}

func (val *stringMapValue) Get() any { return map[string]string(*val.value) }

func (val *stringMapValue) String() string { return maps.Join(*val.value, val.listSep, val.keyValSep) }
