package hclparse

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

// getDiagnosticVariablesFromAttribute extracts variable references from HCL diagnostic messages.
// This function is essential for identifying expansion-related decode failures.
//
// When HCL decoding fails due to undefined variables like "count.index" or "each.key",
// the diagnostic contains position information that we can use to locate the problematic
// attribute. This function finds that attribute and extracts all variable references
// from its expression, allowing us to identify which failures are expansion-related
// versus legitimate configuration errors.
//
// Returns a slice of variable names found in the diagnostic's attribute expression.
func getDiagnosticVariablesFromAttribute(file *File, diag *hcl.Diagnostic) []string {
	var variables []string
	
	// Locate the attribute at the diagnostic position to analyze its expression
	if attr := file.AttributeAtPos(diag.Subject.Start); attr != nil {
		if attr.Expr != nil {
			if vars := attr.Expr.Variables(); len(vars) > 0 {
				for _, traversal := range vars {
					variables = append(variables, renderTraversal(traversal))
				}
			}
		}
	}
	
	return variables
}

// renderTraversal converts an HCL traversal into a human-readable string representation.
// This is used for diagnostic analysis and pattern matching of variable references.
//
// HCL traversals represent variable access patterns in expressions. For expansion detection,
// we need to convert these into strings so we can pattern-match against known expansion
// variables like "count.index", "each.key", and "each.value".
//
// The function handles different traversal types:
//   - TraverseRoot: Root variable name (e.g., "count")
//   - TraverseAttr: Attribute access (e.g., ".index" → "count.index")
//   - TraverseIndex: Index access (e.g., "[0]" → "dependency[0]")
//
// Returns a string representation of the traversal path.
func renderTraversal(traversal hcl.Traversal) string {
	var result string
	for _, step := range traversal {
		switch tStep := step.(type) {
		case hcl.TraverseRoot:
			result += tStep.Name
		case hcl.TraverseAttr:
			result += "." + tStep.Name
		case hcl.TraverseIndex:
			result += "["
			if keyTy := tStep.Key.Type(); keyTy.IsPrimitiveType() {
				// For primitive types, try to get a string representation
				if keyTy == cty.String {
					result += tStep.Key.AsString()
				} else if keyTy == cty.Number {
					result += tStep.Key.AsBigFloat().String()
				} else {
					result += "..."
				}
			} else {
				result += "..."
			}
			result += "]"
		}
	}
	return result
}

// processExpandableBlocks handles the expansion of blocks with count/for_each
// processExpandableBlocks orchestrates the expansion of blocks that use count/for_each meta-arguments.
// This is the main entry point for the expansion system, called during HCL decoding.
//
// The function processes HCL decoding diagnostics to identify blocks that failed because they
// use count/for_each variables (like count.index, each.key, each.value) but haven't been
// expanded yet. It then expands these blocks and filters out the related errors.
//
// This approach is necessary because HCL's gohcl.DecodeBody expects a fixed struct definition,
// but count/for_each creates dynamic block instances. We let the initial decode fail with
// "count.index" or "each.key" errors, then analyze those failures to identify expansion candidates.
//
// Returns filtered diagnostics (with expansion-related errors removed) and any expansion errors.
func (file *File) processExpandableBlocks(out any, evalContext *hcl.EvalContext, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
	var filteredDiags hcl.Diagnostics

	file.logger.Tracef("Analyzing %d diagnostics for expansion opportunities in %s", len(diags), file.ConfigPath)

	// Process each diagnostic to identify expansion candidates
	// We examine decode failures that mention count/for_each variables
	for i, diag := range diags {
		if !file.isExpandableBlockError(diag) {
			// Not an expansion-related error, keep it for final error handling
			filteredDiags = append(filteredDiags, diag)
			continue
		}

		file.logger.Tracef("Found expandable block error (diagnostic %d/%d): %s", i+1, len(diags), diag.Summary)

		// Find the block that caused this expansion-related error
		// The diagnostic subject should point to the block's location
		blocks := file.BlocksAtPos(diag.Subject.Start)
		if len(blocks) == 0 {
			file.logger.Debugf("No blocks found at diagnostic position")
			filteredDiags = append(filteredDiags, diag)
			continue
		}

		block := blocks[0]
		file.logger.Debugf("Found block '%s' of type '%s' for expansion", block.Labels, block.Type)

		// Validate the block is actually expandable by checking struct compatibility
		fieldValue, isExpandable := file.getExpandableField(out, block)
		if !isExpandable {
			file.logger.Debugf("Block '%s' is not expandable, skipping", block.Labels)
			filteredDiags = append(filteredDiags, diag)
			continue
		}

		file.logger.Tracef("Expanding block '%s' with %d existing elements", block.Labels, fieldValue.Len())

		// Perform the actual expansion: evaluate count/for_each and create multiple block instances and replace the original block with the expanded blocks
		if err := file.expandAndReplaceBlock(fieldValue, block, evalContext); err != nil {
			file.logger.Debugf("Failed to expand block '%s': %v", block.Labels, err)
			return filteredDiags, err
		}

		file.logger.Debugf("Successfully expanded block '%s', now has %d elements", block.Labels, fieldValue.Len())
	}

	file.logger.Tracef("Completed expansion analysis, filtered %d diagnostics to %d", len(diags), len(filteredDiags))

	return filteredDiags, nil
}

