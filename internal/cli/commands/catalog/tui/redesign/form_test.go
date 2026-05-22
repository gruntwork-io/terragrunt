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
func pressJ() tea.KeyPressMsg        { return tea.KeyPressMsg{Code: 'j', Text: "j"} }
func pressK() tea.KeyPressMsg        { return tea.KeyPressMsg{Code: 'k', Text: "k"} }
func pressX() tea.KeyPressMsg        { return tea.KeyPressMsg{Code: 'x', Text: "x"} }
func pressEnter() tea.KeyPressMsg    { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func pressEsc() tea.KeyPressMsg      { return tea.KeyPressMsg{Code: tea.KeyEscape} }
func pressCtrlD() tea.KeyPressMsg    { return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl} }
func pressTab() tea.KeyPressMsg      { return tea.KeyPressMsg{Code: tea.KeyTab} }
func pressShiftTab() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift} }

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

func TestFormValidationOnExitFlagsBadHCL(t *testing.T) {
	t.Parallel()

	// Type a partial map literal and confirm no error surfaces while
	// the user is still typing; on esc back to navigate the validator
	// runs once and flags the broken value.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "tags", TypeStr: "map"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range `{"foo": "ba` {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	assert.Empty(t, f.Field(0).ValidationErr,
		"validation shouldn't fire mid-typing; the user is still working")

	f, _ = f.Update(pressEsc())

	assert.NotEmpty(t, f.Field(0).ValidationErr,
		"on blur (esc back to navigate) the broken literal should be flagged")

	// Re-enter edit and finish the literal; on the next blur the error
	// should clear.
	f, _ = f.Update(pressEnter())

	for _, r := range `r"}` {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	f, _ = f.Update(pressEsc())

	assert.Empty(t, f.Field(0).ValidationErr,
		"completing the literal then blurring should clear the error")
}

func TestFormValidationOnExitFlagsTypeMismatch(t *testing.T) {
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

	f, _ = f.Update(pressEsc())

	assert.NotEmpty(t, f.Field(0).ValidationErr,
		"a string literal in a number field should be flagged on blur")
}

func TestFormValidationOnExitFlagsBareIdentifier(t *testing.T) {
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

	f, _ = f.Update(pressEsc())

	assert.NotEmpty(t, f.Field(0).ValidationErr,
		"a bare identifier should be flagged on blur as not matching the declared type")
}

func TestFormValidationOnExitAcceptsReferences(t *testing.T) {
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

	f, _ = f.Update(pressEsc())

	assert.Empty(t, f.Field(0).ValidationErr,
		"references should be accepted on blur; eval errors aren't validation errors")
}

func TestFormValidationDoesNotFireMidTyping(t *testing.T) {
	t.Parallel()

	// Eben specifically called out distracting partial-syntax errors
	// flashing while typing. Confirm no validation error appears until
	// the user signals they're done (esc, or a future tab to the next
	// field).
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "port", TypeStr: "number"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range `"abc` {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})

		assert.Empty(t, f.Field(0).ValidationErr,
			"validation must not fire on any keystroke; only on blur")
	}
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

func TestFormTabCyclesCategoryAndNarrowsCursor(t *testing.T) {
	t.Parallel()

	// Mix required and optional fields; the cursor starts at the first
	// required field. tab moves to the Required-only tab (no-op for the
	// cursor since it's already on a required), then to Optional which
	// snaps the cursor onto the first optional field.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
		{Name: "vpc_id", Required: true, Literal: true, TypeStr: "string"},
		{Name: "tier", Literal: true, TypeStr: "string", Initial: "basic"},
		{Name: "debug", Checkbox: true, TypeStr: "bool"},
	})
	f.SetSize(120, 40)

	require.Equal(t, 0, f.Cursor())

	// All -> Required.
	f, _ = f.Update(pressTab())
	assert.Equal(t, 0, f.Cursor(),
		"first required is already focused; cursor doesn't move")

	// j should stay inside Required-only.
	f, _ = f.Update(pressJ())
	assert.Equal(t, 1, f.Cursor(), "j walks to the next required field")

	f, _ = f.Update(pressJ())
	assert.Equal(t, 1, f.Cursor(),
		"j past the last required clamps; Optional fields aren't reachable here")

	// Required -> Optional snaps to the first optional.
	f, _ = f.Update(pressTab())
	assert.Equal(t, 2, f.Cursor(),
		"switching to Optional should land on the first optional field")

	// shift+tab back to Required.
	f, _ = f.Update(pressShiftTab())
	assert.Less(t, f.Cursor(), 2,
		"shift+tab from Optional cycles back to Required; cursor returns to a required field")
}

