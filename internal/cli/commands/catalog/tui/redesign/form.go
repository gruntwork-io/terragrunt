package redesign

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/paginator"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/pkg/config"
)

// formMode is the form's modal state. The form opens in navigate mode;
// `enter` on a text field switches to edit mode, and `esc` from edit mode
// returns to navigate. Bool fields toggle in place from navigate mode and
// never enter edit mode.
type formMode int

const (
	navigateMode formMode = iota
	editMode
)

// filterState tracks the form's filter widget. `/` in navigate mode opens
// the filter (filterTyping); enter commits it (filterApplied); esc clears.
// While typing or applied, navigation and rendering operate on the
// matching subset of fields.
type filterState int

const (
	filterInactive filterState = iota
	filterTyping
	filterApplied
)

// FormField captures the prompt and current value of one discovered
// variable. Placeholder is the ghost text shown when the input is empty;
// Initial is the default value pre-loaded into the input for optional
// fields and pre-loaded when the user enters edit mode.
//
// Set indicates whether this field's value should be written to the
// generated file. Optional fields default to Set=false so the source's
// default stays implicit; required fields with Set=false render as
// `# TODO: fill in value`. Set flips to true when the user toggles a bool
// (via enter), edits a text field's content, or explicitly opts back in
// after an `x` toggle.
//
// Literal is set when the field accepts a plain string value instead of a
// raw HCL expression. In literal mode the form skips HCL validation and
// wraps the user's input with strconv.Quote at submit time.
//
// Checkbox is set for bool-typed variables: the textinput is replaced by
// a togglable `[x] true` / `[ ] false` widget. `enter` flips the value
// directly in navigate mode; non-checkbox fields enter edit mode instead.
//
//nolint:govet // field order chosen for readability over alignment
type FormField struct {
	Input         textinput.Model
	Name          string
	Description   string
	TypeStr       string
	Placeholder   string
	Initial       string
	ValidationErr string
	Bool          bool
	BoolInitial   bool
	Set           bool
	Required      bool
	Literal       bool
	Checkbox      bool
}

// navigateKeyMap groups the navigate-mode bindings. These mirror the
// list-view conventions used elsewhere in the catalog TUI: j/k (and
// arrows) for line moves, h/l (with pgup/pgdn aliases) for page moves,
// home/end for jump-to-end, and `/` for filter.
type navigateKeyMap struct {
	Next          key.Binding
	Prev          key.Binding
	NextPage      key.Binding
	PrevPage      key.Binding
	GoToStart     key.Binding
	GoToEnd       key.Binding
	Interact      key.Binding
	Unset         key.Binding
	UnsetAll      key.Binding
	Filter        key.Binding
	SubmitChecked key.Binding
	Submit        key.Binding
	Cancel        key.Binding
}

// editKeyMap groups the edit-mode bindings. Most keypresses on a text
// field fall through to the focused textinput; ExitEdit, Submit, and
// Toggle (bool-only) are intercepted.
type editKeyMap struct {
	ExitEdit key.Binding
	Submit   key.Binding
	Toggle   key.Binding
}

// FormModel is the interactive value-collection view shown when the user
// presses `s`. Each field corresponds to a discovered variable; submission
// emits a FormSubmitMsg carrying a name->raw-HCL map consumed by either
// scaffold.Plan.Generate or WriteValuesFile.
//
// The form is modal. In navigate mode (the default) j/k move between
// fields, enter interacts with the focused field, x toggles whether an
// optional field's value is included in the output, and esc cancels the
// form. In edit mode a text field's input is live; esc returns to navigate.
//
//nolint:govet // field order chosen for readability over alignment
type FormModel struct {
	component   *Component
	fields      []FormField
	navKeys     navigateKeyMap
	editKeys    editKeyMap
	filterInput textinput.Model
	help        help.Model
	paginator   paginator.Model
	editPreEdit string
	mode        formMode
	filter      filterState
	cursor      int
	pageStart   int
	bodyHeight  int
	width       int
	height      int
	submitted   bool
}

// FormSubmitMsg carries the collected values from a completed form back to
// the outer Model. Empty inputs are omitted so the placeholder/default
// fallback applies at write time.
type FormSubmitMsg struct {
	Values map[string]string
}

// FormCancelMsg signals an esc-from-form, sending control back to the
// pager state without performing the scaffold or copy.
type FormCancelMsg struct{}

// Cursor reports the index of the currently focused field. Exposed for
// tests; production code in the redesign package navigates fields via the
// form's keymap.
func (f *FormModel) Cursor() int {
	return f.cursor
}

// Submitted reports whether the form's submit path has fired. After
// submission the form ignores further input.
func (f *FormModel) Submitted() bool {
	return f.submitted
}

// Field returns a copy of the i-th field, for tests and renderers that
// only need to read state. Panics on an out-of-bounds index, matching
// Go's standard slice-access behavior.
func (f *FormModel) Field(i int) FormField {
	return f.fields[i]
}

// FieldCount returns the number of fields on the form.
func (f *FormModel) FieldCount() int {
	return len(f.fields)
}

// NewFormModel constructs the form. The first field receives focus.
func NewFormModel(c *Component, fields []FormField) *FormModel {
	for i := range fields {
		ti := textinput.New()
		ti.Placeholder = fields[i].Placeholder
		ti.SetValue(fields[i].Initial)
		// The form renders a `value: ` label in front of every widget, so
		// the textinput's own `> ` prompt would be a second cursor marker
		// on the same line. Leave the input prompt empty and let the
		// label do the announcing.
		ti.Prompt = ""

		fields[i].Input = ti
	}

	filter := textinput.New()
	filter.Prompt = "/"

	p := paginator.New()
	p.Type = paginator.Dots
	p.ActiveDot = formPaginationActiveStyle.Render(formPaginationBullet)
	p.InactiveDot = formPaginationDotStyle.Render(formPaginationBullet)

	return &FormModel{
		component:   c,
		fields:      fields,
		navKeys:     newNavigateKeyMap(),
		editKeys:    newEditKeyMap(),
		mode:        navigateMode,
		filter:      filterInactive,
		filterInput: filter,
		help:        help.New(),
		paginator:   p,
	}
}

func newNavigateKeyMap() navigateKeyMap {
	return navigateKeyMap{
		Next: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "next"),
		),
		Prev: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "prev"),
		),
		PrevPage: key.NewBinding(
			key.WithKeys("h", "left", "pgup", "alt+v"),
			key.WithHelp("h/←", "prev page"),
		),
		NextPage: key.NewBinding(
			key.WithKeys("l", "right", "pgdown", "ctrl+v"),
			key.WithHelp("l/→", "next page"),
		),
		GoToStart: key.NewBinding(
			key.WithKeys("home", "ctrl+a"),
			key.WithHelp("home", "first"),
		),
		GoToEnd: key.NewBinding(
			key.WithKeys("end", "ctrl+e"),
			key.WithHelp("end", "last"),
		),
		Interact: key.NewBinding(
			key.WithKeys("enter", "i"),
			key.WithHelp("enter", "edit/toggle"),
		),
		Unset: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "use default"),
		),
		UnsetAll: key.NewBinding(
			key.WithKeys("X"),
			key.WithHelp("X", "use default (all)"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		SubmitChecked: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "scaffold"),
		),
		Submit: key.NewBinding(
			key.WithKeys("S", "ctrl+d"),
			key.WithHelp("S", "scaffold (skip checks)"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

func newEditKeyMap() editKeyMap {
	return editKeyMap{
		ExitEdit: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "done"),
		),
		Submit: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "finish"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "toggle"),
		),
	}
}

