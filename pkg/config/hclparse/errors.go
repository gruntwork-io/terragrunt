package hclparse

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/hcl/v2"
)

type PanicWhileParsingConfigError struct {
	RecoveredValue any
	ConfigFile     string
}

func (err PanicWhileParsingConfigError) Error() string {
	return fmt.Sprintf(
		"Recovering panic while parsing '%s'. Got error of type '%v': %v",
		err.ConfigFile,
		reflect.TypeOf(err.RecoveredValue),
		err.RecoveredValue,
	)
}

// DuplicateExpansionBlockError is returned when a block declares more than one
// expansion block.
type DuplicateExpansionBlockError struct {
	Subject   *hcl.Range
	BlockType string
}

func (err DuplicateExpansionBlockError) Error() string {
	return fmt.Sprintf(
		"%s: the %s block declares more than one expansion block; a block may declare at most one",
		err.Subject,
		err.BlockType,
	)
}

// ConflictingMetaArgsError is returned when an expansion block sets both for_each
// and count.
type ConflictingMetaArgsError struct {
	Subject *hcl.Range
}

func (err ConflictingMetaArgsError) Error() string {
	return fmt.Sprintf(
		"%s: the expansion block sets both %s and %s; set exactly one",
		err.Subject,
		forEachAttrName,
		countAttrName,
	)
}

// MissingMetaArgError is returned when an expansion block sets neither for_each nor count.
type MissingMetaArgError struct {
	Subject *hcl.Range
}

func (err MissingMetaArgError) Error() string {
	return fmt.Sprintf(
		"%s: the expansion block sets neither %s nor %s; set exactly one",
		err.Subject,
		forEachAttrName,
		countAttrName,
	)
}

// InvalidCountError is returned when count does not evaluate to a whole number.
type InvalidCountError struct {
	Err     error
	Subject *hcl.Range
}

func (err InvalidCountError) Error() string {
	return fmt.Sprintf("%s: %s must be a whole number: %v", err.Subject, countAttrName, err.Err)
}

func (err InvalidCountError) Unwrap() error {
	return err.Err
}

// NegativeCountError is returned when count evaluates to a negative number.
type NegativeCountError struct {
	Subject *hcl.Range
	Count   int
}

func (err NegativeCountError) Error() string {
	return fmt.Sprintf(
		"%s: %s is %d; it must not be negative",
		err.Subject,
		countAttrName,
		err.Count,
	)
}

// UnknownExpansionValueError is returned when a meta-arg evaluates to an unknown
// value, which cannot be iterated.
type UnknownExpansionValueError struct {
	Subject *hcl.Range
	Attr    string
}

func (err UnknownExpansionValueError) Error() string {
	return fmt.Sprintf(
		"%s: %s is not known at parse time; it must resolve to a concrete value",
		err.Subject,
		err.Attr,
	)
}

// NullExpansionValueError is returned when a meta-arg evaluates to null.
type NullExpansionValueError struct {
	Subject *hcl.Range
	Attr    string
}

func (err NullExpansionValueError) Error() string {
	return fmt.Sprintf("%s: %s is null; it must resolve to a concrete value", err.Subject, err.Attr)
}

// UnsupportedForEachTypeError is returned when for_each evaluates to something other
// than a set, map, or object.
type UnsupportedForEachTypeError struct {
	Subject *hcl.Range
	Type    string
}

func (err UnsupportedForEachTypeError) Error() string {
	return fmt.Sprintf(
		"%s: %s must be a set or a map, but got %s",
		err.Subject,
		forEachAttrName,
		err.Type,
	)
}

// UnsupportedForEachKeyTypeError is returned when a for_each element key is neither
// a string nor a number.
type UnsupportedForEachKeyTypeError struct {
	Subject *hcl.Range
	Type    string
}

func (err UnsupportedForEachKeyTypeError) Error() string {
	return fmt.Sprintf(
		"%s: %s keys must be strings or numbers, but got %s",
		err.Subject,
		forEachAttrName,
		err.Type,
	)
}