// isExpandableBlockError determines if a diagnostic error is caused by count/for_each expansion needs.
// This function is critical for identifying blocks that should be expanded.
//
// When HCL decoding encounters count/for_each variables (count.index, each.key, each.value) in
// a block that hasn't been expanded yet, it fails with "undefined variable" errors. This function
// detects those specific error patterns to identify expansion candidates.
//
// The function parses diagnostic error messages to extract variable references and checks if
// they match the known count/for_each variable patterns. This allows us to distinguish between
// legitimate configuration errors and expansion-related decode failures.
//
// Returns true if the diagnostic indicates a block needs count/for_each expansion, false otherwise.
func (file *File) isExpandableBlockError(diag *hcl.Diagnostic) bool {
	variables := getDiagnosticVariablesFromAttribute(file, diag)
	for _, variable := range variables {
		if variable == "count.index" || variable == "each.key" || variable == "each.value" {
			file.logger.Tracef("Detected expandable block error with variable '%s'", variable)
			return true
		}
	}
	return false
}

// getExpandableField locates the struct field that corresponds to an expandable block type.
// This function bridges the gap between HCL blocks and Go struct fields for expansion.
//
// The function performs a two-step validation process:
// 1. Field Matching: Find struct fields that match the block type via HCL tags
// 2. Expansion Validation: Verify both the struct and block support count/for_each
//
// For example, a "dependency" block would match a struct field tagged with `hcl:"dependency,block"`,
// and then we verify that both the field's type (e.g., []Dependency) and the actual block
// support the same meta-arguments (count or for_each).
//
// This validation prevents expansion attempts on incompatible struct/block combinations
// and ensures the expansion system only operates on properly configured elements.
//
// Returns the field value and true if an expandable field is found, zero value and false otherwise.
func (file *File) getExpandableField(out any, block *hcl.Block) (reflect.Value, bool) {
	structType := reflect.TypeOf(out).Elem()
	
	file.logger.Tracef("Searching for expandable field in struct with %d fields for block type '%s'", structType.NumField(), block.Type)
	
	// Search through all struct fields to find one that matches the block type
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if file.isMatchingBlockField(field, block.Type) {
			fieldValue := reflect.ValueOf(out).Elem().Field(i)
			// Found a matching field, now verify it's compatible with expansion
			if file.hasExpandableAttributes(field.Type, block) {
				file.logger.Tracef("Found expandable field '%s' for block type '%s'", field.Name, block.Type)
				return fieldValue, true
			}
		}
	}

	file.logger.Debugf("No expandable field found for block type '%s'", block.Type)
	return reflect.Value{}, false
}

