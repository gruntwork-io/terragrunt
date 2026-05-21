package redesign_test

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

// drainFormCmds walks a tea.Cmd chain and returns the first non-nil
// message. tea.BatchMsg is expanded in order so a form-emitted submit /
// cancel hidden inside a batch still surfaces.
func drainFormCmds(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	if msg == nil {
		return nil
	}

	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}

	for _, c := range batch {
		if got := drainFormCmds(c); got != nil {
			return got
		}
	}

	return nil
}

// pressJ etc. are tiny ergonomic helpers so the modal test sequences stay
// readable.
func pressJ() tea.KeyPressMsg     { return tea.KeyPressMsg{Code: 'j', Text: "j"} }
func pressK() tea.KeyPressMsg     { return tea.KeyPressMsg{Code: 'k', Text: "k"} }
func pressX() tea.KeyPressMsg     { return tea.KeyPressMsg{Code: 'x', Text: "x"} }
func pressEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func pressEsc() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyEscape} }
func pressCtrlD() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl} }

func TestFormFieldsFromParsedVariables_OrderingAndMetadata(t *testing.T) {
	t.Parallel()

	required := []*config.ParsedVariable{
		{Name: "region", Type: "string", Description: "AWS region.", DefaultValuePlaceholder: `""`},
		{Name: "app_name", Type: "string", Description: "App name.", DefaultValuePlaceholder: `""`},
	}
	optional := []*config.ParsedVariable{
		{Name: "tier", Type: "string", DefaultValue: `"basic"`, DefaultValuePlaceholder: `""`},
	}

	fields := redesign.FieldsFromParsedVariables(required, optional)

	require.Len(t, fields, 3)

	assert.Equal(t, "region", fields[0].Name)
	assert.True(t, fields[0].Required)
	assert.True(t, fields[0].Literal)
	assert.False(t, fields[0].Set, "fields start unset; user opts in via edit or toggle")

	assert.Equal(t, "tier", fields[2].Name)
	assert.False(t, fields[2].Required)
	assert.True(t, fields[2].Literal)
	assert.Equal(t, "basic", fields[2].Initial,
		"optional string default seeds Initial so edit mode opens from a known value")
	assert.False(t, fields[2].Set, "optional default stays implicit until the user edits")
}

func TestFormStartsInNavigateMode(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
	})
	f.SetSize(120, 40)

	// In navigate mode the textinput receives nothing, so typing into a
	// field is a no-op until the user enters edit mode.
	f, _ = f.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})

	assert.Empty(t, f.Field(0).Input.Value(),
		"keystrokes in navigate mode shouldn't reach the textinput")
}

func TestFormJKNavigates(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "a", Required: true},
		{Name: "b", Required: true},
		{Name: "c"},
	})
	f.SetSize(120, 40)

	require.Equal(t, 0, f.Cursor())

	f, _ = f.Update(pressJ())
	assert.Equal(t, 1, f.Cursor())

	f, _ = f.Update(pressJ())
	assert.Equal(t, 2, f.Cursor())

	// Clamped at the last field.
	f, _ = f.Update(pressJ())
	assert.Equal(t, 2, f.Cursor())

	f, _ = f.Update(pressK())
	assert.Equal(t, 1, f.Cursor())

	f, _ = f.Update(pressK())
	assert.Equal(t, 0, f.Cursor())

	// Clamped at the first field.
	f, _ = f.Update(pressK())
	assert.Equal(t, 0, f.Cursor())
}

func TestFormEnterOnTextFieldEntersEditMode(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
	})
	f.SetSize(120, 40)

	// Enter edit mode, type a value, then esc back to navigate.
	f, _ = f.Update(pressEnter())
	f, _ = f.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	f, _ = f.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	f, _ = f.Update(pressEsc())

	assert.Equal(t, "us", f.Field(0).Input.Value(),
		"typed characters should reach the input while in edit mode")
	assert.True(t, f.Field(0).Set,
		"edit-mode changes flip Set to true on exit")
}

func TestFormEditExitWithoutChangeKeepsSetFalse(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "tier", Literal: true, TypeStr: "string", Initial: "basic"},
	})
	f.SetSize(120, 40)

	// Enter edit, immediately exit. The user "looked" but didn't change
	// anything, so the field stays unset and the source default applies.
	f, _ = f.Update(pressEnter())
	f, _ = f.Update(pressEsc())

	assert.False(t, f.Field(0).Set,
		"exit without changes should not mark the field Set")
}

