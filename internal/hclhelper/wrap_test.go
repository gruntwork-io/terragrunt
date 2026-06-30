package hclhelper_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclhelper"
)

func TestWrapMapToSingleLineHcl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    map[string]any
		expected string
	}{
		{
			name:     "SimpleMap",
			input:    map[string]any{"key1": "value1", "key2": 46521694, "key3": true},
			expected: `{key1="value1",key2=46521694,key3=true}`,
		},
		{
			name:     "NestedMap",
			input:    map[string]any{"key1": "value1", "key2": map[string]any{"nestedKey": "nestedValue"}},
			expected: `{key1="value1",key2={nestedKey="nestedValue"}}`,
		},
		{
			name:     "StringWithSpecialChars",
			input:    map[string]any{"key": "a\"b\nc\td"},
			expected: `{key="a\"b\nc\td"}`,
		},
		{
			name:     "SliceOfStrings",
			input:    map[string]any{"files": []any{"a", "b"}},
			expected: `{files=["a","b"]}`,
		},
		{
			name:     "SliceOfMixed",
			input:    map[string]any{"mixed": []any{"a", 1, true}},
			expected: `{mixed=["a",1,true]}`,
		},
		{
			name:     "EmptySlice",
			input:    map[string]any{"files": []any{}},
			expected: `{files=[]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := hclhelper.WrapMapToSingleLineHcl(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %s, but got %s", tc.expected, result)
			}
		})
	}
}

func TestWrapListToSingleLineHcl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		expected string
		input    []any
	}{
		{
			name:     "Empty",
			input:    []any{},
			expected: `[]`,
		},
		{
			name:     "Strings",
			input:    []any{"a", "b", "c"},
			expected: `["a","b","c"]`,
		},
		{
			name:     "StringsWithEscapes",
			input:    []any{"a\"b", "c\nd"},
			expected: `["a\"b","c\nd"]`,
		},
		{
			name:     "Mixed",
			input:    []any{"a", 1, true, 2.5},
			expected: `["a",1,true,2.5]`,
		},
		{
			name:     "NestedMap",
			input:    []any{map[string]any{"k": "v"}},
			expected: `[{k="v"}]`,
		},
		{
			name:     "NestedList",
			input:    []any{[]any{"a", "b"}, []any{1, 2}},
			expected: `[["a","b"],[1,2]]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := hclhelper.WrapListToSingleLineHcl(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %s, but got %s", tc.expected, result)
			}
		})
	}
}

func TestFormatValueToSingleLineHcl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    any
		expected string
	}{
		{name: "String", input: "hello", expected: `"hello"`},
		{name: "StringWithQuote", input: `a"b`, expected: `"a\"b"`},
		{name: "StringWithNewline", input: "a\nb", expected: `"a\nb"`},
		{name: "StringWithTab", input: "a\tb", expected: `"a\tb"`},
		{name: "Int", input: 42, expected: `42`},
		{name: "Bool", input: true, expected: `true`},
		{name: "Float", input: 2.5, expected: `2.5`},
		{name: "Map", input: map[string]any{"k": "v"}, expected: `{k="v"}`},
		{name: "Slice", input: []any{"a", 1}, expected: `["a",1]`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := hclhelper.FormatValueToSingleLineHcl(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %s, but got %s", tc.expected, result)
			}
		})
	}
}