// FieldsFromParsedVariables maps scaffold's variable representation onto
// the form's field shape. Required precedes optional so the user clears
// must-fills before deciding which optional defaults to override. String
// fields are flagged literal so the user enters a plain value (no quotes
// needed) and the form wraps it correctly at submit time.
func FieldsFromParsedVariables(required, optional []*config.ParsedVariable) []FormField {
	fields := make([]FormField, 0, len(required)+len(optional))

	for _, v := range required {
		fields = append(fields, newParsedVariableField(v, true))
	}

	for _, v := range optional {
		fields = append(fields, newParsedVariableField(v, false))
	}

	return fields
}

// newParsedVariableField constructs a single field from a ParsedVariable.
//
// String-typed variables become literal-mode: the user types the raw
// value (no surrounding quotes), the form skips HCL validation, and
// values() wraps the input with strconv.Quote on submit. Bool-typed
// variables become checkbox-mode: the textinput is replaced by a togglable
// `[x] true` / `[ ] false` widget seeded from the parsed default. All
// other types stay raw HCL. Set defaults to false on every field so the
// generated file only carries lines the user explicitly opts in to.
func newParsedVariableField(v *config.ParsedVariable, required bool) FormField {
	isString := v.Type == "string"
	isBool := v.Type == "bool"

	f := FormField{
		Name:        v.Name,
		Description: v.Description,
		TypeStr:     v.Type,
		Placeholder: v.DefaultValuePlaceholder,
		Required:    required,
		Literal:     isString,
		Checkbox:    isBool,
	}

	if !required {
		f.Initial = v.DefaultValue
	}

	if isString {
		f.Placeholder = ""

		if !required {
			f.Initial = decodeStringDefault(v.DefaultValue)
		}
	}

	if isBool {
		boolDefault := parseBoolDefault(v.DefaultValue)
		f.Bool = boolDefault
		f.BoolInitial = boolDefault
	}

	return f
}

// parseBoolDefault maps a ParsedVariable.DefaultValue ("true", "false", or
// "") to the checkbox's initial value. A bool without a parsed default
// falls back to false, matching Go's zero value and the most common
// "no, don't enable this" semantics for terraform booleans.
func parseBoolDefault(raw string) bool {
	return raw == "true"
}

// FieldsFromValuesReferences maps Unit/Stack `values.*` discovery onto the
// form's field shape. Required entries come first; optional entries are
// seeded with the HCL-formatted try() fallback. Optionals whose fallback
// is a known string default are flagged literal so the user edits the raw
// value the same way they would for a module string variable.
func FieldsFromValuesReferences(refs ValuesReferences) []FormField {
	fields := make([]FormField, 0, len(refs.Required)+len(refs.Optional))

	for _, name := range refs.Required {
		fields = append(fields, FormField{
			Name:     name,
			Required: true,
			TypeStr:  "any",
		})
	}

	for _, o := range refs.Optional {
		fields = append(fields, newValuesField(o))
	}

	return fields
}

// newValuesField builds a form field for an optional unit/stack values.*
// reference. Known-string defaults become literal-mode (the user edits the
// raw value); known-bool defaults become checkbox-mode; everything else
// stays raw HCL with the default pre-formatted.
func newValuesField(o OptionalValue) FormField {
	switch o.Default.Type() {
	case cty.String:
		return FormField{
			Name:    o.Name,
			TypeStr: "string",
			Initial: o.Default.AsString(),
			Literal: true,
		}
	case cty.Bool:
		return FormField{
			Name:        o.Name,
			TypeStr:     "bool",
			Checkbox:    true,
			Bool:        o.Default.True(),
			BoolInitial: o.Default.True(),
		}
	}

	return FormField{
		Name:    o.Name,
		TypeStr: "any",
		Initial: CtyValueAsHCL(o.Default),
	}
}

// decodeStringDefault unwraps the JSON-encoded form of a ParsedVariable's
// DefaultValue when it represents a string. The variable parser
// JSON-marshals defaults (so a default of `"prod"` arrives as the
// six-byte string `"prod"`); literal-mode fields show the unwrapped value
// in the input box. Falls back to raw on parse failure so an unexpected
// shape doesn't lose data.
func decodeStringDefault(raw string) string {
	if raw == "" {
		return ""
	}

	var out string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return raw
	}

	return out
}

// CtyValueAsHCL renders v as the HCL fragment hclwrite would emit, so the
// optional defaults pre-fill in a shape the user can edit or accept. Falls
// back to "" when serialization yields nothing useful.
func CtyValueAsHCL(v cty.Value) string {
	file := hclwrite.NewEmptyFile()
	file.Body().SetAttributeValue("v", v)

	src := string(file.Bytes())

	_, rhs, ok := strings.Cut(src, "=")
	if !ok {
		return ""
	}

	return strings.TrimSpace(strings.TrimSuffix(rhs, "\n"))
}

// values collects each field's input into a name->raw-HCL map. Empty
// trimmed inputs from an unset field are skipped so the source's default
// (or the TODO placeholder for required vars) is preserved verbatim in
// the generated file. Set fields emit their value: checkboxes as
// "true"/"false", literal strings via strconv.Quote, everything else
// verbatim.
func (f *FormModel) values() map[string]string {
	out := map[string]string{}

	for i := range f.fields {
		if !f.fields[i].Set {
			continue
		}

		if f.fields[i].Checkbox {
			out[f.fields[i].Name] = strconv.FormatBool(f.fields[i].Bool)
			continue
		}

		val := strings.TrimSpace(f.fields[i].Input.Value())
		if val == "" {
			continue
		}

		if f.fields[i].Literal {
			out[f.fields[i].Name] = strconv.Quote(val)
			continue
		}

		out[f.fields[i].Name] = val
	}

	return out
}

// SetSize lets the outer model push viewport dimensions into the form.
// The viewport's content is rebuilt on every View() call, so size changes
// take effect on the next frame.
func (f *FormModel) SetSize(w, h int) {
	f.width = w
	f.height = h

	const (
		inputPadding  = 6
		minInputWidth = 10
	)

	inputWidth := max(w-inputPadding, minInputWidth)

	for i := range f.fields {
		f.fields[i].Input.SetWidth(inputWidth)
	}

	f.filterInput.SetWidth(inputWidth)

	f.help.SetWidth(w)

	f.syncLayout()
}

