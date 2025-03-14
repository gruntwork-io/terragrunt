package s3_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
)

func TestConfig_IsEqual(t *testing.T) {
	t.Parallel()

	logger := log.New()

	testCases := []struct {
		name          string
		cfg           s3.Config
		comparableCfg s3.Config
		shouldBeEqual bool
	}{
		{
			"equal-both-empty",
			s3.Config{},
			s3.Config{},
			true,
		},
		{
			"equal-empty-and-nil",
			s3.Config{},
			nil,
			true,
		},
		{
			"equal-one-key",
			s3.Config{"foo": "bar"},
			s3.Config{"foo": "bar"},
			true,
		},
		{
			"equal-multiple-keys",
			s3.Config{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			s3.Config{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			true,
		},
		{
			"equal-encrypt-bool-handling",
			s3.Config{"encrypt": true},
			s3.Config{"encrypt": "true"},
			true,
		},
		{
			"equal-general-bool-handling",
			s3.Config{"something": true, "encrypt": true},
			s3.Config{"something": "true", "encrypt": "true"},
			true,
		},
		{
			"equal-ignore-s3-tags",
			s3.Config{"foo": "bar", "s3_bucket_tags": []map[string]string{{"foo": "bar"}}},
			s3.Config{"foo": "bar"},
			true,
		},
		{
			"equal-ignore-dynamodb-tags",
			s3.Config{"dynamodb_table_tags": []map[string]string{{"foo": "bar"}}},
			s3.Config{},
			true,
		},
		{
			"equal-ignore-accesslogging-options",
			s3.Config{
				"accesslogging_bucket_tags":                         []map[string]string{{"foo": "bar"}},
				"accesslogging_target_object_partition_date_source": "EventTime",
				"accesslogging_target_prefix":                       "test/",
				"skip_accesslogging_bucket_acl":                     false,
				"skip_accesslogging_bucket_enforced_tls":            false,
				"skip_accesslogging_bucket_public_access_blocking":  false,
				"skip_accesslogging_bucket_ssencryption":            false,
			},
			s3.Config{},
			true,
		},
		{
			"unequal-values",
			s3.Config{"foo": "bar"},
			s3.Config{"foo": "different"},
			false,
		},
		{
			"unequal-non-empty-config-nil",
			s3.Config{"foo": "bar"},
			nil,
			false,
		},
		{
			"unequal-general-bool-handling",
			s3.Config{"something": true},
			s3.Config{"something": "false"},
			false,
		},
		{
			"equal-null-ignored",
			s3.Config{"something": "foo"},
			s3.Config{"something": "foo", "ignored-because-null": nil},
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