// isMatchingBlockField determines if a struct field corresponds to a specific HCL block type.
// This is a helper function for field discovery during expansion.
//
// The function examines the struct field's HCL tag to determine if it should receive
// blocks of the specified type. HCL tags have the format "blockname,block" or similar.
//
// For example:
//   - Field tagged `hcl:"dependency,block"` matches blockType "dependency"
//   - Field tagged `hcl:"unit,block"` matches blockType "unit"
//   - Field with no HCL tag doesn't match any block type
//
// Returns true if the field's HCL tag matches the block type, false otherwise.
func (file *File) isMatchingBlockField(field reflect.StructField, blockType string) bool {
	tag := field.Tag.Get("hcl")
	if tag == "" {
		return false
	}
	tagParts := strings.Split(tag, ",")
	return tagParts[0] == blockType
}

// hasExpandableAttributes determines if a block is eligible for count/for_each expansion.
// This is the core validation that ensures both the struct definition and HCL block usage
// are compatible for expansion.
//
// The function performs dual validation:
// 1. Struct Compatibility: Checks if the target struct has fields tagged with "count,attr" or "for_each,attr"
// 2. Block Usage: Verifies the HCL block actually uses count or for_each attributes
//
// Both conditions must be true for expansion to proceed. This prevents expansion attempts
// on structs that lack the required metadata fields (CountIndex, EachKey) that expansion
// depends on.
//
// Returns true if the block should be expanded, false otherwise.
func (file *File) hasExpandableAttributes(fieldType reflect.Type, block *hcl.Block) bool {
	// Extract the underlying struct type, handling slices and pointers
	// Most terragrunt blocks are stored as []BlockType, so we need the element type
	elemType := fieldType
	if fieldType.Kind() == reflect.Slice {
		elemType = fieldType.Elem()
	}
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		return false
	}

	// Check struct compatibility: does the struct have the required HCL tags?
	// These tags indicate the struct can receive count/for_each values during decoding
	hasCount := file.hasStructField(elemType, "count,attr")
	hasForEach := file.hasStructField(elemType, "for_each,attr")
	
	// Check block usage: does the HCL block actually use count or for_each?
	// We need to parse the block attributes to see what meta-arguments are present
	attrs, diags := block.Body.JustAttributes()
	if diags.HasErrors() {
		return false
	}
	
	_, blockHasCount := attrs["count"]
	_, blockHasForEach := attrs["for_each"]
	
	// Expansion is only valid when both struct supports AND block uses the same meta-argument
	// This prevents mismatched scenarios like a block using count but struct lacking Count field
	result := (hasCount && blockHasCount) || (hasForEach && blockHasForEach)
	
	file.logger.Tracef("Block expandability check: struct has count=%v, for_each=%v; block has count=%v, for_each=%v; result=%v", 
		hasCount, hasForEach, blockHasCount, blockHasForEach, result)
	
	return result
}

// hasStructField checks if a struct contains a field with a specific HCL tag.
// This is used to verify struct compatibility with count/for_each expansion.
//
// The function searches through all struct fields looking for an exact match
// on the HCL tag. For expansion support, we look for:
// - "count,attr" tag (indicates struct can receive count values)
// - "for_each,attr" tag (indicates struct can receive for_each values)
//
// Example: A struct with `Count *cty.Value `hcl:"count,attr"`` would return true
// when called with tag "count,attr".
//
// Returns true if a field with the specified HCL tag exists, false otherwise.
func (file *File) hasStructField(structType reflect.Type, tag string) bool {
	for i := 0; i < structType.NumField(); i++ {
		if structType.Field(i).Tag.Get("hcl") == tag {
			return true
		}
	}
	return false
}

