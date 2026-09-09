// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gzipcompat reproduces the base64gzip() output of Terragrunt v1.1.3 and earlier.
package gzipcompat

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

const gzipHeader = "\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff"

// Encode returns the gzip stream Terragrunt v1.1.3 produced for input, base64 encoded.
func Encode(input string) (string, error) {
	data := []byte(input)
	output := bytes.NewBufferString(gzipHeader)

	var d compressor

	d.init(output)

	if _, err := d.write(data); err != nil {
		return "", err
	}

	if err := d.syncFlush(); err != nil {
		return "", err
	}

	if err := d.close(); err != nil {
		return "", err
	}

	out := output.Bytes()
	out = binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(data))
	out = binary.LittleEndian.AppendUint32(out, uint32(len(data)))

	return base64.StdEncoding.EncodeToString(out), nil
}

// Func returns Encode as an HCL function of one string. onUse runs before each
// encoding and must not be nil; returning an error from it fails the call.
func Func(onUse func() error) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name: "str",
				Type: cty.String,
			},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			if err := onUse(); err != nil {
				return cty.UnknownVal(cty.String), err
			}

			encoded, err := Encode(args[0].AsString())
			if err != nil {
				return cty.UnknownVal(cty.String), err
			}

			return cty.StringVal(encoded), nil
		},
	})
}
