package redesign

import (
	tea "charm.land/bubbletea/v2"
)

// NewCopyFinishedMsgForTest constructs the private copyFinishedMsg so
// external tests can drive Model.Update through the copy-exit code path
// without spinning up a real bubbletea runtime.
func NewCopyFinishedMsgForTest(err error, workingDir string, required []string, optional []string, valuesWritten, valuesSkipped bool) tea.Msg {
	opt := make([]OptionalValue, 0, len(optional))
	for _, name := range optional {
		opt = append(opt, OptionalValue{Name: name})
	}

	return copyFinishedMsg{
		err: err,
		result: copyResult{
			workingDir:    workingDir,
			references:    ValuesReferences{Required: required, Optional: opt},
			valuesWritten: valuesWritten,
			valuesSkipped: valuesSkipped,
		},
	}
}

// NewScaffoldFinishedMsgForTest constructs the private scaffoldFinishedMsg
// for external tests.
func NewScaffoldFinishedMsgForTest(err error) tea.Msg {
	return scaffoldFinishedMsg{err: err}
}

// NewRendererErrMsgForTest constructs the private rendererErrMsg for
// external tests.
func NewRendererErrMsgForTest(err error) tea.Msg {
	return rendererErrMsg{err: err}
}
