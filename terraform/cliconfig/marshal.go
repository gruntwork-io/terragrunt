// Copyright (c) 2022 Peter Bi

package cliconfig

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/genelet/determined/utils"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// Marshal marshals object into HCL string
func Marshal(current interface{}) ([]byte, error) {
	if current == nil {
		return nil, nil
	}
	return MarshalLevel(current, 0)
}

func MarshalLevel(current interface{}, level int) ([]byte, error) {
	rv := reflect.ValueOf(current)
	if rv.IsValid() && rv.IsZero() {
		return nil, nil
	}

	switch rv.Kind() {
	case reflect.Pointer, reflect.Struct:
		return marshal(current, level)
	default:
	}

	return utils.Encoding(current, level)
}

func marshal(current interface{}, level int, keyname ...string) ([]byte, error) {
	if current == nil {
		return nil, nil
	}
	leading := strings.Repeat("  ", level+1)
	lessLeading := strings.Repeat("  ", level)

	t := reflect.TypeOf(current)
	oriValue := reflect.ValueOf(current)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
		oriValue = oriValue.Elem()
	}

	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		if oriValue.IsValid() {
			return []byte(fmt.Sprintf("= %v", oriValue.Interface())), nil
		}
		return nil, nil
	case reflect.String:
		if oriValue.IsValid() {
			return []byte(" = " + oriValue.String()), nil
		}
		return nil, nil
	case reflect.Pointer:
		return marshal(oriValue.Elem().Interface(), level, keyname...)
	default:
	}

	newFields, err := getFields(t, oriValue)
	if err != nil {
		return nil, err
	}

	var plains []reflect.StructField
	for _, mField := range newFields {
		if !mField.out {
			plains = append(plains, mField.field)
		}
	}
	newType := reflect.StructOf(plains)
	tmp := reflect.New(newType).Elem()
	var outliers []*marshalOut
	var labels []string

	k := 0
	for _, mField := range newFields {
		field := mField.field
		oriField := mField.value
		if mField.out {
			outlier, err := getOutlier(field, oriField, level)
			if err != nil {
				return nil, err
			}
			outliers = append(outliers, outlier...)
		} else {
			fieldTag := field.Tag
			hcl := tag2(fieldTag)
			if hcl[1] == "label" {
				label := oriField.Interface().(string)
				if keyname == nil || keyname[0] != label {
					labels = append(labels, label)
				}
				k++
				continue
			}
			tmp.Field(k).Set(oriField)
			k++
		}
	}

	f := hclwrite.NewEmptyFile()
	gohcl.EncodeIntoBody(tmp.Addr().Interface(), f.Body())
	bs := f.Bytes()

	str := string(bs)
	str = leading + strings.ReplaceAll(str, "\n", "\n"+leading)

	var lines []string
	for _, item := range outliers {
		line := string(item.b0) + " "
		if item.encode {
			line += " = "
		}
		if item.b1 != nil {
			line += `"` + strings.Join(item.b1, `" "`) + `" `
		}
		line += string(item.b2)
		lines = append(lines, line)
	}
	if len(lines) > 0 {
		str += strings.Join(lines, "\n"+leading)
	}

	str = strings.TrimRight(str, " \t\n\r")
	if level > 0 { // not root
		str = fmt.Sprintf("{\n%s\n%s}", str, lessLeading)
		if labels != nil {
			str = "\"" + strings.Join(labels, "\" \"") + "\" " + str
		}
	}

	return []byte(str), nil
}

type marshalField struct {
	field reflect.StructField
	value reflect.Value
	out   bool
}

func getFields(t reflect.Type, oriValue reflect.Value) ([]*marshalField, error) {
	var newFields []*marshalField
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		typ := field.Type
		if !unicode.IsUpper([]rune(field.Name)[0]) {
			continue
		}
		if !oriValue.IsValid() || oriValue.IsZero() {
			continue
		}
		oriField := oriValue.Field(i)
		two := tag2(field.Tag)
		tcontent := two[0]
		if tcontent == `-` || (len(tcontent) >= 2 && tcontent[len(tcontent)-2:] == `,-`) {
			continue
		}

		if field.Anonymous && tcontent == "" {
			switch typ.Kind() {
			case reflect.Ptr:
				mfs, err := getFields(typ.Elem(), oriField.Elem())
				if err != nil {
					return nil, err
				}
				newFields = append(newFields, mfs...)
			case reflect.Struct:
				mfs, err := getFields(typ, oriField)
				if err != nil {
					return nil, err
				}
				newFields = append(newFields, mfs...)
			default:
			}
			continue
		}

		pass := false
		switch typ.Kind() {
		case reflect.Interface, reflect.Pointer, reflect.Struct:
			pass = true
		case reflect.Slice:
			if oriField.Len() == 0 {
				continue
			}
			switch oriField.Index(0).Kind() {
			case reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.Struct:
				pass = true
			default:
			}
		case reflect.Map:
			if oriField.Len() == 0 {
				continue
			}
			switch oriField.MapIndex(oriField.MapKeys()[0]).Kind() {
			case reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.Struct:
				pass = true
			default:
			}
		default:
			if oriField.IsValid() && oriField.IsZero() {
				continue
			}
		}
		if tcontent == "" {
			if pass {
				field.Tag = reflect.StructTag(fmt.Sprintf(`hcl:"%s,block"`, strings.ToLower(field.Name)))
			} else {
				field.Tag = reflect.StructTag(fmt.Sprintf(`hcl:"%s,optional"`, strings.ToLower(field.Name)))
			}
		}
		newFields = append(newFields, &marshalField{field, oriField, pass})
	}
	return newFields, nil
}