// computeBodyHeight derives the rows available for field cards from the
// current height minus the chrome. The chrome counts have to mirror the
// row list View() builds, including whether the optional filter line is
// actually included (lipgloss.Height("") is 1, so naive subtraction
// over-reserves a line).
func (f *FormModel) computeBodyHeight() {
	if f.height == 0 {
		return
	}

	// reservedRows counts the literal blank rows in View()'s row list:
	// one top blank for breathing room and one between the header and
	// the body. Pagination and hint are counted via their rendered
	// heights below; filter is only included when it'll actually appear.
	const reservedRows = 2

	filterHeight := 0
	if filterLine := f.renderFilterLine(); filterLine != "" {
		filterHeight = lipgloss.Height(filterLine)
	}

	used := reservedRows +
		lipgloss.Height(f.renderHeader()) +
		filterHeight +
		1 + // pagination row (always reserved, blank when one page)
		lipgloss.Height(f.renderHint())

	f.bodyHeight = max(f.height-used, 1)
}

// setCursor moves the cursor to field i, clamping to the valid range
// when j/k push past either end. Assumes len(f.fields) > 0; callers
// (updateNavigate, submit) route through other checks before reaching
// here.
func (f *FormModel) setCursor(i int) {
	if i < 0 {
		i = 0
	}

	if i >= len(f.fields) {
		i = len(f.fields) - 1
	}

	f.cursor = i
}

// visibleIndices returns the indices the cursor is allowed to land on.
// With no filter the cursor walks every field; with the filter active
// (typing or applied) it walks only matching fields, so j/k skips over
// non-matches.
func (f *FormModel) visibleIndices() []int {
	if f.filter == filterInactive {
		return f.allIndices()
	}

	query := strings.ToLower(strings.TrimSpace(f.filterInput.Value()))
	if query == "" {
		return f.allIndices()
	}

	matches := make([]int, 0, len(f.fields))

	for i := range f.fields {
		if strings.Contains(strings.ToLower(f.fields[i].Name), query) {
			matches = append(matches, i)
		}
	}

	return matches
}

// renderIndices returns the indices renderBody should emit. With the
// filter open but no query typed yet, every field is rendered (dimmed,
// so the user sees the full inventory before they narrow it). Once any
// query character is typed, the body collapses to matching fields only,
// matching the list view's "all dim then narrow" filter UX.
func (f *FormModel) renderIndices() []int {
	if f.filter == filterInactive {
		return f.allIndices()
	}

	if f.filterQuery() == "" {
		return f.allIndices()
	}

	return f.visibleIndices()
}

// filterQuery returns the trimmed, lowercased query string when the
// filter is active; empty otherwise. Centralizes the read so renderField,
// visibleIndices, and the highlight logic all stay in sync.
func (f *FormModel) filterQuery() string {
	if f.filter == filterInactive {
		return ""
	}

	return strings.ToLower(strings.TrimSpace(f.filterInput.Value()))
}

// allIndices returns 0..len(fields)-1 as a slice. Extracted so the
// "no filter" branch of visibleIndices reads at the same level as the
// filtered branch.
func (f *FormModel) allIndices() []int {
	out := make([]int, len(f.fields))
	for i := range out {
		out[i] = i
	}

	return out
}

// cursorVisiblePos finds where the cursor sits within visibleIndices.
// When the cursor points at a field the filter hides, the helper returns
// the position of the closest visible field at or after the cursor (or
// the last visible field if the cursor is past the tail).
func (f *FormModel) cursorVisiblePos(visible []int) int {
	for i, idx := range visible {
		if idx >= f.cursor {
			return i
		}
	}

	return len(visible) - 1
}

// moveCursor walks the cursor delta positions through visibleIndices,
// clamping at either end. Used by j/k (delta ±1) and h/l/pgup/pgdn
// (delta ±pageSize).
func (f *FormModel) moveCursor(delta int) {
	visible := f.visibleIndices()
	if len(visible) == 0 {
		return
	}

	pos := max(f.cursorVisiblePos(visible)+delta, 0)
	if pos >= len(visible) {
		pos = len(visible) - 1
	}

	f.cursor = visible[pos]
}

// jumpCursor moves the cursor to the first or last visible field. Used by
// home/end and ctrl+a/ctrl+e.
func (f *FormModel) jumpCursor(toEnd bool) {
	visible := f.visibleIndices()
	if len(visible) == 0 {
		return
	}

	if toEnd {
		f.cursor = visible[len(visible)-1]
		return
	}

	f.cursor = visible[0]
}

// nextPage advances the cursor (and pageStart) to the first field of the
// next page. No-op when already on the last page.
func (f *FormModel) nextPage() {
	rendered := f.renderIndices()
	if len(rendered) == 0 {
		return
	}

	end := f.pageEndFromStart(f.pageStart, rendered)
	if end >= len(rendered) {
		return
	}

	f.pageStart = end
	f.cursor = rendered[end]
}

// prevPage moves the cursor (and pageStart) to the first field of the
// preceding page. No-op when already on the first page.
func (f *FormModel) prevPage() {
	rendered := f.renderIndices()
	if len(rendered) == 0 || f.pageStart == 0 {
		return
	}

	f.pageStart = f.prevPageStart(f.pageStart, rendered)
	f.cursor = rendered[f.pageStart]
}

// enterEdit transitions navigate to edit on the focused field. For text
// fields the input is focused and its current value snapshotted so
// exitEdit can decide whether the user actually changed anything; an
// unset optional with a default is also seeded with that default so the
// user has a sensible starting point. Bool fields just flip the mode flag
// (there's no textinput to focus); subsequent enters toggle the value.
// Callers (interact) guarantee len(f.fields) > 0.
func (f *FormModel) enterEdit() tea.Cmd {
	f.mode = editMode
	fld := &f.fields[f.cursor]

	if fld.Checkbox {
		return nil
	}

	if !fld.Set && fld.Initial != "" && fld.Input.Value() == "" {
		fld.Input.SetValue(fld.Initial)
	}

	f.editPreEdit = fld.Input.Value()
	f.refreshValidationErr(f.cursor)

	return fld.Input.Focus()
}

// exitEdit transitions edit to navigate. For text fields, if the input
// value changed during the edit session the field is marked Set so
// values() emits it. Bool fields don't track edit-session changes (each
// toggle commits via toggleBool), so we only reset the mode flag here.
// Only called after a successful enterEdit, so len(f.fields) > 0 is
// guaranteed.
func (f *FormModel) exitEdit() {
	fld := &f.fields[f.cursor]

	if !fld.Checkbox {
		if fld.Input.Value() != f.editPreEdit {
			fld.Set = true
		}

		fld.Input.Blur()
	}

	f.editPreEdit = ""
	f.mode = navigateMode
}