// expandAndReplaceBlock expands a block and replaces it in the field
func (file *File) expandAndReplaceBlock(fieldValue reflect.Value, block *hcl.Block, evalContext *hcl.EvalContext) error {
	// Check if already processed
	if file.isAlreadyExpanded(fieldValue, block) {
		file.logger.Tracef("Block '%s' is already expanded, skipping", block.Labels)
		return nil
	}

	// Create expansion context
	expCtx := &expansionContext{
		fieldValue:  fieldValue,
		block:       block,
		evalContext: evalContext,
		file:        file,
	}

	file.logger.Debugf("Starting expansion for block '%s'", block.Labels)

	// Expand the block
	expandedBlocks, err := expCtx.expand()
	if err != nil {
		return err
	}

	file.logger.Debugf("Generated %d expanded blocks for '%s'", len(expandedBlocks), block.Labels)

	// Replace the original block with expanded blocks
	file.replaceBlockInSlice(fieldValue, block, expandedBlocks)
	
	file.logger.Tracef("Successfully replaced block '%s' with %d expanded instances", block.Labels, len(expandedBlocks))
	
	return nil
}

// isAlreadyExpanded checks if a block has already been expanded
func (file *File) isAlreadyExpanded(fieldValue reflect.Value, targetBlock *hcl.Block) bool {
	for i := 0; i < fieldValue.Len(); i++ {
		blockValue := fieldValue.Index(i)
		if blockValue.Kind() == reflect.Ptr && !blockValue.IsNil() {
			blockValue = blockValue.Elem()
		}
		
		nameField := blockValue.FieldByName("Name")
		if nameField.IsValid() && nameField.String() == targetBlock.Labels[0] {
			// Check if this is an expanded block
			if file.hasExpansionMetadata(blockValue) {
				return true
			}
		}
	}
	return false
}

// hasExpansionMetadata checks if a block has expansion metadata (CountIndex or EachKey)
func (file *File) hasExpansionMetadata(blockValue reflect.Value) bool {
	countField := blockValue.FieldByName("CountIndex")
	eachField := blockValue.FieldByName("EachKey")
	return (countField.IsValid() && !countField.IsNil()) || (eachField.IsValid() && !eachField.IsNil())
}

// replaceBlockInSlice replaces the original block with expanded blocks in the slice
func (file *File) replaceBlockInSlice(fieldValue reflect.Value, targetBlock *hcl.Block, expandedBlocks []reflect.Value) {
	originalLen := fieldValue.Len()
	newSlice := reflect.MakeSlice(fieldValue.Type(), 0, originalLen+len(expandedBlocks))
	
	// Add expanded blocks first
	for i, expandedBlock := range expandedBlocks {
		newSlice = reflect.Append(newSlice, expandedBlock)
		file.logger.Tracef("Added expanded block %d/%d to slice", i+1, len(expandedBlocks))
	}
	
	// Add existing blocks that don't match the target
	kept := 0
	for i := 0; i < fieldValue.Len(); i++ {
		blockValue := fieldValue.Index(i)
		if !file.isTargetBlock(blockValue, targetBlock) {
			newSlice = reflect.Append(newSlice, blockValue)
			kept++
		}
	}
	
	file.logger.Tracef("Slice replacement: original=%d, expanded=%d, kept=%d, final=%d", 
		originalLen, len(expandedBlocks), kept, newSlice.Len())
	
	fieldValue.Set(newSlice)
}

// isTargetBlock checks if a block matches the target block name
func (file *File) isTargetBlock(blockValue reflect.Value, targetBlock *hcl.Block) bool {
	if blockValue.Kind() == reflect.Ptr && !blockValue.IsNil() {
		blockValue = blockValue.Elem()
	}
	nameField := blockValue.FieldByName("Name")
	return nameField.IsValid() && nameField.String() == targetBlock.Labels[0]
}