func TestFormEnterOnCheckboxEntersEditThenToggles(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "enable_dns", Checkbox: true, TypeStr: "bool", Bool: false},
	})
	f.SetSize(120, 40)

	// First enter enters edit mode on the checkbox; no toggle yet.
	f, _ = f.Update(pressEnter())
	assert.False(t, f.Field(0).Bool, "first enter only enters edit mode")
	assert.False(t, f.Field(0).Set)

	// Subsequent enters toggle in place.
	f, _ = f.Update(pressEnter())
	assert.True(t, f.Field(0).Bool)
	assert.True(t, f.Field(0).Set)

	f, _ = f.Update(pressEnter())
	assert.False(t, f.Field(0).Bool)
	assert.True(t, f.Field(0).Set, "toggling stays Set; only x clears it")

	// Esc returns to navigate mode without losing the toggled value.
	f, _ = f.Update(pressEsc())
	assert.False(t, f.Field(0).Bool)
	assert.True(t, f.Field(0).Set)
}

func TestFormXUnsetsOptionalAndPreservesInput(t *testing.T) {
	t.Parallel()

	// Drive a text field to Set=true via edit, then press x to mark it
	// "use default" again. The input value should be preserved so the
	// user can recover their work by re-entering edit mode.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "tier", Literal: true, TypeStr: "string", Initial: "basic"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())
	f, _ = f.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	f, _ = f.Update(pressEsc())

	require.True(t, f.Field(0).Set)
	require.Equal(t, "basicp", f.Field(0).Input.Value())

	f, _ = f.Update(pressX())
	assert.False(t, f.Field(0).Set, "x should clear Set on an optional field")
	assert.Equal(t, "basicp", f.Field(0).Input.Value(),
		"x should preserve the input so the user can recover prior edits")
}

func TestFormXIsNoOpWhenAlreadyUnset(t *testing.T) {
	t.Parallel()

	// An optional field with a default is unset by default; pressing x
	// on it should NOT flip it to Set=true.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "tier", Literal: true, TypeStr: "string", Initial: "basic"},
	})
	f.SetSize(120, 40)
	require.False(t, f.Field(0).Set)

	f, _ = f.Update(pressX())
	assert.False(t, f.Field(0).Set,
		"x on an already-unset field should remain a no-op (one-way unset)")
}

func TestFormCapitalXUnsetsAllOptionalsAndLeavesRequired(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
		{Name: "tier", Literal: true, TypeStr: "string", Initial: "basic"},
		{Name: "enabled", Checkbox: true, TypeStr: "bool", Bool: true, BoolInitial: true},
	})
	f.SetSize(120, 40)

	// Set all three fields.
	f, _ = f.Update(pressEnter()) // edit region
	f, _ = f.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	f, _ = f.Update(pressEsc())
	f, _ = f.Update(pressJ())
	f, _ = f.Update(pressEnter()) // edit tier
	f, _ = f.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	f, _ = f.Update(pressEsc())
	f, _ = f.Update(pressJ())
	f, _ = f.Update(pressEnter()) // edit checkbox
	f, _ = f.Update(pressEnter()) // toggle
	f, _ = f.Update(pressEsc())

	require.True(t, f.Field(0).Set)
	require.True(t, f.Field(1).Set)
	require.True(t, f.Field(2).Set)

	f, _ = f.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})

	assert.True(t, f.Field(0).Set, "required fields should keep their Set state")
	assert.False(t, f.Field(1).Set, "optional text field should be unset by X")
	assert.False(t, f.Field(2).Set, "optional checkbox should be unset by X")

	// Input/Bool are preserved for the unset optionals.
	assert.Equal(t, "basicp", f.Field(1).Input.Value())
}

func TestFormXIsNoOpOnRequired(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
	})
	f.SetSize(120, 40)

	// Set the field via edit first.
	f, _ = f.Update(pressEnter())
	f, _ = f.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	f, _ = f.Update(pressEsc())

	require.True(t, f.Field(0).Set)

	// x must not flip Required fields.
	f, _ = f.Update(pressX())
	assert.True(t, f.Field(0).Set, "required fields can't be marked unset")
	assert.Equal(t, "a", f.Field(0).Input.Value(),
		"the input value should be preserved when x is ignored")
}

func TestFormSubmitOmitsUnsetFields(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
		{Name: "tier", Literal: true, TypeStr: "string", Initial: "basic"},
		{Name: "enabled", Checkbox: true, TypeStr: "bool", Bool: false},
	})
	f.SetSize(120, 40)

	// Set only the required field; leave the optional + bool unset.
	f, _ = f.Update(pressEnter())
	f, _ = f.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	f, _ = f.Update(pressEsc())

	_, cmd := f.Update(pressCtrlD())
	require.NotNil(t, cmd)

	msg := drainFormCmds(cmd)
	sub, ok := msg.(redesign.FormSubmitMsg)
	require.True(t, ok, "expected FormSubmitMsg, got %T", msg)

	assert.Equal(t, map[string]string{
		"region": `"u"`,
	}, sub.Values, "only fields the user explicitly set should appear in the submit map")
}

