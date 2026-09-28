// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2026 Gruntwork, LLC
// SPDX-License-Identifier: MPL-2.0

package patch

import (
	"encoding/base64"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ErrBase64NotUTF8 is returned when a decoded base64 string is not valid
// UTF-8.
var ErrBase64NotUTF8 = errors.New(
	"the result of decoding the provided string is not valid UTF-8",
)

// Base64DecodeFunc returns a function that decodes a base64 string, reporting
// what it decoded to l when the result is not valid UTF-8.
func Base64DecodeFunc(l log.Logger) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:        "str",
				Type:        cty.String,
				AllowMarked: true,
			},
		},
		Type:         function.StaticReturnType(cty.String),
		RefineResult: refineNotNull,
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			str, strMarks := args[0].Unmark()
			s := str.AsString()

			decoded, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return cty.UnknownVal(cty.String), fmt.Errorf(
					"failed to decode base64 data %s",
					redact(s, strMarks),
				)
			}

			if !utf8.Valid(decoded) {
				l.Debugf(
					"the result of decoding the provided string is not valid UTF-8: %s",
					redact(string(decoded), strMarks),
				)

				return cty.UnknownVal(cty.String), ErrBase64NotUTF8
			}

			return cty.StringVal(string(decoded)).WithMarks(strMarks), nil
		},
	})
}