// createNewBlock creates a new block instance with proper pointer handling
func (file *File) createNewBlock(elemType reflect.Type) reflect.Value {
	if elemType.Kind() == reflect.Ptr {
		return reflect.New(elemType.Elem()).Elem()
	}
	return reflect.New(elemType).Elem()
}

// expansionContext holds the context for block expansion
type expansionContext struct {
	fieldValue  reflect.Value
	block       *hcl.Block
	evalContext *hcl.EvalContext
	file        *File
}

// expand performs the actual block expansion
func (ctx *expansionContext) expand() ([]reflect.Value, error) {
	attrs, diags := ctx.block.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to get block attributes: %v", diags)
	}

	countExpr, hasCount := attrs["count"]
	forEachExpr, hasForEach := attrs["for_each"]

	// Validate mutual exclusion: only one of count or for_each is allowed
	if hasCount && hasForEach {
		return nil, fmt.Errorf("block '%s' cannot have both count and for_each - they are mutually exclusive", ctx.block.Labels)
	}

	if hasCount {
		ctx.file.logger.Tracef("Expanding block '%s' using count", ctx.block.Labels)
		return ctx.expandWithCount(countExpr)
	}
	
	if hasForEach {
		ctx.file.logger.Tracef("Expanding block '%s' using for_each", ctx.block.Labels)
		return ctx.expandWithForEach(forEachExpr)
	}

	return nil, fmt.Errorf("block has neither count nor for_each attribute")
}

// expandWithCount expands a block using count
func (ctx *expansionContext) expandWithCount(countExpr *hcl.Attribute) ([]reflect.Value, error) {
	if err := ctx.validateCountExpression(countExpr.Expr); err != nil {
		return nil, err
	}

	countVal, diags := countExpr.Expr.Value(ctx.evalContext)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to evaluate count expression: %v", diags)
	}

	count, err := convertCtyToInt(countVal)
	if err != nil {
		return nil, fmt.Errorf("count expression must evaluate to a number: %v", err)
	}

	ctx.file.logger.Tracef("Expanding block '%s' with count=%d", ctx.block.Labels, count)

	var expandedBlocks []reflect.Value
	for i := 0; i < count; i++ {
		ctx.file.logger.Tracef("Creating expanded block %d/%d for '%s'", i+1, count, ctx.block.Labels)
		
		newBlock, err := ctx.createExpandedBlock(i, nil, nil)
		if err != nil {
			return nil, err
		}
		expandedBlocks = append(expandedBlocks, newBlock)
	}

	return expandedBlocks, nil
}

// expandWithForEach expands a block using for_each
func (ctx *expansionContext) expandWithForEach(forEachExpr *hcl.Attribute) ([]reflect.Value, error) {
	if err := ctx.validateForEachExpression(forEachExpr.Expr); err != nil {
		return nil, err
	}

	forEachVal, diags := forEachExpr.Expr.Value(ctx.evalContext)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to evaluate for_each expression: %v", diags)
	}

	if forEachVal.Type().IsMapType() || forEachVal.Type().IsSetType() {
		collectionType := "map"
		if forEachVal.Type().IsSetType() {
			collectionType = "set"
		}
		ctx.file.logger.Tracef("Expanding block '%s' with for_each %s containing %d elements", ctx.block.Labels, collectionType, forEachVal.LengthInt())
		return ctx.expandWithForEachCollection(forEachVal)
	}

	return nil, fmt.Errorf("for_each expression must evaluate to a map or set, got %s", forEachVal.Type().FriendlyName())
}

