package hclparse

// FunctionCalls is the set of functions some HCL calls. The zero value is the set of HCL that may call any function.
type FunctionCalls struct {
	names map[string]struct{}
}

// AnyFunctionCalls returns the set of HCL that may call any function.
func AnyFunctionCalls() FunctionCalls {
	return FunctionCalls{}
}

// Calls reports whether the HCL may call the function name.
func (calls FunctionCalls) Calls(name string) bool {
	if calls.names == nil {
		return true
	}

	_, called := calls.names[name]

	return called
}

// Known reports whether the set names every function the HCL calls, which it does not for HCL that may call any.
func (calls FunctionCalls) Known() bool {
	return calls.names != nil
}

// All yields the name of each function the HCL calls. It yields none for HCL that may call any function.
func (calls FunctionCalls) All(yield func(name string) bool) {
	for name := range calls.names {
		if !yield(name) {
			return
		}
	}
}