func TestFormBoolToggleEmitsValue(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "enabled", Checkbox: true, TypeStr: "bool", Bool: true},
	})
	f.SetSize(120, 40)

	// First enter enters edit mode; second enter toggles the value.
	f, _ = f.Update(pressEnter())
	f, _ = f.Update(pressEnter())

	_, cmd := f.Update(pressCtrlD())
	msg := drainFormCmds(cmd)
	sub, ok := msg.(redesign.FormSubmitMsg)
	require.True(t, ok)

	assert.Equal(t, map[string]string{"enabled": "false"}, sub.Values)
}

func TestFormLiteralFieldAutoQuotesOnSubmit(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range "us-east-1" {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	f, _ = f.Update(pressEsc())

	_, cmd := f.Update(pressCtrlD())
	msg := drainFormCmds(cmd)
	sub, ok := msg.(redesign.FormSubmitMsg)
	require.True(t, ok)

	assert.Equal(t, `"us-east-1"`, sub.Values["region"],
		"literal string fields wrap with strconv.Quote at submit")
}

func TestFormHCLValidationBlocksSubmit(t *testing.T) {
	t.Parallel()

	// Non-literal field (raw HCL); typing broken HCL should block submit.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "ports", Required: true, TypeStr: "list(number)"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range "[1, 2" {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	f, _ = f.Update(pressEsc())

	updated, cmd := f.Update(pressCtrlD())
	assert.False(t, updated.Submitted(), "broken HCL should block the submit cmd")
	assert.Nil(t, cmd, "no submit cmd should fire when validation fails")
	assert.NotEmpty(t, updated.Field(0).ValidationErr,
		"validation failure should attach an error to the field")
}

func TestFormLiveValidationFlagsBadHCL(t *testing.T) {
	t.Parallel()

	// Type a partial map literal one keystroke at a time and confirm the
	// field's ValidationErr toggles in response to the syntactic state.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "tags", TypeStr: "map"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range `{"foo": "ba` {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	assert.NotEmpty(t, f.Field(0).ValidationErr,
		"partial map literal should be flagged while still being typed")

	// Finish the literal; the error should clear.
	for _, r := range `r"}` {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	assert.Empty(t, f.Field(0).ValidationErr,
		"completing the map literal should clear the error")
}

func TestFormLiveValidationFlagsTypeMismatch(t *testing.T) {
	t.Parallel()

	// Declared type is "number" but the user types a string.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "port", TypeStr: "number"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range `"foo"` {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	assert.NotEmpty(t, f.Field(0).ValidationErr,
		"a string literal in a number field should be flagged")
}

func TestFormLiveValidationFlagsBareIdentifier(t *testing.T) {
	t.Parallel()

	// A bare word in a map field parses as a single-step traversal,
	// which HCL eval rejects "because variables aren't allowed". That
	// path used to fall through to "accept as a reference"; bare
	// identifiers are almost always typos, so the validator should
	// flag them against the declared type.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "tags", TypeStr: "map"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range "asdlfkj" {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	assert.NotEmpty(t, f.Field(0).ValidationErr,
		"a bare identifier should be flagged as not matching the declared type")
}

func TestFormLiveValidationAcceptsReferences(t *testing.T) {
	t.Parallel()

	// References can't be evaluated without context; the validator
	// should accept the syntactically-valid expression and let
	// terragrunt resolve it at run time.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "tags", TypeStr: "map"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range "local.tags" {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	assert.Empty(t, f.Field(0).ValidationErr,
		"references should be accepted; eval errors aren't validation errors")
}

func TestFormPageNavigationJumpsMultipleFields(t *testing.T) {
	t.Parallel()

	// Construct enough fields that one page-down doesn't reach the end.
	const fieldCount = 20

	fields := make([]redesign.FormField, fieldCount)
	for i := range fields {
		fields[i] = redesign.FormField{Name: fmt.Sprintf("f%02d", i), Literal: true, TypeStr: "string"}
	}

	f := redesign.NewFormModel(nil, fields)
	f.SetSize(120, 40)

	startCursor := f.Cursor()

	f, _ = f.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	assert.Greater(t, f.Cursor(), startCursor+1,
		"page-down should advance more than a single line")

	mid := f.Cursor()

	f, _ = f.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	assert.Less(t, f.Cursor(), mid,
		"page-up should move back from the mid-point")
}

