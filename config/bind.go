package config

import "github.com/zclconf/go-cty/cty"

// bindString bind parsed string value onto passed interface{} field
func bindString(arg cty.Value, val *interface{}, field string) {
	attrValue := arg.GetAttr(field)
	if !attrValue.IsNull() {
		*val = attrValue.AsString()
	}
}

// bindString bind parsed string value onto passed interface{} field
func bindNumber(arg cty.Value, val *interface{}, field string) {
	attrValue := arg.GetAttr(field)
	if !attrValue.IsNull() {
		retNumber := attrValue.AsBigFloat()

		if !retNumber.IsInt() {
			panic("not integer")
		}

		*val = retNumber
	}
}

// bindStringList bind parsed strings list onto passed interface{} field
func bindStringList(arg cty.Value, val *interface{}, field string) {
	attrValue := arg.GetAttr(field)
	var accountList []string
	if !attrValue.IsNull() {
		iter := attrValue.ElementIterator()
		for iter.Next() {
			_, accountId := iter.Element()
			accountList = append(accountList, accountId.AsString())
		}

		*val = accountList
	}
}

// bindBool bind parsed bool value onto passed interface{} field
func bindBool(arg cty.Value, val *interface{}, field string) {
	attrValue := arg.GetAttr(field)
	if !attrValue.IsNull() {
		*val = attrValue.True()
	}
}