// validateField parses the current input as an HCL expression and, when
// the expression evaluates to a known cty value, checks that its type
// family matches the variable's declared type. An empty input is treated
// as "not supplied" and reports no error. Literal-mode fields skip
// validation because the form quotes the value at submit time; checkbox
// fields skip too because their state is always a well-formed bool.
//
// Expressions that can't be evaluated without context (references like
// `local.x`) are accepted as long as they parse: the form has no way to
// resolve them and the user may know what they're doing.
func (f *FormModel) validateField(i int) error {
	fld := &f.fields[i]
	if fld.Checkbox || fld.Literal {
		return nil
	}

	val := strings.TrimSpace(fld.Input.Value())
	if val == "" {
		return nil
	}

	expr, diags := hclsyntax.ParseExpression([]byte(val), "form_input.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return errors.New(firstDiagMessage(diags))
	}

	v, evalDiags := expr.Value(nil)
	if evalDiags.HasErrors() {
		// Multi-step traversals (`local.x`, `var.y`), function calls,
		// and the like can't evaluate without a context; terragrunt
		// resolves them at run time, so accept the syntactically-valid
		// HCL. A bare single-word identifier, on the other hand, is
		// almost always a typo, so flag it against the declared type.
		if isBareIdentifier(expr) {
			return fmt.Errorf("not a valid %s", fld.TypeStr)
		}

		return nil
	}

	if !typeMatches(v.Type(), fld.TypeStr) {
		return fmt.Errorf("expected %s, got %s", fld.TypeStr, v.Type().FriendlyName())
	}

	return nil
}

// isBareIdentifier reports whether expr is a single-step variable
// reference like `asdlfkj` (as opposed to `local.x`, `var.y`, or a
// function call). A bare identifier with no surrounding HCL syntax is a
// reference to a name that, in any realistic terragrunt context, has to
// be qualified (`local.`, `var.`, `module.`, ...), so it's almost always
// a typo the user didn't realize they made.
func isBareIdentifier(expr hcl.Expression) bool {
	trav, ok := expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok {
		return false
	}

	return len(trav.Traversal) == 1
}

// firstDiagMessage returns the first error diagnostic's headline plus
// detail, with the file/position prefix stripped. HCL's default Error()
// output leads with a synthetic filename and column range that don't
// help a user editing a single field, but the summary alone is often
// too terse ("missing expression"), so the detail sentence is appended
// when present to give the user something actionable.
func firstDiagMessage(diags hcl.Diagnostics) string {
	for _, d := range diags {
		if d.Severity != hcl.DiagError || d.Summary == "" {
			continue
		}

		msg := strings.ToLower(d.Summary[:1]) + d.Summary[1:]
		if d.Detail != "" {
			msg += ": " + d.Detail
		}

		return msg
	}

	return "invalid HCL"
}

// typeMatches reports whether actual is in the same family as the
// variable's declared type. Inner types are not checked; the form
// trusts hclwrite-style structural correctness and lets the OpenTofu /
// Terraform plan surface deeper mismatches.
func typeMatches(actual cty.Type, declared string) bool {
	switch declared {
	case "number":
		return actual.Equals(cty.Number)
	case "bool":
		return actual.Equals(cty.Bool)
	case "string":
		return actual.Equals(cty.String)
	case "list", "set", "tuple":
		return actual.IsListType() || actual.IsSetType() || actual.IsTupleType()
	case "map", "object":
		return actual.IsMapType() || actual.IsObjectType()
	}

	return true
}

// validateAll runs validateField on each populated input and returns the
// index of the first invalid field, or -1 when everything parses.
func (f *FormModel) validateAll() int {
	bad := -1

	for i := range f.fields {
		err := f.validateField(i)
		if err == nil {
			f.fields[i].ValidationErr = ""
			continue
		}

		f.fields[i].ValidationErr = err.Error()

		if bad < 0 {
			bad = i
		}
	}

	return bad
}

// submit validates every set field and, on success, marks the form as
// submitted and emits a FormSubmitMsg. If validation fails the cursor
// jumps to the first bad field and no message is emitted. Submit can fire
// from either mode; if the user is in edit mode, exitEdit runs first so
// pending changes are captured before validation.
func (f *FormModel) submit() (*FormModel, tea.Cmd) {
	if f.mode == editMode {
		f.exitEdit()
	}

	bad := f.validateAll()
	if bad >= 0 {
		f.setCursor(bad)
		return f, nil
	}

	f.submitted = true
	vals := f.values()

	return f, func() tea.Msg { return FormSubmitMsg{Values: vals} }
}

// submitChecked is the same as submit but additionally refuses to submit
// while any required field is unset, jumping the cursor to the first
// missing one. Use this for the user-facing "s" shortcut so the form
// guides the user back to incomplete required values; the force-submit
// path (S, ctrl+d) bypasses this and falls back to TODO placeholders.
func (f *FormModel) submitChecked() (*FormModel, tea.Cmd) {
	if f.mode == editMode {
		f.exitEdit()
	}

	if missing := f.firstMissingRequired(); missing >= 0 {
		f.setCursor(missing)
		return f, nil
	}

	return f.submit()
}

// firstMissingRequired marks each unset required field with a "required
// value missing" validation error and returns the index of the first such
// field, or -1 when every required field has been set.
func (f *FormModel) firstMissingRequired() int {
	missing := -1

	for i := range f.fields {
		fld := &f.fields[i]
		if !fld.Required || fld.Set {
			continue
		}

		fld.ValidationErr = "required value missing"

		if missing < 0 {
			missing = i
		}
	}

	return missing
}

// Update handles a single tea.Msg and returns the (possibly mutated) form
// plus any command to fire. The dispatcher splits on mode: navigate mode
// consumes keypresses for movement, mode entry, and set toggling, while
// edit mode forwards keypresses to the focused textinput except for the
// few bindings that switch the mode or submit the form.
func (f *FormModel) Update(msg tea.Msg) (*FormModel, tea.Cmd) {
	if f.submitted {
		return f, nil
	}

	next, cmd := f.dispatch(msg)
	next.syncLayout()

	return next, cmd
}

// dispatch routes an incoming message to the mode-appropriate handler.
func (f *FormModel) dispatch(msg tea.Msg) (*FormModel, tea.Cmd) {
	if f.mode == editMode {
		return f.updateEdit(msg)
	}

	return f.updateNavigate(msg)
}

// syncLayout recomputes derived layout state (bodyHeight, pageStart, and
// paginator position) after an Update. Centralizing the mutation here keeps
// View pure.
func (f *FormModel) syncLayout() {
	f.computeBodyHeight()
	f.ensureCursorOnPage()
	f.syncPaginator()
}

// syncPaginator recomputes the paginator's total page count and current
// page from the current pageStart. Called from syncLayout so View can stay
// read-only.
func (f *FormModel) syncPaginator() {
	rendered := f.renderIndices()
	if len(rendered) == 0 {
		f.paginator.TotalPages = 1
		f.paginator.Page = 0

		return
	}

	starts := f.computePageStarts(rendered)
	if len(starts) == 0 {
		f.paginator.TotalPages = 1
		f.paginator.Page = 0

		return
	}

	curPage := len(starts) - 1

	for i, start := range starts {
		if f.pageStart < start {
			curPage = i - 1

			break
		}
	}

	f.paginator.TotalPages = len(starts)
	f.paginator.Page = curPage
}