func TestFormCategoryANDsWithTextFilter(t *testing.T) {
	t.Parallel()

	// Optional category + "vpc" text filter should leave only optional
	// fields whose names contain "vpc".
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
		{Name: "vpc_required", Required: true, Literal: true, TypeStr: "string"},
		{Name: "vpc_cidr", Literal: true, TypeStr: "string", Initial: "10.0.0.0/16"},
		{Name: "vpc_name", Literal: true, TypeStr: "string", Initial: "main"},
		{Name: "tier", Literal: true, TypeStr: "string", Initial: "basic"},
	})
	f.SetSize(120, 40)

	// Switch to Optional (tab x2: All -> Required -> Optional).
	f, _ = f.Update(pressTab())
	f, _ = f.Update(pressTab())

	// Open the filter and type "vpc".
	f, _ = f.Update(tea.KeyPressMsg{Code: '/', Text: "/"})

	for _, r := range "vpc" {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	f, _ = f.Update(pressEnter())

	// Walk every visible field and confirm the visible set is the
	// intersection of Optional + contains("vpc").
	const walkSteps = 4

	visited := make([]string, 0, walkSteps+1)
	visited = append(visited, f.Field(f.Cursor()).Name)

	for range walkSteps {
		f, _ = f.Update(pressJ())
		visited = append(visited, f.Field(f.Cursor()).Name)
	}

	for _, name := range visited {
		assert.Contains(t, name, "vpc", "filter should restrict to vpc-named fields")
		assert.NotEqual(t, "vpc_required", name,
			"required vpc_required must be excluded by the Optional category")
	}
}

func TestFormTabInEditCommitsValidatesAndAdvancesStayingInEdit(t *testing.T) {
	t.Parallel()

	// Enter edit on the first field, type a value, tab to the next field.
	// The first field should be marked Set with no validation error (the
	// value is a valid HCL number), the cursor should move to field 1,
	// and edit mode should persist so the next character lands in the
	// new textinput.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "port", Required: true, TypeStr: "number"},
		{Name: "host", Required: true, TypeStr: "string", Literal: true},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range "8080" {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	f, _ = f.Update(pressTab())

	assert.Equal(t, 1, f.Cursor(), "tab should advance the cursor")
	assert.True(t, f.Field(0).Set,
		"the original field should be committed (Set=true) on tab")
	assert.Empty(t, f.Field(0).ValidationErr,
		"valid input should produce no on-blur validation error")

	// Type into the new field; if edit mode persisted, the textinput
	// should pick up the character.
	f, _ = f.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})

	assert.Equal(t, "a", f.Field(1).Input.Value(),
		"after tab the next field should be in edit mode and accept input")
}

func TestFormTabInEditFlagsBadValueOnBlur(t *testing.T) {
	t.Parallel()

	// Tab away from a broken value: validation should fire as part of
	// the tab-commit step, same as it would on esc.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "port", Required: true, TypeStr: "number"},
		{Name: "host", Required: true, TypeStr: "string", Literal: true},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range `"oops"` {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	f, _ = f.Update(pressTab())

	assert.Equal(t, 1, f.Cursor())
	assert.NotEmpty(t, f.Field(0).ValidationErr,
		"a type mismatch should be flagged when tab commits the field")
}

func TestFormShiftTabInEditMovesPrev(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "port", Required: true, TypeStr: "number"},
		{Name: "host", Required: true, TypeStr: "string", Literal: true},
	})
	f.SetSize(120, 40)

	// Move cursor to field 1, enter edit, then shift+tab back to field 0.
	f, _ = f.Update(pressJ())
	f, _ = f.Update(pressEnter())
	f, _ = f.Update(pressShiftTab())

	assert.Equal(t, 0, f.Cursor(),
		"shift+tab in edit mode should move to the previous visible field")
}

