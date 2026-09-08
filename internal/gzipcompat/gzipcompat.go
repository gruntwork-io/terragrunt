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

	var output bytes.Buffer

	_, _ = output.WriteString(gzipHeader)

	var d compressor

	d.init(&output)

	if _, err := d.write(data); err != nil {
		return "", err
	}

	if err := d.syncFlush(); err != nil {
		return "", err
	}

	if err := d.close(); err != nil {
		return "", err
	}

	var trailer [8]byte

	binary.LittleEndian.PutUint32(trailer[0:4], crc32.ChecksumIEEE(data))
	binary.LittleEndian.PutUint32(trailer[4:8], uint32(len(data)))
	_, _ = output.Write(trailer[:])

	return base64.StdEncoding.EncodeToString(output.Bytes()), nil
}

// Func returns Encode as the HCL function base64gzip(str); onUse runs first on every call.
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