// updateNavigate handles keypresses while the form is in navigate mode.
// j/k (and arrow keys) move one field; h/l (pgup/pgdn) jump a page; home/
// end go to the first/last field. enter on a text field opens edit mode;
// enter on a checkbox toggles its value. x marks an optional field "use
// default"; X applies that to every optional. `/` opens the filter input.
// ctrl+d submits; esc cancels (clearing an applied filter first).
func (f *FormModel) updateNavigate(msg tea.Msg) (*FormModel, tea.Cmd) {
	if f.filter == filterTyping {
		return f.updateFilterTyping(msg)
	}

	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return f, nil
	}

	switch {
	case key.Matches(keyMsg, f.navKeys.Cancel):
		if f.filter == filterApplied {
			f.clearFilter()
			return f, nil
		}

		return f, func() tea.Msg { return FormCancelMsg{} }
	case key.Matches(keyMsg, f.navKeys.SubmitChecked):
		return f.submitChecked()
	case key.Matches(keyMsg, f.navKeys.Submit):
		return f.submit()
	case key.Matches(keyMsg, f.navKeys.Filter):
		return f, f.beginFilter()
	}

	// The remaining bindings all operate on the focused field; on an
	// empty form there is no field to interact with.
	if len(f.fields) == 0 {
		return f, nil
	}

	switch {
	case key.Matches(keyMsg, f.navKeys.Next):
		f.moveCursor(1)
	case key.Matches(keyMsg, f.navKeys.Prev):
		f.moveCursor(-1)
	case key.Matches(keyMsg, f.navKeys.NextPage):
		f.nextPage()
	case key.Matches(keyMsg, f.navKeys.PrevPage):
		f.prevPage()
	case key.Matches(keyMsg, f.navKeys.GoToStart):
		f.jumpCursor(false)
	case key.Matches(keyMsg, f.navKeys.GoToEnd):
		f.jumpCursor(true)
	case key.Matches(keyMsg, f.navKeys.Interact):
		return f.interact()
	case key.Matches(keyMsg, f.navKeys.UnsetAll):
		f.unsetAllFields()
	case key.Matches(keyMsg, f.navKeys.Unset):
		f.unsetField(f.cursor)
	}

	return f, nil
}

// updateFilterTyping handles keypresses while the user is typing into the
// filter input. enter commits the filter, esc cancels (back to inactive),
// everything else forwards to the textinput so the user can edit the
// query character by character.
func (f *FormModel) updateFilterTyping(msg tea.Msg) (*FormModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, f.editKeys.ExitEdit):
			f.clearFilter()
			return f, nil
		case key.Matches(keyMsg, f.navKeys.Interact):
			f.applyFilter()
			return f, nil
		}
	}

	ti, cmd := f.filterInput.Update(msg)
	f.filterInput = ti

	// Keep the cursor on a matching field while the user types so the
	// "currently focused" highlight tracks the filter live.
	f.snapCursorToVisible()

	return f, cmd
}

// beginFilter switches the form into filterTyping. The returned tea.Cmd
// drives the textinput's cursor blink and must reach the Bubble Tea loop.
func (f *FormModel) beginFilter() tea.Cmd {
	f.filter = filterTyping
	return f.filterInput.Focus()
}

// applyFilter commits the typed query. The form returns to plain navigate
// mode with the filter active; the cursor snaps onto the first visible
// match if it's currently on a hidden field.
func (f *FormModel) applyFilter() {
	f.filterInput.Blur()

	if strings.TrimSpace(f.filterInput.Value()) == "" {
		f.filter = filterInactive
		return
	}

	f.filter = filterApplied
	f.snapCursorToVisible()
}

// clearFilter cancels any in-progress or committed filter and restores
// the cursor to a position that's visible without filtering.
func (f *FormModel) clearFilter() {
	f.filterInput.Blur()
	f.filterInput.SetValue("")
	f.filter = filterInactive
}

// snapCursorToVisible ensures the cursor points at a field that the
// current filter would render. Called after every filter-query change so
// the focused-field highlight stays aligned with what the user sees.
func (f *FormModel) snapCursorToVisible() {
	visible := f.visibleIndices()
	if len(visible) == 0 {
		return
	}

	if slices.Contains(visible, f.cursor) {
		return
	}

	f.cursor = visible[0]
}

// updateEdit handles keypresses while the form is in edit mode. esc
// returns to navigate mode (committing any change as Set=true on text
// fields); ctrl+d submits the form after a forced edit-to-navigate
// transition. On bool fields, enter toggles the value in place so the
// user can flip it as many times as they like before pressing esc.
// Everything else on a text field is forwarded to the focused textinput.
func (f *FormModel) updateEdit(msg tea.Msg) (*FormModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, f.editKeys.ExitEdit):
			f.exitEdit()
			return f, nil
		case key.Matches(keyMsg, f.editKeys.Submit):
			return f.submit()
		}

		if f.fields[f.cursor].Checkbox && key.Matches(keyMsg, f.editKeys.Toggle) {
			f.toggleBool(f.cursor)
			return f, nil
		}
	}

	// Bool fields have no textinput to feed; ignore non-key messages and
	// any keys that didn't match the bindings above.
	if f.fields[f.cursor].Checkbox {
		return f, nil
	}

	ti, cmd := f.fields[f.cursor].Input.Update(msg)
	f.fields[f.cursor].Input = ti
	f.refreshValidationErr(f.cursor)

	return f, cmd
}

// refreshValidationErr re-runs validateField on the i-th field and stores
// the result. Called after every keystroke in edit mode (so the cursor
// bar turns red the moment the value goes off-type) and on enterEdit (so
// a previously-broken value surfaces immediately).
func (f *FormModel) refreshValidationErr(i int) {
	if err := f.validateField(i); err != nil {
		f.fields[i].ValidationErr = err.Error()
		return
	}

	f.fields[i].ValidationErr = ""
}

// interact resolves an `enter` keypress against the focused field. Every
// field type transitions into edit mode; the difference is what edit mode
// does on subsequent keypresses (typing into a textinput, or toggling a
// bool with enter). Empty forms have no field to interact with.
func (f *FormModel) interact() (*FormModel, tea.Cmd) {
	if len(f.fields) == 0 {
		return f, nil
	}

	return f, f.enterEdit()
}

// unsetField marks the focused optional field as "use default" so it is
// left out of the generated file. The call is a no-op on required fields
// (the user owes the form a value either way) and on fields that are
// already unset (no toggling: x only unsets). The field's Input and Bool
// stay as the user left them, so re-entering edit picks up where they
// last were instead of jumping back to a blank slate.
//
// updateNavigate guarantees a valid cursor before calling, so there's no
// bounds check here.
func (f *FormModel) unsetField(i int) {
	fld := &f.fields[i]
	if fld.Required || !fld.Set {
		return
	}

	fld.Set = false
	fld.ValidationErr = ""
}

