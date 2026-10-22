package tf_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodePlanChanges pins that the counts match the summary line of
// `tofu plan`: a replace counts once as an add and once as a destroy, an
// import counts whether or not the resource also changes, and only outputs
// whose values change count.
func TestDecodePlanChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
		want tf.PlanChanges
	}{
		{
			name: "no changes",
			json: `{"resource_changes":[{"address":"a.one","change":{"actions":["no-op"]}}],` +
				`"output_changes":{"id":{"actions":["no-op"]}}}`,
			want: tf.PlanChanges{},
		},
		{
			name: "every kind of change",
			json: `{"resource_changes":[` +
				`{"address":"a.one","change":{"actions":["create"]}},` +
				`{"address":"a.two","change":{"actions":["update"],"importing":{"id":"two"}}},` +
				`{"address":"a.three","change":{"actions":["delete","create"]}},` +
				`{"address":"a.four","change":{"actions":["forget"]}},` +
				`{"address":"a.five","change":{"actions":["no-op"],"importing":{"id":"five"}}}],` +
				`"output_changes":{"id":{"actions":["update"]},"name":{"actions":["create"]},"same":{"actions":["no-op"]}}}`,
			want: tf.PlanChanges{Add: 2, Change: 1, Destroy: 1, Import: 2, Forget: 1, Outputs: 2},
		},
		{
			name: "a plan without resource or output changes",
			json: `{"format_version":"1.2","terraform_version":"1.9.0"}`,
			want: tf.PlanChanges{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tf.DecodePlanChanges(strings.NewReader(tc.json))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestDecodePlanChangesRejectsWhatIsNotJSON pins that a plan file that is not
// JSON is an error, not a plan with no changes.
func TestDecodePlanChangesRejectsWhatIsNotJSON(t *testing.T) {
	t.Parallel()

	_, err := tf.DecodePlanChanges(
		strings.NewReader("No changes. Your infrastructure matches the configuration.\n"),
	)
	require.Error(t, err)
}