func TestFormTabInEditAtBoundaryIsNoOp(t *testing.T) {
	t.Parallel()

	// On the last field, tab should not wrap; the user stays put with
	// edit mode preserved.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "a", Required: true, Literal: true, TypeStr: "string"},
		{Name: "b", Required: true, Literal: true, TypeStr: "string"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressJ())
	f, _ = f.Update(pressEnter())

	for _, r := range "x" {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	f, _ = f.Update(pressTab())

	assert.Equal(t, 1, f.Cursor(),
		"tab at the last field is a no-op")

	// Subsequent character still lands in field 1.
	f, _ = f.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	assert.Equal(t, "xy", f.Field(1).Input.Value())
}

func TestFormDetailOverlayOpenAndClose(t *testing.T) {
	t.Parallel()

	// `?` from navigate opens the overlay; `?` again closes it.
	// Default-state form is in navigate mode.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string",
			Description: "AWS region for the resource."},
	})
	f.SetSize(120, 40)

	assert.False(t, f.DetailOpen())

	f, _ = f.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	assert.True(t, f.DetailOpen(), "? in navigate mode should open the overlay")

	view := f.View()
	assert.Contains(t, view, "AWS region for the resource.",
		"the overlay should render the full description")
	assert.Contains(t, view, "? close",
		"the overlay should advertise how to close")

	f, _ = f.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	assert.False(t, f.DetailOpen(), "? again should close the overlay")
}

func TestFormDetailOverlayEscClosesWithoutCancelingForm(t *testing.T) {
	t.Parallel()

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string",
			Description: "AWS region."},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	require.True(t, f.DetailOpen())

	f, cmd := f.Update(pressEsc())

	assert.False(t, f.DetailOpen(), "esc should close the overlay")
	assert.Nil(t, drainFormCmds(cmd),
		"esc in the overlay must not also cancel the form")
}

func TestFormUnfocusedDescriptionIsTruncatedAndFocusedIsFull(t *testing.T) {
	t.Parallel()

	longDesc := "L1\nL2\nL3\nL4"

	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "a", Required: true, Literal: true, TypeStr: "string",
			Description: longDesc},
		{Name: "b", Required: true, Literal: true, TypeStr: "string",
			Description: longDesc},
	})
	f.SetSize(120, 40)

	view := f.View()

	// Field 0 is focused: full description visible.
	for _, line := range []string{"L1", "L2", "L3", "L4"} {
		assert.Contains(t, view, line,
			"focused field's description should render in full")
	}

	// Move focus to field 1; field 0 is now unfocused. With descPreviewLines=2
	// and an extra paragraph past it, the unfocused field's render should
	// end with an ellipsis marker. We can't tell which field's L3/L4 lines
	// are which without parsing, so confirm an ellipsis appears in the view
	// (which only happens via the truncation helper).
	f, _ = f.Update(pressJ())

	view = f.View()
	assert.Contains(t, view, "…",
		"unfocused field with a long description should be truncated with …")
}