// unsetAllFields marks every optional Set field as "use default" in one
// pass. Required fields and already-unset optionals are skipped. Input
// and Bool state is preserved so the user can recover prior edits by
// re-entering edit mode.
func (f *FormModel) unsetAllFields() {
	for i := range f.fields {
		fld := &f.fields[i]
		if fld.Required || !fld.Set {
			continue
		}

		fld.Set = false
		fld.ValidationErr = ""
	}
}

// toggleBool flips a checkbox field's value between true and false and
// marks it Set, so the choice ends up in the generated file regardless of
// whether it matches the source default. Callers (interact, updateNavigate)
// guarantee a valid cursor before calling.
func (f *FormModel) toggleBool(i int) {
	f.fields[i].Bool = !f.fields[i].Bool
	f.fields[i].Set = true
}

// Form styling. These mirror the surrounding pager/list look so the form
// feels native to the catalog TUI rather than a separate widget.
var (
	formTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A8ACB1")).
			Background(lipgloss.Color("#1D252F")).
			Padding(0, 1).
			Bold(true)

	formFieldNameStyle = lipgloss.NewStyle().Bold(true)

	// Cursor styles for the focused-field vertical bar and the field name.
	// Navigate uses the same cyan as the list view's selected-item bar so
	// the two screens share a visual language. Edit swaps to yellow so the
	// mode change is unmistakable.
	formNavCursorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#63C5DA"))

	formNavCursorBoldStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#63C5DA")).
				Bold(true)

	formEditCursorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F1FA8C"))

	formEditCursorBoldStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F1FA8C")).
				Bold(true)

	formFieldRequiredTag = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF5555")).
				Render(" required")

	formFieldOptionalTag = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A8ACB1")).
				Render(" optional")

	formMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A8ACB1"))

	// One color per HCL type family so the user can scan a long form and
	// pick out strings vs bools vs collections without reading each line.
	// Fallback purple covers `any` (unit/stack required fields, where the
	// type isn't known at discovery time) and anything we don't classify.
	formTypeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#BD93F9"))

	formTypeStringStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#50FA7B"))

	formTypeNumberStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F1FA8C"))

	formTypeBoolStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8BE9FD"))

	formTypeCollectionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFB86C"))

	formTypeStructStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF79C6"))

	formDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A8ACB1")).
			Italic(true)

	formErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555"))

	formErrorBoldStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF5555")).
				Bold(true)

	// Set bool values get a clear positive/negative color so the user
	// can see at a glance which checkboxes they flipped.
	formBoolTrueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#50FA7B")).
				Bold(true)

	formBoolFalseStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF5555")).
				Bold(true)

	formDefaultHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A8ACB1")).
				Italic(true)

	// Dim style for fields that don't match the current filter query
	// while the user is still typing. Stronger Faint than the muted
	// gray so the eye glides past dimmed rows.
	formDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3C3F45"))

	// Pagination dots: colors mirror the bubbles list defaults so the
	// dot row matches the list view's footer character-for-character.
	formPaginationActiveStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#979797"))

	formPaginationDotStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#3C3C3C"))

	// The list view pads its pagination row with PaddingLeft(2); match
	// it so the dots sit at the same column.
	formPaginationLayoutStyle = lipgloss.NewStyle().PaddingLeft(formPaginationLeftPad)
)

const (
	formPaginationBullet  = "•"
	formPaginationLeftPad = 2
)

// typeStyle picks the color used to render an HCL type label. Primitives
// get distinct shades; collections (list, set, tuple) and structures
// (map, object) share a color each so the user can tell at a glance which
// family they're dealing with. Anything else (including unit/stack `any`
// fields where the type isn't known at discovery time) falls back to the
// generic purple.
func typeStyle(typeStr string) lipgloss.Style {
	switch {
	case typeStr == "string":
		return formTypeStringStyle
	case typeStr == "number":
		return formTypeNumberStyle
	case typeStr == "bool":
		return formTypeBoolStyle
	case strings.HasPrefix(typeStr, "list"),
		strings.HasPrefix(typeStr, "set"),
		strings.HasPrefix(typeStr, "tuple"):
		return formTypeCollectionStyle
	case strings.HasPrefix(typeStr, "map"),
		strings.HasPrefix(typeStr, "object"):
		return formTypeStructStyle
	}

	return formTypeStyle
}

// renderCheckbox produces the visual for a Set bool-mode field. True
// renders green and false red so the user can scan the form and see the
// set choices without re-reading each field's value.
func renderCheckbox(checked bool) string {
	if checked {
		return formBoolTrueStyle.Render("[x] true")
	}

	return formBoolFalseStyle.Render("[ ] false")
}

// View renders the form. When the rendered body exceeds the available
// height the viewport scrolls it; the cursor is kept on-screen by
// adjusting the y-offset to track the focused field. A blank top row
// mirrors the list view's breathing room above the tab bar.
func (f *FormModel) View() string {
	header := f.renderHeader()
	filterLine := f.renderFilterLine()
	hint := f.renderHint()
	pagination := f.renderPagination()
	body := padToHeight(f.renderBody(), f.bodyHeight)

	// Lay out: blank top, header, blank, [filter], body, pagination, hint.
	// filterLine is optional and only takes a row when shown; pagination
	// always claims its row (blank when there's only one page).
	rows := []string{"", header, ""}
	if filterLine != "" {
		rows = append(rows, filterLine)
	}

	rows = append(rows, body, pagination, hint)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderHeader is the title strip identifying the component being
// scaffolded. It mirrors the pager title styling.
func (f *FormModel) renderHeader() string {
	if f.component == nil {
		return formTitleStyle.Render("Scaffold")
	}

	return formTitleStyle.Render("Scaffold: " + f.component.Title())
}

// renderHint is the bottom keybinding strip. It uses bubbles' help.Model
// so the styling, dimness, and `key desc • key desc` bullet layout match
// the list and pager views' help bars exactly. The set of bindings
// surfaced depends on mode and filter state.
func (f *FormModel) renderHint() string {
	return f.help.View(formHelpKeyMap{bindings: f.hintBindings()})
}

// hintBindings returns the bindings visible in the hint bar for the
// current form state. Edit mode shows the few keys that get the user
// back out; the filter-typing state shows just enter/esc; navigate mode
// shows the full nav + action set.
func (f *FormModel) hintBindings() []key.Binding {
	if f.mode == editMode {
		bindings := []key.Binding{f.editKeys.ExitEdit}

		if f.cursor >= 0 && f.cursor < len(f.fields) && f.fields[f.cursor].Checkbox {
			bindings = append(bindings, f.editKeys.Toggle)
		}

		return bindings
	}

	if f.filter == filterTyping {
		return []key.Binding{
			f.navKeys.Interact, // enter: apply filter
			f.editKeys.ExitEdit,
		}
	}

	cancel := f.navKeys.Cancel
	if f.filter == filterApplied {
		cancel = key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "clear filter"),
		)
	}

	return []key.Binding{
		f.navKeys.Next,
		f.navKeys.Prev,
		f.navKeys.Filter,
		f.navKeys.Interact,
		f.navKeys.Unset,
		f.navKeys.UnsetAll,
		f.navKeys.SubmitChecked,
		f.navKeys.Submit,
		cancel,
	}
}