func TestFormHomeEndJumpsToBounds(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "a", Required: true},
		{Name: "b", Required: true},
		{Name: "c", Required: true},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	assert.Equal(t, 2, f.Cursor(), "end should jump to the last field")

	f, _ = f.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	assert.Equal(t, 0, f.Cursor(), "home should jump back to the first field")
}

func TestFormFilterNarrowsAndNavigates(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
		{Name: "vpc_id", Required: true, Literal: true, TypeStr: "string"},
		{Name: "subnet_ids", Required: true, TypeStr: "list(string)"},
		{Name: "vpc_cidr", Required: true, Literal: true, TypeStr: "string"},
	})
	f.SetSize(120, 40)

	// Open the filter and type "vpc".
	f, _ = f.Update(tea.KeyPressMsg{Code: '/', Text: "/"})

	for _, r := range "vpc" {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	// Cursor should snap onto a vpc-prefixed field; the non-matching
	// region/subnet_ids should be skipped by j/k navigation.
	f, _ = f.Update(pressEnter()) // apply

	startCursor := f.Cursor()
	assert.Contains(t, f.Field(startCursor).Name, "vpc",
		"after applying the filter the cursor should sit on a matching field")

	f, _ = f.Update(pressJ())
	assert.Contains(t, f.Field(f.Cursor()).Name, "vpc",
		"j should walk to the next matching field, skipping non-matches")
	assert.NotEqual(t, startCursor, f.Cursor())
}

func TestFormFilterEntryReturnsFocusCmd(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true},
		{Name: "vpc_id", Required: true},
	})
	f.SetSize(120, 40)

	_, cmd := f.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	require.NotNil(t, cmd, "entering filter mode must return the filterInput Focus Cmd so the cursor blinks")
}

func TestFormFilterEscClearsAndPreservesForm(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true},
		{Name: "vpc_id", Required: true},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	f, _ = f.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	f, _ = f.Update(pressEnter()) // applied

	// First esc clears the applied filter, NOT the form.
	_, cmd := f.Update(pressEsc())
	assert.Nil(t, cmd, "esc on an applied filter should not emit a cancel message")

	// Second esc now cancels the form.
	_, cmd = f.Update(pressEsc())
	require.NotNil(t, cmd)
	_, ok := drainFormCmds(cmd).(redesign.FormCancelMsg)
	assert.True(t, ok, "esc with no active filter should cancel the form")
}

func TestFormCancelEmitsCancelMsg(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{{Name: "region", Required: true}})
	f.SetSize(120, 40)

	_, cmd := f.Update(pressEsc())
	require.NotNil(t, cmd)

	msg := drainFormCmds(cmd)
	_, ok := msg.(redesign.FormCancelMsg)
	assert.True(t, ok, "expected FormCancelMsg, got %T", msg)
}

func TestFormEscFromEditReturnsToNavigateNotCancel(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())
	f, _ = f.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})

	// esc from edit mode should NOT emit FormCancelMsg; it returns to navigate.
	_, cmd := f.Update(pressEsc())
	require.Nil(t, cmd, "esc out of edit mode shouldn't fire a cancel command")

	// Now a fresh esc in navigate mode cancels the form.
	_, cmd = f.Update(pressEsc())
	require.NotNil(t, cmd)
	_, ok := drainFormCmds(cmd).(redesign.FormCancelMsg)
	assert.True(t, ok)
}

func TestFormValuesReferencesPromotesBoolDefaults(t *testing.T) {
	t.Parallel()

	refs := redesign.ValuesReferences{
		Required: []string{"region"},
		Optional: []redesign.OptionalValue{
			{Name: "enable_dns", Default: cty.True},
			{Name: "delete_on_destroy", Default: cty.False},
		},
	}

	fields := redesign.FieldsFromValuesReferences(refs)
	require.Len(t, fields, 3)

	assert.False(t, fields[0].Checkbox,
		"required values.* with unknown type stay raw HCL")

	assert.True(t, fields[1].Checkbox)
	assert.True(t, fields[1].Bool, "string default true seeds Bool=true")
	assert.False(t, fields[1].Set, "default stays implicit until user opts in")

	assert.True(t, fields[2].Checkbox)
	assert.False(t, fields[2].Bool)
}

func TestCtyValueAsHCLRoundTrips(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   cty.Value
		want string
	}{
		{name: "string", in: cty.StringVal("hello"), want: `"hello"`},
		{name: "bool", in: cty.BoolVal(true), want: "true"},
		{name: "number", in: cty.NumberIntVal(42), want: "42"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, redesign.CtyValueAsHCL(tc.in))
		})
	}
}
