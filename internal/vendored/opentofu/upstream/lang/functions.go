// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	ctyyaml "github.com/zclconf/go-cty-yaml"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"

	"github.com/gruntwork-io/terragrunt/internal/vendored/opentofu/upstream/lang/funcs"
)

// MakeBaseFunctionTable constructs the initial table of functions that we uses as
// the basis for the function table in each scope.
//
// This function intentionally always returns a fresh map on each call because the
// caller is expected to modify it further before storing it as part of a
// particular [Scope], based on the unique settings of that scope.
func MakeBaseFunctionTable(baseDir string) map[string]function.Function {
	// Some of our functions are just directly the cty stdlib functions.
	// Others are implemented in the subdirectory "funcs" here in this
	// repository. New functions should generally start out their lives
	// in the "funcs" directory and potentially graduate to cty stdlib
	// later if the functionality seems to be something domain-agnostic
	// that would be useful to all applications using cty functions.

	ret := map[string]function.Function{
		"abs": stdlib.AbsoluteFunc,
		// "abspath":             funcs.AbsPathFunc,
		"alltrue":             funcs.AllTrueFunc,
		"anytrue":             funcs.AnyTrueFunc,
		"assumeequal":         funcs.AssumeEqualFunc,
		"assumelistlength":    funcs.AssumeListLengthFunc,
		"assumelistlengthmax": funcs.AssumeListLengthMaxFunc,
		"assumelistlengthmin": funcs.AssumeListLengthMinFunc,
		"assumemaplength":     funcs.AssumeMapLengthFunc,
		"assumemaplengthmax":  funcs.AssumeMapLengthMaxFunc,
		"assumemaplengthmin":  funcs.AssumeMapLengthMinFunc,
		"assumenotnull":       funcs.AssumeNotNullFunc,
		"assumesetlength":     funcs.AssumeSetLengthFunc,
		"assumesetlengthmax":  funcs.AssumeSetLengthMaxFunc,
		"assumesetlengthmin":  funcs.AssumeSetLengthMinFunc,
		"assumestringprefix":  funcs.AssumeStringPrefixFunc,
		"basename":            funcs.BasenameFunc,
		// "base64decode":        funcs.Base64DecodeFunc,
		"base64encode":    funcs.Base64EncodeFunc,
		"base64gzip":      funcs.Base64GzipFunc,
		"base64gunzip":    funcs.Base64GunzipFunc,
		"base64sha256":    funcs.Base64Sha256Func,
		"base64sha512":    funcs.Base64Sha512Func,
		"bcrypt":          funcs.BcryptFunc,
		"can":             tryfunc.CanFunc,
		"ceil":            stdlib.CeilFunc,
		"chomp":           stdlib.ChompFunc,
		"cidrcontains":    funcs.CidrContainsFunc,
		"cidrhost":        funcs.CidrHostFunc,
		"cidrnetmask":     funcs.CidrNetmaskFunc,
		"cidrsubnet":      funcs.CidrSubnetFunc,
		"cidrsubnets":     funcs.CidrSubnetsFunc,
		"coalesce":        funcs.CoalesceFunc,
		"coalescelist":    stdlib.CoalesceListFunc,
		"compact":         stdlib.CompactFunc,
		"concat":          stdlib.ConcatFunc,
		"contains":        stdlib.ContainsFunc,
		"csvdecode":       stdlib.CSVDecodeFunc,
		"dirname":         funcs.DirnameFunc,
		"distinct":        stdlib.DistinctFunc,
		"element":         stdlib.ElementFunc,
		"endswith":        funcs.EndsWithFunc,
		"ephemeralasnull": funcs.EphemeralAsNullFunc,
		"chunklist":       stdlib.ChunklistFunc,
		// "file":                funcs.MakeFileFunc(baseDir, false),
		// "fileexists":          funcs.MakeFileExistsFunc(baseDir),
		// "fileset":             funcs.MakeFileSetFunc(baseDir),
		// "filebase64":          funcs.MakeFileFunc(baseDir, true),
		// "filebase64sha256":    funcs.MakeFileBase64Sha256Func(baseDir),
		// "filebase64sha512":    funcs.MakeFileBase64Sha512Func(baseDir),
		// "filemd5":             funcs.MakeFileMd5Func(baseDir),
		// "filesha1":            funcs.MakeFileSha1Func(baseDir),
		// "filesha256":          funcs.MakeFileSha256Func(baseDir),
		// "filesha512":          funcs.MakeFileSha512Func(baseDir),
		"flatten":    stdlib.FlattenFunc,
		"floor":      stdlib.FloorFunc,
		"format":     stdlib.FormatFunc,
		"formatdate": stdlib.FormatDateFunc,
		"formatlist": stdlib.FormatListFunc,
		"indent":     stdlib.IndentFunc,
		"index":      funcs.IndexFunc, // stdlib.IndexFunc is not compatible
		"join":       stdlib.JoinFunc,
		"jsondecode": stdlib.JSONDecodeFunc,
		"jsonencode": stdlib.JSONEncodeFunc,
		"keys":       stdlib.KeysFunc,
		"length":     funcs.LengthFunc,
		"list":       funcs.ListFunc,
		"log":        stdlib.LogFunc,
		"lookup":     funcs.LookupFunc,
		"lower":      stdlib.LowerFunc,
		"map":        funcs.MapFunc,
		"matchkeys":  funcs.MatchkeysFunc,
		"max":        stdlib.MaxFunc,
		"md5":        funcs.Md5Func,
		"merge":      stdlib.MergeFunc,
		"min":        stdlib.MinFunc,
		"one":        funcs.OneFunc,
		"parseint":   stdlib.ParseIntFunc,
		// "pathexpand":          funcs.PathExpandFunc,
		"pow":              stdlib.PowFunc,
		"range":            stdlib.RangeFunc,
		"regex":            stdlib.RegexFunc,
		"regexall":         stdlib.RegexAllFunc,
		"replace":          funcs.ReplaceFunc,
		"reverse":          stdlib.ReverseListFunc,
		"rsadecrypt":       funcs.RsaDecryptFunc,
		"sensitive":        funcs.SensitiveFunc,
		"nonsensitive":     funcs.NonsensitiveFunc,
		"issensitive":      funcs.IsSensitiveFunc,
		"setintersection":  stdlib.SetIntersectionFunc,
		"setproduct":       stdlib.SetProductFunc,
		"setsubtract":      stdlib.SetSubtractFunc,
		"setunion":         stdlib.SetUnionFunc,
		"sha1":             funcs.Sha1Func,
		"sha256":           funcs.Sha256Func,
		"sha512":           funcs.Sha512Func,
		"signum":           stdlib.SignumFunc,
		"slice":            stdlib.SliceFunc,
		"sort":             stdlib.SortFunc,
		"split":            stdlib.SplitFunc,
		"startswith":       funcs.StartsWithFunc,
		"strcontains":      funcs.StrContainsFunc,
		"strrev":           stdlib.ReverseFunc,
		"substr":           stdlib.SubstrFunc,
		"sum":              funcs.SumFunc,
		"textdecodebase64": funcs.TextDecodeBase64Func,
		"textencodebase64": funcs.TextEncodeBase64Func,
		"timestamp":        funcs.TimestampFunc,
		"timeadd":          stdlib.TimeAddFunc,
		"timecmp":          funcs.TimeCmpFunc,
		"title":            stdlib.TitleFunc,
		"tostring":         funcs.MakeToFunc(cty.String),
		"tonumber":         funcs.MakeToFunc(cty.Number),
		"tobool":           funcs.MakeToFunc(cty.Bool),
		"toset":            funcs.MakeToFunc(cty.Set(cty.DynamicPseudoType)),
		"tolist":           funcs.MakeToFunc(cty.List(cty.DynamicPseudoType)),
		"tomap":            funcs.MakeToFunc(cty.Map(cty.DynamicPseudoType)),
		"transpose":        funcs.TransposeFunc,
		"trim":             stdlib.TrimFunc,
		"trimprefix":       stdlib.TrimPrefixFunc,
		"trimspace":        stdlib.TrimSpaceFunc,
		"trimsuffix":       stdlib.TrimSuffixFunc,
		"try":              tryfunc.TryFunc,
		"upper":            stdlib.UpperFunc,
		"urlencode":        funcs.URLEncodeFunc,
		"urldecode":        funcs.URLDecodeFunc,
		"uuid":             funcs.UUIDFunc,
		"uuidv5":           funcs.UUIDV5Func,
		"values":           stdlib.ValuesFunc,
		"yamldecode":       ctyyaml.YAMLDecodeFunc,
		"yamlencode":       ctyyaml.YAMLEncodeFunc,
		"zipmap":           stdlib.ZipmapFunc,
	}

	// The patch package registers a vfs-backed replacement for this:
	//
	// ret["templatefile"] = funcs.MakeTemplateFileFunc(baseDir, func() map[string]function.Function {
	// 	return ret
	// })

	// Registers "templatestring" function in function map.
	ret["templatestring"] = funcs.MakeTemplateStringFunc(baseDir, func() map[string]function.Function {
		// This anonymous function returns the existing map of functions for initialization.
		return ret
	})

	return ret
}