// formHelpKeyMap adapts a flat list of bindings to the help.KeyMap
// interface bubbles' help.Model expects.
type formHelpKeyMap struct {
	bindings []key.Binding
}

func (k formHelpKeyMap) ShortHelp() []key.Binding {
	return k.bindings
}

func (k formHelpKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.bindings}
}

// padToHeight pads s with trailing newlines so that lipgloss.Height
// reports at least h lines. Used to anchor the body to bodyHeight when a
// short page would otherwise leave the pagination dots and hint floating
// up the screen instead of sitting on the bottom row.
func padToHeight(s string, h int) string {
	cur := lipgloss.Height(s)
	if cur >= h {
		return s
	}

	return s + strings.Repeat("\n", h-cur)
}

// renderBody composes the current page of field cards. Only fields that
// fit entirely in bodyHeight are emitted; partially-clipped trailing
// fields move to the next page so the user never sees a half-rendered
// row at the bottom.
func (f *FormModel) renderBody() string {
	if len(f.fields) == 0 {
		return formMetaStyle.Render(
			"No variables to populate. Press ctrl+d to scaffold or esc to cancel.",
		)
	}

	indices := f.renderIndices()
	if len(indices) == 0 {
		return formMetaStyle.Render("No fields match the filter.")
	}

	pageEnd := f.pageEndFromStart(f.pageStart, indices)

	parts := make([]string, 0, pageEnd-f.pageStart)
	for i := f.pageStart; i < pageEnd; i++ {
		parts = append(parts, f.renderField(indices[i]))
	}

	return strings.Join(parts, "\n\n")
}

// pageEndFromStart walks rendered[start:] adding field heights (plus the
// one-line gap between them) until the next field would push past
// bodyHeight. Returns the index just past the last fully-fitting field,
// so callers can slice rendered[start:end]. A single oversized field is
// always shown (clipped) rather than producing an empty page.
func (f *FormModel) pageEndFromStart(start int, rendered []int) int {
	if start >= len(rendered) {
		return start
	}

	end := start
	used := 0

	for i := start; i < len(rendered); i++ {
		h := lipgloss.Height(f.renderField(rendered[i]))

		gap := 0
		if i > start {
			gap = 1
		}

		if used+gap+h > f.bodyHeight {
			if i == start {
				// First field on the page is taller than the viewport.
				// Render it anyway so the page is never empty.
				return i + 1
			}

			return i
		}

		used += gap + h
		end = i + 1
	}

	return end
}

// prevPageStart returns the pageStart for the page that immediately
// precedes curStart. It walks backwards from curStart-1 packing fields
// until adding another would exceed bodyHeight.
func (f *FormModel) prevPageStart(curStart int, rendered []int) int {
	if curStart <= 0 {
		return 0
	}

	end := curStart - 1
	used := lipgloss.Height(f.renderField(rendered[end]))
	start := end

	for i := end - 1; i >= 0; i-- {
		h := lipgloss.Height(f.renderField(rendered[i]))
		gap := 1

		if used+gap+h > f.bodyHeight {
			return start
		}

		used += gap + h
		start = i
	}

	return 0
}

// ensureCursorOnPage adjusts pageStart so the cursor falls within the
// currently visible page. Used after j/k or filter changes to re-page
// when the cursor moves beyond the current window.
func (f *FormModel) ensureCursorOnPage() {
	indices := f.renderIndices()
	if len(indices) == 0 {
		f.pageStart = 0
		return
	}

	cursorPos := f.cursorPosIn(indices)
	if cursorPos < 0 {
		f.pageStart = 0
		return
	}

	if f.pageStart > cursorPos {
		// Cursor moved above the current page; snap so the cursor sits
		// at the bottom of the previous page.
		f.pageStart = f.pageStartContaining(cursorPos, indices)
		return
	}

	end := f.pageEndFromStart(f.pageStart, indices)
	if cursorPos >= end {
		// Cursor moved past the current page; start a new page at the
		// cursor so it lands at the top of the next view.
		f.pageStart = cursorPos
	}
}

// pageStartContaining returns the largest pageStart such that target is
// still on the page (i.e., its position is within [pageStart, pageEnd)).
// Used by ensureCursorOnPage to snap a cursor that moved up into the
// previous page rather than re-anchoring it at the top.
func (f *FormModel) pageStartContaining(target int, rendered []int) int {
	used := lipgloss.Height(f.renderField(rendered[target]))
	start := target

	for i := target - 1; i >= 0; i-- {
		h := lipgloss.Height(f.renderField(rendered[i]))
		gap := 1

		if used+gap+h > f.bodyHeight {
			return start
		}

		used += gap + h
		start = i
	}

	return 0
}

// cursorPosIn returns the cursor's position within the rendered indices
// slice, or -1 when the cursor's field is hidden by the current filter.
func (f *FormModel) cursorPosIn(rendered []int) int {
	for i, idx := range rendered {
		if idx == f.cursor {
			return i
		}
	}

	return -1
}

// renderPagination produces the dotted page indicator that sits between
// the body and the hint, matching the bubbles list footer. Returns an
// empty string when every renderable field fits on a single page; the
// caller still reserves the row, so showing nothing keeps the layout
// stable across navigations.
func (f *FormModel) renderPagination() string {
	if f.paginator.TotalPages <= 1 {
		return ""
	}

	return formPaginationLayoutStyle.Render(f.paginator.View())
}

// computePageStarts walks the rendered slice page by page and records
// each pageStart. The returned slice always begins at 0; entries after
// the first mark the start of subsequent pages.
func (f *FormModel) computePageStarts(rendered []int) []int {
	if len(rendered) == 0 {
		return nil
	}

	starts := []int{0}
	cur := 0

	for cur < len(rendered) {
		next := f.pageEndFromStart(cur, rendered)
		if next <= cur {
			break
		}

		if next < len(rendered) {
			starts = append(starts, next)
		}

		cur = next
	}

	return starts
}

// renderFilterLine renders the filter row that sits between the header
// and the body. It shows the live input when the user is typing and the
// committed query when the filter is applied; nothing when the filter is
// inactive so the form stays clean for the common no-filter path.
func (f *FormModel) renderFilterLine() string {
	switch f.filter {
	case filterTyping:
		return "  " + f.filterInput.View()
	case filterApplied:
		return "  " + formMetaStyle.Render("filter: /"+f.filterInput.Value())
	case filterInactive:
	}

	return ""
}

