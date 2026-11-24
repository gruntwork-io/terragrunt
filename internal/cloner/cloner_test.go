package cloner_test

import (
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cloner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type level int

type label string

type inner struct {
	Name string
}

type everyKind struct {
	Any       any
	Ptr       *inner
	NilPtr    *inner
	Map       map[string]*inner
	Fn        func() string
	Ch        chan int
	unexposed *inner
	Array     [2]*inner
	Str       string
	Label     label
	Slice     []*inner
	Int       int
	Int64     int64
	Uint      uint
	Float64   float64
	Level     level
	Float32   float32
	Uint16    uint16
	Int8      int8
	Bool      bool
}

type tagged struct {
	Shared   *inner `clone:"shadowcopy"`
	Skipped  *inner `clone:"skip"`
	Dashed   *inner `clone:"-"`
	Required *inner `clone:"required"`
	Plain    *inner
}

type withExternal struct {
	Query url.Values
	Local map[string]string
	Named labels
}

type labels map[string]string

func TestClone_DeepCopiesEveryKind(t *testing.T) {
	t.Parallel()

	shared := &inner{Name: "any"}
	ch := make(chan int)
	src := everyKind{
		Ptr:       &inner{Name: "ptr"},
		Map:       map[string]*inner{"k": {Name: "map"}},
		Any:       shared,
		Fn:        func() string { return "fn" },
		Ch:        ch,
		unexposed: &inner{Name: "hidden"},
		Str:       "str",
		Label:     label("label"),
		Slice:     []*inner{{Name: "slice"}},
		Array:     [2]*inner{{Name: "array"}, nil},
		Int:       -1,
		Int8:      -8,
		Int64:     -64,
		Uint:      1,
		Uint16:    16,
		Float32:   3.5,
		Float64:   6.25,
		Level:     level(7),
		Bool:      true,
	}

	dst := cloner.Clone(src)

	assert.Equal(t, src.Ptr, dst.Ptr, "pointed-to values should match")
	assert.NotSame(t, src.Ptr, dst.Ptr, "pointers should be deep copied")
	assert.Nil(t, dst.NilPtr, "a nil pointer should stay nil")
	assert.Equal(t, src.Map, dst.Map)
	assert.NotSame(t, src.Map["k"], dst.Map["k"], "map values should be deep copied")
	assert.Same(t, shared, dst.Any, "interface values should be shared, not copied")
	require.NotNil(t, dst.Fn, "funcs should be carried over")
	assert.Equal(t, "fn", dst.Fn())
	assert.Equal(t, ch, dst.Ch, "channels should be shared")
	assert.Nil(t, dst.unexposed, "unexported fields should not be copied")
	assert.Equal(t, src.Slice, dst.Slice)
	assert.NotSame(t, src.Slice[0], dst.Slice[0], "slice elements should be deep copied")
	assert.Equal(t, src.Array, dst.Array)
	assert.NotSame(t, src.Array[0], dst.Array[0], "array elements should be deep copied")
	assert.Nil(t, dst.Array[1], "a nil array element should stay nil")

	scalars := func(v everyKind) []any {
		return []any{v.Str, v.Label, v.Int, v.Int8, v.Int64, v.Uint, v.Uint16, v.Float32, v.Float64, v.Level, v.Bool}
	}
	assert.Equal(t, scalars(src), scalars(dst), "scalar fields should keep their values and types")

	dst.Ptr.Name = "changed"
	dst.Map["k"].Name = "changed"
	dst.Slice[0].Name = "changed"
	dst.Array[0].Name = "changed"

	assert.Equal(t, "ptr", src.Ptr.Name, "changing the clone should not reach the source pointer")
	assert.Equal(t, "map", src.Map["k"].Name, "changing the clone should not reach the source map")
	assert.Equal(t, "slice", src.Slice[0].Name, "changing the clone should not reach the source slice")
	assert.Equal(t, "array", src.Array[0].Name, "changing the clone should not reach the source array")
}

func TestClone_TopLevelCollections(t *testing.T) {
	t.Parallel()

	srcMap := map[string]string{"a": "1"}
	dstMap := cloner.Clone(srcMap)
	dstMap["a"] = "2"

	assert.Equal(t, "1", srcMap["a"], "a cloned map should not share storage")

	srcSlice := []int{1, 2}
	dstSlice := cloner.Clone(srcSlice)
	dstSlice[0] = 9

	assert.Equal(t, []int{1, 2}, srcSlice, "a cloned slice should not share storage")
}

func TestClone_FieldTags(t *testing.T) {
	t.Parallel()

	src := &tagged{
		Shared:   &inner{Name: "shared"},
		Skipped:  &inner{Name: "skipped"},
		Dashed:   &inner{Name: "dashed"},
		Required: &inner{Name: "required"},
		Plain:    &inner{Name: "plain"},
	}

	dst := cloner.Clone(src)

	require.NotSame(t, src, dst)
	assert.Same(t, src.Shared, dst.Shared, "shadowcopy fields should keep the source pointer")
	assert.Nil(t, dst.Skipped, "skip fields should be left empty")
	assert.Nil(t, dst.Dashed, "fields tagged - should be left empty")
	assert.Equal(t, src.Required, dst.Required)
	assert.NotSame(t, src.Required, dst.Required, "required fields should be deep copied")
	assert.Equal(t, src.Plain, dst.Plain)
	assert.NotSame(t, src.Plain, dst.Plain, "untagged fields should be deep copied")
}

func TestClone_RequiredTagOverridesShadowCopyType(t *testing.T) {
	t.Parallel()

	src := struct {
		Required *inner `clone:"required"`
		Plain    *inner
	}{
		Required: &inner{Name: "required"},
		Plain:    &inner{Name: "plain"},
	}

	dst := cloner.Clone(src, cloner.WithShadowCopyTypes(&inner{}))

	assert.NotSame(t, src.Required, dst.Required, "required fields should be deep copied despite the option")
	assert.Same(t, src.Plain, dst.Plain, "other fields of a shadow copy type should share the pointer")
}

func TestClone_Options(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		opts       []cloner.Option
		wantShared bool
		wantNil    bool
	}{
		{
			name: "no options deep copies",
		},
		{
			name:       "shadow copy types share the pointer",
			opts:       []cloner.Option{cloner.WithShadowCopyTypes(&inner{})},
			wantShared: true,
		},
		{
			name:    "skipping types leave the field empty",
			opts:    []cloner.Option{cloner.WithSkippingTypes(&inner{})},
			wantNil: true,
		},
		{
			name: "unrelated shadow copy types are ignored",
			opts: []cloner.Option{cloner.WithShadowCopyTypes(new(level))},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := struct{ Ptr *inner }{Ptr: &inner{Name: "ptr"}}

			dst := cloner.Clone(src, tc.opts...)

			if tc.wantNil {
				assert.Nil(t, dst.Ptr, "a skipped type should not be copied")
				return
			}

			require.NotNil(t, dst.Ptr)
			assert.Equal(t, "ptr", dst.Ptr.Name)

			if tc.wantShared {
				assert.Same(t, src.Ptr, dst.Ptr, "a shadow copy type should keep the source pointer")
				return
			}

			assert.NotSame(t, src.Ptr, dst.Ptr, "the pointer should be deep copied")
		})
	}
}

func TestClone_ShadowCopyInversePkgPrefixes(t *testing.T) {
	t.Parallel()

	src := withExternal{
		Query: url.Values{"q": {"1"}},
		Local: map[string]string{"k": "v"},
		Named: labels{"k": "v"},
	}

	dst := cloner.Clone(src, cloner.WithShadowCopyInversePkgPrefixes("github.com/gruntwork-io/terragrunt"))

	assert.Equal(t, src.Named, dst.Named, "named types inside the prefixes should keep their contents")

	dst.Query.Set("q", "2")
	dst.Local["k"] = "changed"
	dst.Named["k"] = "changed"

	assert.Equal(t, "2", src.Query.Get("q"), "types outside the prefixes should be shared with the source")
	assert.Equal(t, "v", src.Local["k"], "unnamed types should still be deep copied")
	assert.Equal(t, "v", src.Named["k"], "named types inside the prefixes should be deep copied")
}

func TestCloner_CloneUsesConfig(t *testing.T) {
	t.Parallel()

	c := cloner.Cloner[[]string]{Config: &cloner.Config{}}
	src := []string{"a"}

	dst := c.Clone(src)
	dst[0] = "b"

	assert.Equal(t, []string{"a"}, src, "the clone should not share the backing array")
}
