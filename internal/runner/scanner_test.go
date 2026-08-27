package runner_test

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubstringScanner(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name    string
		needles []string
		writes  []string
		want    bool
	}{
		{
			name:    "both needles in one write",
			needles: []string{"alpha", "beta"},
			writes:  []string{"xx alpha yy beta zz"},
			want:    true,
		},
		{
			name:    "needles arrive in separate writes",
			needles: []string{"alpha", "beta"},
			writes:  []string{"xx alpha ", "yy beta zz"},
			want:    true,
		},
		{
			name:    "needle straddles a write boundary",
			needles: []string{"alpha"},
			writes:  []string{"xx al", "pha yy"},
			want:    true,
		},
		{
			name:    "needle arrives one byte per write",
			needles: []string{"alpha"},
			writes:  []string{"a", "l", "p", "h", "a"},
			want:    true,
		},
		{
			name:    "needle straddles many writes with filler between",
			needles: []string{"alpha"},
			writes:  []string{"filler al", "p", "ha"},
			want:    true,
		},
		{
			name:    "one needle never appears",
			needles: []string{"alpha", "beta"},
			writes:  []string{"xx alpha yy"},
			want:    false,
		},
		{
			name:    "nothing written",
			needles: []string{"alpha"},
			writes:  nil,
			want:    false,
		},
		{
			name:    "empty needles are dropped",
			needles: []string{"", "alpha"},
			writes:  []string{"alpha"},
			want:    true,
		},
		{
			name:    "a scanner with no needles has nothing left to find",
			needles: nil,
			writes:  []string{"anything"},
			want:    true,
		},
		{
			name:    "single byte needle cannot straddle a boundary",
			needles: []string{"x"},
			writes:  []string{"aaa", "bxb"},
			want:    true,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scanner := runner.NewSubstringScanner(tt.needles...)

			for _, write := range tt.writes {
				n, err := scanner.Write([]byte(write))
				require.NoError(t, err)
				assert.Equal(t, len(write), n)
			}

			assert.Equal(t, tt.want, scanner.FoundAll())
		})
	}
}

func TestSubstringScannerMatchesAfterLongStream(t *testing.T) {
	t.Parallel()

	scanner := runner.NewSubstringScanner("alpha")

	chunk := []byte("filler filler filler")
	for range 1000 {
		_, err := scanner.Write(chunk)
		require.NoError(t, err)
	}

	// A needle arriving only after a long run of unrelated output still matches,
	// which is the property the fixed-size retention has to preserve.
	_, err := scanner.Write([]byte("alpha"))
	require.NoError(t, err)
	assert.True(t, scanner.FoundAll())
}

// The markers a unit's plan stderr is watched for. Their length decides how much
// the scanner has to retain between writes.
const (
	benchNeedlePrefix   = "Error running plan:"
	benchNeedleResource = ": Resource 'data.terraform_remote_state."
)

// planStderrChunk mimics one read from a unit's stderr during a plan. The lines
// are warnings and progress, so none of them match either marker.
func planStderrChunk(lines int) []byte {
	var sb strings.Builder

	for i := range lines {
		sb.WriteString("Warning: Argument is deprecated on main.tf line ")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(", in resource \"aws_s3_bucket\" \"logs\"\n")
	}

	return []byte(sb.String())
}

// BenchmarkPlanErrorScan contrasts scanning a unit's stderr as it streams against
// accumulating the stream and testing it once at the end, which is what the runner
// did before. Read the bytes-per-operation column. The scanner's figure holds
// steady as the stream grows.
func BenchmarkPlanErrorScan(b *testing.B) {
	chunk := planStderrChunk(32)

	for _, chunks := range []int{16, 256, 4096} {
		total := int64(len(chunk) * chunks)

		b.Run("scanner/"+strconv.Itoa(chunks)+"chunks", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(total)

			for b.Loop() {
				scanner := runner.NewSubstringScanner(
					benchNeedlePrefix,
					benchNeedleResource,
				)

				for range chunks {
					if _, err := scanner.Write(chunk); err != nil {
						b.Fatal(err)
					}
				}

				if scanner.FoundAll() {
					b.Fatal("benchmark input should not match")
				}
			}
		})

		b.Run("buffered/"+strconv.Itoa(chunks)+"chunks", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(total)

			for b.Loop() {
				buf := &bytes.Buffer{}

				for range chunks {
					if _, err := buf.Write(chunk); err != nil {
						b.Fatal(err)
					}
				}

				output := buf.String()
				if strings.Contains(output, benchNeedlePrefix) &&
					strings.Contains(output, benchNeedleResource) {
					b.Fatal("benchmark input should not match")
				}
			}
		})
	}
}
