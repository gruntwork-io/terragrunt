//go:build aws

package awshelper_test

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/terragrunt/awshelper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwsIsAddedInUserAgent(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	sess, err := awshelper.CreateAwsSession(l, nil, options.NewTerragruntOptions())
	require.NoError(t, err)

	op := &request.Operation{
		Name:       "",
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}
	input := &sts.GetCallerIdentityInput{}
	output := &sts.GetCallerIdentityOutput{}

	r := sts.New(sess).NewRequest(op, input, output)
	sess.Handlers.Build.Run(r)

	assert.Contains(t, r.HTTPRequest.Header.Get("User-Agent"), "terragrunt")
}

func TestAwsSessionValidationFail(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	sess, err := awshelper.CreateAwsSession(l, &awshelper.AwsSessionConfig{
		Region:        "not-existing-region",
		CredsFilename: "/tmp/not-existing-file",
	}, options.NewTerragruntOptions())
	require.NoError(t, err)

	err = awshelper.ValidateAwsSession(sess)
	assert.Error(t, err)
}

// Test to validate cases when is not possible to read all S3 configurations
// https://github.com/gruntwork-io/terragrunt/issues/2109
func TestAwsNegativePublicAccessResponse(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		response *s3.GetPublicAccessBlockOutput
		name     string
	}{
		{
			name: "nil-response",
			response: &s3.GetPublicAccessBlockOutput{
				PublicAccessBlockConfiguration: nil,
			},
		},
		{
			name: "legacy-bucket",
			response: &s3.GetPublicAccessBlockOutput{
				PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
					BlockPublicAcls:       nil,
					BlockPublicPolicy:     nil,
					IgnorePublicAcls:      nil,
					RestrictPublicBuckets: nil,
				},
			},
		},
		{
			name: "false-response",
			response: &s3.GetPublicAccessBlockOutput{
				PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
					BlockPublicAcls:       aws.Bool(false),
					BlockPublicPolicy:     aws.Bool(false),
					IgnorePublicAcls:      aws.Bool(false),
					RestrictPublicBuckets: aws.Bool(false),
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			response, err := awshelper.ValidatePublicAccessBlock(tc.response)
			require.NoError(t, err)
			assert.False(t, response)
		})
	}
}