func TestFormUntouchedCursorTracksFirstVisibleAcrossTabs(t *testing.T) {
	t.Parallel()

	// A user who only tabs / searches (no j/k) should keep focus on the
	// first visible field as the category changes. Mirrors the list view's
	// "freshly inserted item snaps selection to 0 until you navigate"
	// pattern.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
		{Name: "vpc_id", Required: true, Literal: true, TypeStr: "string"},
		{Name: "tier", Literal: true, TypeStr: "string", Initial: "basic"},
		{Name: "size", Literal: true, TypeStr: "string", Initial: "small"},
	})
	f.SetSize(120, 40)

	// All -> Required: still first.
	f, _ = f.Update(pressTab())
	assert.Equal(t, 0, f.Cursor(), "first required is still index 0")

	// Required -> Optional: first optional is index 2.
	f, _ = f.Update(pressTab())
	assert.Equal(t, 2, f.Cursor(),
		"un-navigated user should land on the first visible field after a tab")

	// Optional -> All: first overall is index 0.
	f, _ = f.Update(pressTab())
	assert.Equal(t, 0, f.Cursor(),
		"un-navigated user should snap back to the first field on All")
}

func TestFormNavigationStickyAfterJK(t *testing.T) {
	t.Parallel()

	// Once the user presses j/k, subsequent tabs preserve the cursor
	// (when the field stays visible) instead of snapping to the top.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "a", Required: true, Literal: true, TypeStr: "string"},
		{Name: "b", Required: true, Literal: true, TypeStr: "string"},
		{Name: "c", Required: true, Literal: true, TypeStr: "string"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressJ())
	require.Equal(t, 1, f.Cursor())

	// Tab cycles to Required (already required-only fields here); the
	// cursor's field is still visible, so it should stay put.
	f, _ = f.Update(pressTab())
	assert.Equal(t, 1, f.Cursor(),
		"after j, the cursor sticks across category cycles when still visible")
}

func TestFormUntouchedCursorTracksFirstMatchWhileFiltering(t *testing.T) {
	t.Parallel()

	// A user who hasn't navigated should also have the cursor snap to
	// the first match as they type into the filter, even when their
	// pre-filter cursor would have stayed visible.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
		{Name: "vpc_id", Required: true, Literal: true, TypeStr: "string"},
	})
	f.SetSize(120, 40)

	// Open filter, type 'v' so "vpc_id" matches but "region" doesn't.
	f, _ = f.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	f, _ = f.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})

	assert.Equal(t, 1, f.Cursor(),
		"un-navigated cursor should snap to the first matching field")
}

func TestFormEnterInEditExitsBackToNavigateOnTextField(t *testing.T) {
	t.Parallel()

	// Enter on a text/HCL field acts as the symmetric counterpart to
	// the enter that brought the user into edit mode: commit + validate
	// + return to navigate, same as esc.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "region", Required: true, Literal: true, TypeStr: "string"},
	})
	f.SetSize(120, 40)

	f, _ = f.Update(pressEnter())

	for _, r := range "us-east-1" {
		f, _ = f.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	// Sanity: the keystrokes went into the input (edit mode is live).
	require.Equal(t, "us-east-1", f.Field(0).Input.Value())

	f, _ = f.Update(pressEnter())

	// After enter we should be back in navigate; a typed character
	// should no longer reach the input.
	f, _ = f.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})

	assert.Equal(t, "us-east-1", f.Field(0).Input.Value(),
		"enter in edit mode should exit; subsequent keystrokes shouldn't reach the input")
	assert.True(t, f.Field(0).Set,
		"the edited field should be committed (Set=true) on enter")
}

func TestFormEnterInEditStillTogglesBool(t *testing.T) {
	t.Parallel()

	// Enter on a bool field in edit mode keeps its toggle semantics; the
	// new exit-on-enter behavior is text-field-only.
	f := redesign.NewFormModel(nil, []redesign.FormField{
		{Name: "debug", Checkbox: true, TypeStr: "bool"},
	})
	f.SetSize(120, 40)

	require.False(t, f.Field(0).Bool)
	require.False(t, f.Field(0).Set)

	// First enter: navigate -> edit on a checkbox flips into edit mode
	// without committing yet (Set stays false until a toggle).
	f, _ = f.Update(pressEnter())

	// Second enter: toggle to true.
	f, _ = f.Update(pressEnter())

	assert.True(t, f.Field(0).Bool, "enter in checkbox edit mode should toggle")
	assert.True(t, f.Field(0).Set,
		"toggle commits the field so values() emits it")
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
