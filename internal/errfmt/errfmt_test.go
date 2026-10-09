package errfmt_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errfmt"
	"github.com/stretchr/testify/assert"
)

func TestFormat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		err  error
		name string
		want string
	}{
		{
			name: "plain error",
			err:  errors.New("boom"),
			want: "boom",
		},
		{
			name: "join drops nil",
			err:  errors.Join(nil, errors.New("boom"), nil),
			want: "error occurred:\n\n* boom\n",
		},
		{
			name: "multi-line error indents continuation lines",
			err:  errors.Join(errors.New("first line\nsecond line")),
			want: "error occurred:\n\n* first line\n  second line\n",
		},
		{
			name: "windows line endings",
			err:  errors.Join(errors.New("first line\r\nsecond line")),
			want: "error occurred:\n\n* first line\n  second line\n",
		},
		{
			name: "multiple errors",
			err:  errors.Join(errors.New("one"), errors.New("two")),
			want: "2 errors occurred:\n\n* one\n\n* two\n",
		},
		{
			name: "nested joins flatten",
			err: errors.Join(
				errors.New("one"),
				errors.Join(errors.New("two"), errors.New("three")),
				errors.Join(errors.Join(errors.New("four"))),
			),
			want: "4 errors occurred:\n\n* one\n\n* two\n\n* three\n\n* four\n",
		},
		{
			name: "wrapper keeps its prefix",
			err:  fmt.Errorf("downloading source\n%w", errors.Join(errors.New("one"), errors.New("two"))),
			want: "downloading source\n2 errors occurred:\n\n* one\n\n* two\n",
		},
		{
			name: "wrapped join inside a join renders nested",
			err: errors.Join(
				errors.New("one"),
				fmt.Errorf("unit: %w", errors.Join(errors.New("two"))),
			),
			want: "2 errors occurred:\n\n* one\n\n* unit: error occurred:\n  \n  * two\n  \n",
		},
		{
			name: "wrapper with trailing text renders as its message",
			err:  fmt.Errorf("%w (retrying)", errors.Join(errors.New("one"), errors.New("two"))),
			want: "one\ntwo (retrying)",
		},
		{
			name: "multiple %w verbs render as their message",
			err:  fmt.Errorf("%w: %w", errors.New("sentinel"), errors.New("cause")),
			want: "sentinel: cause",
		},
		{
			name: "multiple %w verbs inside a join stay one bullet",
			err:  errors.Join(fmt.Errorf("%w: %w", errors.New("sentinel"), errors.New("cause"))),
			want: "error occurred:\n\n* sentinel: cause\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, errfmt.Format(tc.err))
		})
	}
}

func TestFormatBoundsRecursionDepth(t *testing.T) {
	t.Parallel()

	err := errors.Join(errors.New("a"), errors.New("b"))
	for range 1_000 {
		err = errors.Join(err)
	}

	assert.Equal(t, "error occurred:\n\n* a\n  b\n", errfmt.Format(err))
}