// renderField composes one field card: name (with focus highlight + req
// tag), type meta, optional description, the value widget, and any
// validation error. The focused field gets a colored vertical bar running
// down its left edge, with the bar's color reflecting the current mode
// (cyan in navigate, yellow in edit). Unfocused fields are indented the
// same width so the rows line up.
func (f *FormModel) renderField(i int) string {
	// While the filter is open without a query, every field renders
	// dimmed so the user sees what's about to drop off once they start
	// typing.
	if f.filter == filterTyping && f.filterQuery() == "" {
		return f.renderDimmedField(i)
	}

	fld := &f.fields[i]
	focused := i == f.cursor

	prefix := "  "
	nameStyle := formFieldNameStyle

	if focused {
		prefix = f.cursorPrefix()
		nameStyle = f.cursorBoldStyle()
	}

	tag := formFieldOptionalTag
	if fld.Required {
		tag = formFieldRequiredTag
	}

	displayName := renderHighlightedName(fld.Name, f.filterQuery(), nameStyle)

	lines := []string{
		prefix + displayName + tag,
		prefix + formMetaStyle.Render("type: ") + typeStyle(fld.TypeStr).Render(fld.TypeStr),
	}

	if fld.Description != "" {
		lines = append(lines, prefixEveryLine(formDescStyle.Render(fld.Description), prefix))
	}

	lines = append(lines, prefix+formMetaStyle.Render("value: ")+f.renderFieldValue(fld, focused))

	if fld.ValidationErr != "" {
		lines = append(lines, prefix+formErrorStyle.Render(fld.ValidationErr))
	}

	return strings.Join(lines, "\n")
}

// cursorPrefix returns the two-character indent for a focused field's
// line: a colored vertical bar followed by a space. Unfocused fields use
// two spaces (`"  "`), so the leading column lines up across the form.
// The bar's color reflects the mode (cyan in navigate, yellow in edit)
// or flips to red when the focused field carries a validation error.
func (f *FormModel) cursorPrefix() string {
	return f.cursorPlainStyle().Render("│") + " "
}

// cursorBoldStyle returns the bold variant of the cursor color so the
// focused field's name color matches the bar.
func (f *FormModel) cursorBoldStyle() lipgloss.Style {
	if f.focusedHasError() {
		return formErrorBoldStyle
	}

	if f.mode == editMode {
		return formEditCursorBoldStyle
	}

	return formNavCursorBoldStyle
}

// cursorPlainStyle is the non-bold counterpart to cursorBoldStyle, used
// for the vertical bar character.
func (f *FormModel) cursorPlainStyle() lipgloss.Style {
	if f.focusedHasError() {
		return formErrorStyle
	}

	if f.mode == editMode {
		return formEditCursorStyle
	}

	return formNavCursorStyle
}

// focusedHasError reports whether the currently focused field's stored
// validation error should drive the cursor color. Unset fields are
// excluded in navigate mode because their value is irrelevant: the
// source default will apply at write time regardless of what's typed.
func (f *FormModel) focusedHasError() bool {
	if f.cursor < 0 || f.cursor >= len(f.fields) {
		return false
	}

	fld := f.fields[f.cursor]
	if fld.ValidationErr == "" {
		return false
	}

	if f.mode == navigateMode && !fld.Set {
		return false
	}

	return true
}

// prefixEveryLine prepends prefix to every line of s. Used to extend the
// focused field's cursor bar down through a multi-line description.
func prefixEveryLine(s, prefix string) string {
	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = prefix + parts[i]
	}

	return strings.Join(parts, "\n")
}

// renderHighlightedName renders name with baseStyle, with any matching
// substring underlined so the user sees which characters their filter
// query just landed on. Falls back to a plain styled render when there
// is no query or no match.
func renderHighlightedName(name, query string, baseStyle lipgloss.Style) string {
	if query == "" {
		return baseStyle.Render(name)
	}

	idx := strings.Index(strings.ToLower(name), query)
	if idx < 0 {
		return baseStyle.Render(name)
	}

	before := name[:idx]
	matched := name[idx : idx+len(query)]
	after := name[idx+len(query):]

	highlight := baseStyle.Underline(true)

	return baseStyle.Render(before) + highlight.Render(matched) + baseStyle.Render(after)
}

// renderDimmedField renders a field card in the dim style used while the
// filter is being typed. The card structure mirrors renderField exactly
// so dimmed and bright rows stay visually aligned; the inner colors are
// dropped and an outer dim style coats the result so the eye glides
// past hidden rows.
func (f *FormModel) renderDimmedField(i int) string {
	fld := &f.fields[i]

	tag := " optional"
	if fld.Required {
		tag = " required"
	}

	lines := []string{
		"  " + fld.Name + tag,
		"  type: " + fld.TypeStr,
	}

	if fld.Description != "" {
		lines = append(lines, prefixEveryLine(fld.Description, "  "))
	}

	lines = append(lines, "  value: "+f.renderFieldValuePlain(fld))

	if fld.ValidationErr != "" {
		lines = append(lines, "  "+fld.ValidationErr)
	}

	return formDimStyle.Render(strings.Join(lines, "\n"))
}

// renderFieldValuePlain returns the value widget as a plain (uncolored)
// string for use by renderDimmedField. Mirrors renderFieldValue's logic
// but skips every internal style so the outer dim style applies cleanly.
func (f *FormModel) renderFieldValuePlain(fld *FormField) string {
	if fld.Checkbox {
		if fld.Set {
			if fld.Bool {
				return "[x] true"
			}

			return "[ ] false"
		}

		if fld.Required {
			return "(unset)"
		}

		val := "false"
		if fld.BoolInitial {
			val = "true"
		}

		return "(default: " + val + ")"
	}

	if fld.Set {
		return fld.Input.Value()
	}

	if fld.Required {
		return "(unset)"
	}

	if fld.Initial == "" {
		return "(default)"
	}

	return "(default: " + fld.Initial + ")"
}

// renderFieldValue produces the value widget for a single field. Set
// fields render their live value; unset fields render a muted hint that
// distinguishes "no value yet" (required) from "use this default"
// (optional, with the default value shown so the user can decide whether
// to override).
func (f *FormModel) renderFieldValue(fld *FormField, focused bool) string {
	if fld.Checkbox {
		return renderCheckboxValue(fld)
	}

	if focused && f.mode == editMode {
		return fld.Input.View()
	}

	if fld.Set {
		return fld.Input.View()
	}

	return renderUnsetTextValue(fld)
}

// renderUnsetTextValue picks the hint shown for an unset text/HCL field.
// Required fields read as "(unset)" since there's no fallback; optional
// fields surface their default so the user can see what would land in
// the generated file if they leave the field alone.
func renderUnsetTextValue(fld *FormField) string {
	if fld.Required {
		return formDefaultHintStyle.Render("(unset)")
	}

	if fld.Initial == "" {
		return formDefaultHintStyle.Render("(default)")
	}

	return formDefaultHintStyle.Render("(default: " + fld.Initial + ")")
}

// renderCheckboxValue picks the visual for a bool-mode field. Set fields
// render in the bright committed style; unset optional fields fall back to
// the same `(default: <value>)` shape used by text fields so each row
// reads consistently. Required fields without a value land on "(unset)"
// since there's no fallback.
func renderCheckboxValue(fld *FormField) string {
	if fld.Set {
		return renderCheckbox(fld.Bool)
	}

	if fld.Required {
		return formDefaultHintStyle.Render("(unset)")
	}

	val := "false"
	if fld.BoolInitial {
		val = "true"
	}

	return formDefaultHintStyle.Render("(default: " + val + ")")
}
