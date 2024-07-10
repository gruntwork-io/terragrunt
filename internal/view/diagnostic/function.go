package diagnostic

import (
	"encoding/json"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// FunctionParam represents a single parameter to a function, as represented by type Function.
type FunctionParam struct {
	// Name is a name for the function which is used primarily for documentation purposes.
	Name string `json:"name"`

	// Type is a type constraint which is a static approximation of the possibly-dynamic type of the parameter
	Type json.RawMessage `json:"type"`

	Description     string `json:"description,omitempty"`
	DescriptionKind string `json:"description_kind,omitempty"`
}

func DescribeFunctionParam(p *function.Parameter) FunctionParam {
	ret := FunctionParam{
		Name: p.Name,
	}

	if raw, err := p.Type.MarshalJSON(); err != nil {
		// Treat any errors as if the function is dynamically typed because it would be weird to get here.
		ret.Type = json.RawMessage(`"dynamic"`)
	} else {
		ret.Type = raw
	}

	return ret
}

// Function is a description of the JSON representation of the signature of a function callable from the Terraform language.
type Function struct {
	// Name is the leaf name of the function, without any namespace prefix.
	Name string `json:"name"`

	Params        []FunctionParam `json:"params"`
	VariadicParam *FunctionParam  `json:"variadic_param,omitempty"`

	// ReturnType is type constraint which is a static approximation of the possibly-dynamic return type of the function.
	ReturnType json.RawMessage `json:"return_type"`

	Description     string `json:"description,omitempty"`
	DescriptionKind string `json:"description_kind,omitempty"`
}

// DescribeFunction returns a description of the signature of the given cty function, as a pointer to this package's serializable type Function.
func DescribeFunction(name string, f function.Function) *Function {
	ret := &Function{
		Name: name,
	}

	params := f.Params()
	ret.Params = make([]FunctionParam, len(params))
	typeCheckArgs := make([]cty.Type, len(params), len(params)+1)
	for i, param := range params {
		ret.Params[i] = DescribeFunctionParam(&param)
		typeCheckArgs[i] = param.Type
	}
	if varParam := f.VarParam(); varParam != nil {
		descParam := DescribeFunctionParam(varParam)
		ret.VariadicParam = &descParam
		typeCheckArgs = append(typeCheckArgs, varParam.Type)
	}

	retType, err := f.ReturnType(typeCheckArgs)
	if err != nil {
		retType = cty.DynamicPseudoType
	}

	if raw, err := retType.MarshalJSON(); err != nil {
		// Treat any errors as if the function is dynamically typed because it would be weird to get here.
		ret.ReturnType = json.RawMessage(`"dynamic"`)
	} else {
		ret.ReturnType = raw
	}

	return ret
}

// FunctionCall represents a function call whose information is being included as part of a diagnostic snippet.
type FunctionCall struct {
	// CalledAs is the full name that was used to call this function, potentially including namespace prefixes if the function does not belong to the default function namespace.
	CalledAs string `json:"called_as"`

	// Signature is a description of the signature of the function that was/ called, if any.
	Signature *Function `json:"signature,omitempty"`
}

func DescribeFunctionCall(hclDiag *hcl.Diagnostic) *FunctionCall {
	callInfo := ExtraInfo[hclsyntax.FunctionCallDiagExtra](hclDiag)
	if callInfo == nil || callInfo.CalledFunctionName() == "" {
		return nil
	}

	calledAs := callInfo.CalledFunctionName()
	baseName := calledAs
	if idx := strings.LastIndex(baseName, "::"); idx >= 0 {
		baseName = baseName[idx+2:]
	}

	var signature *Function

	if f, ok := hclDiag.EvalContext.Functions[calledAs]; ok {
		signature = DescribeFunction(baseName, f)
	}

	return &FunctionCall{
		CalledAs:  calledAs,
		Signature: signature,
	}
}