type marshalOut struct {
	b0     []byte
	b1     []string
	b2     []byte
	encode bool
}

func getOutlier(field reflect.StructField, oriField reflect.Value, level int) ([]*marshalOut, error) {
	var empty []*marshalOut
	fieldTag := field.Tag
	typ := field.Type
	newlevel := level + 1

	switch typ.Kind() {
	case reflect.Interface, reflect.Pointer:
		newCurrent := oriField.Interface()
		bs, err := MarshalLevel(newCurrent, newlevel)
		if err != nil {
			return nil, err
		}
		if bs == nil {
			return nil, nil
		}
		empty = append(empty, &marshalOut{hcltag(fieldTag), nil, bs, false})
	case reflect.Struct:
		var newCurrent interface{}
		if oriField.CanAddr() {
			newCurrent = oriField.Addr().Interface()
		} else {
			newCurrent = oriField.Interface()
		}

		bs, err := MarshalLevel(newCurrent, newlevel)
		if err != nil {
			return nil, err
		}
		if bs == nil {
			return nil, nil
		}
		empty = append(empty, &marshalOut{hcltag(fieldTag), nil, bs, false})
	case reflect.Slice:
		n := oriField.Len()
		if n < 1 {
			return nil, nil
		}
		first := oriField.Index(0)
		var isLoop bool
		switch first.Kind() {
		case reflect.Pointer, reflect.Struct:
			isLoop = true
		case reflect.Interface:
			if first.Elem().Kind() == reflect.Pointer || first.Elem().Kind() == reflect.Struct {
				isLoop = true
			}
		default:
		}

		if isLoop {
			for i := 0; i < n; i++ {
				item := oriField.Index(i)
				bs, err := MarshalLevel(item.Interface(), newlevel)
				if err != nil {
					return nil, err
				}
				if bs == nil {
					continue
				}
				empty = append(empty, &marshalOut{hcltag(fieldTag), nil, bs, false})
			}
		} else {
			bs, err := utils.Encoding(oriField.Interface(), newlevel)
			if err != nil {
				return nil, err
			}
			if bs == nil {
				return nil, nil
			}
			empty = append(empty, &marshalOut{hcltag(fieldTag), nil, bs, true})
		}
	case reflect.Map:
		n := oriField.Len()
		if n < 1 {
			return nil, nil
		}
		first := oriField.MapIndex(oriField.MapKeys()[0])
		var isLoop bool
		switch first.Kind() {
		case reflect.Pointer, reflect.Struct:
			isLoop = true
		case reflect.Interface:
			if first.Elem().Kind() == reflect.Pointer || first.Elem().Kind() == reflect.Struct {
				isLoop = true
			}
		default:
		}

		if isLoop {
			iter := oriField.MapRange()
			for iter.Next() {
				k := iter.Key()
				var arr []string
				switch k.Kind() {
				case reflect.Array, reflect.Slice:
					for i := 0; i < k.Len(); i++ {
						item := k.Index(i)
						if !item.IsZero() {
							arr = append(arr, item.String())
						}
					}
				default:
					arr = append(arr, k.String())
				}

				v := iter.Value()
				var bs []byte
				var err error
				bs, err = marshal(v.Interface(), newlevel, arr...)
				if err != nil {
					return empty, err
				}
				if bs == nil {
					continue
				}
				empty = append(empty, &marshalOut{hcltag(fieldTag), arr, bs, false})
			}
		} else {
			bs, err := utils.Encoding(oriField.Interface(), newlevel)
			if err != nil {
				return nil, err
			}
			if bs == nil {
				return nil, nil
			}
			empty = append(empty, &marshalOut{hcltag(fieldTag), nil, bs, true})
		}
	default:
	}
	return empty, nil
}

func tag2(old reflect.StructTag) [2]string {
	for _, tag := range strings.Fields(string(old)) {
		if len(tag) >= 5 && strings.ToLower(tag[:5]) == "hcl:\"" {
			tag = tag[5 : len(tag)-1]
			two := strings.SplitN(tag, ",", 2)
			count := 2
			if len(two) == count {
				return [2]string{two[0], two[1]}
			}
			return [2]string{two[0], ""}
		}
	}
	return [2]string{}
}

func hcltag(tag reflect.StructTag) []byte {
	two := tag2(tag)
	return []byte(two[0])
}
