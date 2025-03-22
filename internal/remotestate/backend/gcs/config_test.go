package gcs_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
)

func TestConfig_IsEqual(t *testing.T) {
	t.Parallel()

	logger := log.New()

	testCases := []struct { //nolint: govet
		name          string
		cfg           gcs.Config
		comparableCfg gcs.Config
		shouldBeEqual bool
	}{
		{
			"equal-both-empty",
			gcs.Config{},
			gcs.Config{},
			true,
		},
		{
			"equal-empty-and-nil",
			gcs.Config{},
			nil,
			true,
		},
		{
			"equal-one-key",
			gcs.Config{"foo": "bar"},
			gcs.Config{"foo": "bar"},
			true,
		},
		{
			"equal-multiple-keys",
			gcs.Config{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			gcs.Config{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			true,
		},
		{
			"equal-encrypt-bool-handling",
			gcs.Config{"encrypt": true},
			gcs.Config{"encrypt": "true"},
			true,
		},
		{
			"equal-general-bool-handling",
			gcs.Config{"something": true, "encrypt": true},
			gcs.Config{"something": "true", "encrypt": "true"},
			true,
		},
		{
			"equal-ignore-gcs-labels",
			gcs.Config{"foo": "bar", "gcs_bucket_labels": []map[string]string{{"foo": "bar"}}},
			gcs.Config{"foo": "bar"},
			true,
		},
		{
			"unequal-values",
			gcs.Config{"foo": "bar"},
			gcs.Config{"foo": "different"},
			false,
		},
		{
			"unequal-non-empty-cfg-nil",
			gcs.Config{"foo": "bar"},
			nil,
			false,
		},
		{
			"unequal-general-bool-handling",
			gcs.Config{"something": true},
			gcs.Config{"something": "false"},
			false,
		},
		{
			"equal-null-ignored",
			gcs.Config{"something": "foo"},
			gcs.Config{"something": "foo", "ignored-because-null": nil},
			true,
		},
		{
			"terragrunt-only-configs-remain-intact",
			gcs.Config{"something": "foo", "skip_bucket_creation": true},
			gcs.Config{"something": "foo"},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual := testCase.cfg.IsEqual(testCase.comparableCfg, logger)
			assert.Equal(t, testCase.shouldBeEqual, actual)
		})
	}
}