// expandWithForEachCollection expands a block using either a map or set for_each.
// This consolidated function handles both collection types using a unified iteration approach.
func (ctx *expansionContext) expandWithForEachCollection(forEachVal cty.Value) ([]reflect.Value, error) {
	var expandedBlocks []reflect.Value
	isMap := forEachVal.Type().IsMapType()
	
	// Use iterator approach for both maps and sets for consistency
	it := forEachVal.ElementIterator()
	
	i := 0
	for it.Next() {
		i++
		keyVal, value := it.Element()
		
		// Extract string key - maps provide it directly, sets derive it from value
		var key string
		if isMap {
			// For maps, use the actual key
			if keyVal.Type().Equals(cty.String) {
				key = keyVal.AsString()
			} else if keyVal.Type().Equals(cty.Number) {
				key = fmt.Sprintf("%g", keyVal.AsBigFloat())
			} else {
				return nil, fmt.Errorf("unsupported map key type: %s", keyVal.Type().FriendlyName())
			}
		} else {
			// For sets, the "key" is the stringified value
			if value.Type().Equals(cty.String) {
				key = value.AsString()
			} else if value.Type().Equals(cty.Number) {
				key = fmt.Sprintf("%g", value.AsBigFloat())
			} else {
				return nil, fmt.Errorf("unsupported set element type: %s", value.Type().FriendlyName())
			}
		}

		collectionType := "set element"
		if isMap {
			collectionType = "key"
		}
		ctx.file.logger.Tracef("Creating expanded block %d for %s '%s' in '%s'", i, collectionType, key, ctx.block.Labels)

		newBlock, err := ctx.createExpandedBlock(-1, &key, &value)
		if err != nil {
			return nil, err
		}
		expandedBlocks = append(expandedBlocks, newBlock)
	}

	return expandedBlocks, nil
}

// createExpandedBlock creates a single expanded block instance
func (ctx *expansionContext) createExpandedBlock(countIndex int, eachKey *string, eachValue *cty.Value) (reflect.Value, error) {
	elemType := ctx.fieldValue.Type().Elem()
	newBlock := ctx.file.createNewBlock(elemType)

	// Set up evaluation context
	newEvalCtx := *ctx.evalContext
	newEvalCtx.Variables = make(map[string]cty.Value)
	
	// Copy existing variables
	for k, v := range ctx.evalContext.Variables {
		newEvalCtx.Variables[k] = v
	}
	
	if countIndex >= 0 {
		newEvalCtx.Variables["count"] = cty.ObjectVal(map[string]cty.Value{
			"index": cty.NumberIntVal(int64(countIndex)),
		})
		ctx.file.logger.Tracef("Added count.index=%d to evaluation context", countIndex)
	}
	
	if eachKey != nil {
		eachVal := cty.StringVal(*eachKey)
		if eachValue != nil {
			eachVal = *eachValue
		}
		newEvalCtx.Variables["each"] = cty.ObjectVal(map[string]cty.Value{
			"key":   cty.StringVal(*eachKey),
			"value": eachVal,
		})
		ctx.file.logger.Tracef("Added each.key='%s' and each.value to evaluation context", *eachKey)
	}

	// Decode the block
	diags := gohcl.DecodeBody(ctx.block.Body, &newEvalCtx, newBlock.Addr().Interface())
	if diags.HasErrors() {
		return reflect.Value{}, fmt.Errorf("failed to decode block: %v", diags)
	}

	// Set metadata fields directly on the block
	if countIndex >= 0 {
		if field := newBlock.FieldByName("CountIndex"); field.IsValid() && field.CanSet() {
			field.Set(reflect.ValueOf(&countIndex))
		}
	}
	
	if eachKey != nil {
		if field := newBlock.FieldByName("EachKey"); field.IsValid() && field.CanSet() {
			field.Set(reflect.ValueOf(eachKey))
		}
	}
	
	if field := newBlock.FieldByName("Name"); field.IsValid() && field.CanSet() {
		field.SetString(ctx.block.Labels[0])
	}

	// Convert to pointer if needed
	if elemType.Kind() == reflect.Ptr {
		newBlock = newBlock.Addr()
	}

	displayName := ctx.block.Labels[0]
	if eachKey != nil {
		displayName = fmt.Sprintf("%s[%s]", displayName, *eachKey)
	} else if countIndex >= 0 {
		displayName = fmt.Sprintf("%s[%d]", displayName, countIndex)
	}
	ctx.file.logger.Tracef("Successfully created expanded block instance: %s", displayName)

	return newBlock, nil
}



