package cloner

import (
	"reflect"
	"strings"
)

const (
	fieldTagName = "clone"

	// fieldTagValueRequired forces to make deep copy of the field even if the field type is disallowed by the option.
	fieldTagValueRequired = "required"
	// fieldTagValueShadowCopy specifies that the dst field should be assigned the src field pointer instead of deep copying.
	fieldTagValueShadowCopy = "shadowcopy"
	// fieldTagValueSkip specifies that the dst field should have a null value, regardless of the src value.
	fieldTagValueSkip      = "skip"
	fieldTagValueSkipAlias = "-"
)

// Option represents an option to customize deep copied results.
type Option func(*Config)

type Config struct {
	shadowCopyTypes []reflect.Type
	skippingTypes   []reflect.Type

	shadowCopyInversePkgPrefixes []string

	tagPriorityOnce bool
}

type Cloner[T any] struct {
	*Config
}

func (cloner *Cloner[T]) Clone(src T) T {
	var dst T

	val := cloner.cloneValue(reflect.ValueOf(src))

	reflect.ValueOf(&dst).Elem().Set(val)

	return dst
}

func (cloner *Cloner[T]) getDstValue(src reflect.Value) (reflect.Value, bool) {
	var (
		srcType = src.Type()
		pkgPath = src.Type().PkgPath()
		dst     = src
		valid   = false
	)

	if cloner.tagPriorityOnce {
		cloner.tagPriorityOnce = false

		return dst, valid
	}

	if len(cloner.shadowCopyInversePkgPrefixes) != 0 {
		validInverse := false

		for _, pkgPrefix := range cloner.shadowCopyInversePkgPrefixes {
			if pkgPath == "" || strings.HasPrefix(pkgPath, pkgPrefix) {
				validInverse = true

				break
			}
		}

		valid = !validInverse
	}

	for i := range cloner.skippingTypes {
		if srcType == cloner.skippingTypes[i] {
			dst = reflect.Zero(srcType).Elem()
			valid = true
		}
	}

	for i := range cloner.shadowCopyTypes {
		if srcType == cloner.shadowCopyTypes[i] {
			valid = true
		}
	}

	return dst, valid
}

func (cloner *Cloner[T]) cloneValue(src reflect.Value) reflect.Value {
	if dst, ok := cloner.getDstValue(src); ok {
		return dst
	}

	if !src.IsValid() {
		return src
	}

	// Look up the corresponding clone function.
	switch src.Kind() {
	case reflect.Bool:
		return cloner.cloneBool(src)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cloner.cloneInt(src)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return cloner.cloneUint(src)
	case reflect.Float32, reflect.Float64:
		return cloner.cloneFloat(src)
	case reflect.String:
		return cloner.cloneString(src)
	case reflect.Slice:
		return cloner.cloneSlice(src)
	case reflect.Array:
		return cloner.cloneArray(src)
	case reflect.Map:
		return cloner.cloneMap(src)
	case reflect.Pointer, reflect.UnsafePointer:
		return cloner.clonePointer(src)
	case reflect.Struct:
		return cloner.cloneStruct(src)
	case reflect.Invalid, reflect.Uintptr, reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Func, reflect.Interface:
	}

	return src
}

func (cloner *Cloner[T]) cloneInt(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.SetInt(src.Int())

	return dst
}

func (cloner *Cloner[T]) cloneUint(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.SetUint(src.Uint())

	return dst
}

func (cloner *Cloner[T]) cloneFloat(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.SetFloat(src.Float())

	return dst
}

func (cloner *Cloner[T]) cloneBool(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.SetBool(src.Bool())

	return dst
}

func (cloner *Cloner[T]) cloneString(src reflect.Value) reflect.Value {
	if src, ok := src.Interface().(string); ok {
		return reflect.ValueOf(strings.Clone(src))
	}

	return src
}

func (cloner *Cloner[T]) cloneSlice(src reflect.Value) reflect.Value {
	size := src.Len()
	dst := reflect.MakeSlice(src.Type(), size, size)

	for i := range size {
		if val := cloner.cloneValue(src.Index(i)); val.IsValid() {
			dst.Index(i).Set(val)
		}
	}

	return dst
}

func (cloner *Cloner[T]) cloneArray(src reflect.Value) reflect.Value {
	size := src.Type().Len()
	dst := reflect.New(reflect.ArrayOf(size, src.Type().Elem())).Elem()

	for i := range size {
		if val := cloner.cloneValue(src.Index(i)); val.IsValid() {
			dst.Index(i).Set(val)
		}
	}

	return dst
}

func (cloner *Cloner[T]) cloneMap(src reflect.Value) reflect.Value {
	dst := reflect.MakeMapWithSize(src.Type(), src.Len())
	iter := src.MapRange()

	for iter.Next() {
		item := cloner.cloneValue(iter.Value())
		key := cloner.cloneValue(iter.Key())
		dst.SetMapIndex(key, item)
	}

	return dst
}

func (cloner *Cloner[T]) clonePointer(src reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type()).Elem()
	}

	dst := reflect.New(src.Type().Elem())

	if val := cloner.cloneValue(src.Elem()); val.IsValid() {
		dst.Elem().Set(val)
	}

	return dst
}

func (cloner *Cloner[T]) cloneStruct(src reflect.Value) reflect.Value {
	t := src.Type()
	dst := reflect.New(t)

	for i := range t.NumField() {
		srcTypeField := t.Field(i)
		srcField := src.Field(i)

		if !srcTypeField.IsExported() {
			continue
		}

		var val reflect.Value

		switch srcTypeField.Tag.Get(fieldTagName) {
		case fieldTagValueSkip, fieldTagValueSkipAlias:
			cloner.tagPriorityOnce = true
			val = reflect.Zero(srcField.Type()).Elem()
		case fieldTagValueShadowCopy:
			cloner.tagPriorityOnce = true
			val = srcField
		case fieldTagValueRequired:
			cloner.tagPriorityOnce = true
			fallthrough
		default:
			val = cloner.cloneValue(srcField)
		}

		if val.IsValid() {
			dst.Elem().Field(i).Set(val)
		}
	}

	return dst.Elem()
}
