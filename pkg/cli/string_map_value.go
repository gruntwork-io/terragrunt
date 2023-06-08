package cli

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/pkg/maps"
)

// -- string map Value
type stringMapValue struct {
	val                *map[string]string
	hasBeenSet         bool
	listSep, keyValSep string
	splitter           SplitterFunc
}

func newStringMapValue(val map[string]string, p *map[string]string, listSep, keyValSep string, splitter SplitterFunc) *stringMapValue {
	*p = val

	return &stringMapValue{
		val:       p,
		splitter:  splitter,
		listSep:   listSep,
		keyValSep: keyValSep,
	}
}

func (val *stringMapValue) Set(str string) error {
	if !val.hasBeenSet {
		val.hasBeenSet = true
		*val.val = make(map[string]string)
	}

	parts := val.splitter(str, val.keyValSep)
	if len(parts) != 2 {
		err := fmt.Errorf("valid format: key%svalue", val.keyValSep)
		return errors.WithStackTrace(err)
	}

	(*val.val)[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	return nil
}

func (val *stringMapValue) Get() any { return map[string]string(*val.val) }

func (val *stringMapValue) String() string { return maps.Join(*val.val, val.listSep, val.keyValSep) }