// validateCountExpression validates a count expression
func (ctx *expansionContext) validateCountExpression(expr hcl.Expression) error {
	countVal, diags := expr.Value(ctx.evalContext)
	if diags.HasErrors() {
		return ctx.createValidationError("count", expr, fmt.Sprintf("expression evaluation failed: %v", diags))
	}
	
	if !countVal.Type().Equals(cty.Number) {
		return ctx.createValidationError("count", expr, fmt.Sprintf("must be a number, got %s", countVal.Type().FriendlyName()))
	}
	
	if countVal.IsKnown() && !countVal.IsNull() {
		count, err := convertCtyToInt(countVal)
		if err == nil && count < 0 {
			return ctx.createValidationError("count", expr, "must be greater than or equal to 0")
		}
	}
	
	return nil
}

// validateForEachExpression validates a for_each expression
func (ctx *expansionContext) validateForEachExpression(expr hcl.Expression) error {
	forEachVal, diags := expr.Value(ctx.evalContext)
	if diags.HasErrors() {
		return ctx.createValidationError("for_each", expr, fmt.Sprintf("expression evaluation failed: %v", diags))
	}
	
	if !forEachVal.CanIterateElements() {
		return ctx.createValidationError("for_each", expr, fmt.Sprintf("must be a map or set, got %s", forEachVal.Type().FriendlyName()))
	}
	
	if forEachVal.Type().IsMapType() || forEachVal.Type().IsSetType() {
		return ctx.validateForEachElements(forEachVal, expr)
	}
	
	return ctx.createValidationError("for_each", expr, fmt.Sprintf("must be a map or set, got %s", forEachVal.Type().FriendlyName()))
}

// validateForEachElements validates elements in map or set for_each collections.
// This consolidated function handles both map and set validation using a common pattern.
func (ctx *expansionContext) validateForEachElements(forEachVal cty.Value, expr hcl.Expression) error {
	it := forEachVal.ElementIterator()
	if !it.Next() {
		return nil // Empty collection is valid
	}
	
	key, val := it.Element()
	isMap := forEachVal.Type().IsMapType()
	
	// For maps, validate both keys and values; for sets, only validate elements (values)
	if isMap {
		if !key.Type().Equals(cty.String) && !key.Type().Equals(cty.Number) {
			return ctx.createValidationError("for_each", expr, fmt.Sprintf("map keys must be strings or numbers, got %s", key.Type().FriendlyName()))
		}
	}
	
	// Validate element/value type for both maps and sets
	if !val.Type().Equals(cty.String) && !val.Type().Equals(cty.Number) {
		elementType := "set elements"
		if isMap {
			elementType = "map values"
		}
		return ctx.createValidationError("for_each", expr, fmt.Sprintf("%s must be strings or numbers, got %s", elementType, val.Type().FriendlyName()))
	}
	
	return nil
}

// createValidationError creates a standardized validation error
func (ctx *expansionContext) createValidationError(metaArg string, expr hcl.Expression, message string) error {
	return fmt.Errorf("Invalid %s argument\n\n  on %s line %d, in %s:\n  %d:  %s = %s\n\nThe given \"%s\" argument value is unsuitable: %s",
		metaArg, ctx.file.ConfigPath, expr.StartRange().Start.Line, ctx.block.Type, 
		expr.StartRange().Start.Line, metaArg, expr.Range().String(), metaArg, message)
}

// convertCtyToInt converts a cty.Value to an integer
func convertCtyToInt(val cty.Value) (int, error) {
	if !val.Type().Equals(cty.Number) {
		return 0, fmt.Errorf("value is not a number")
	}
	
	var result int
	err := gocty.FromCtyValue(val, &result)
	return result, err
} 