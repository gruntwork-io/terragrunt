package aws_helper_test

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerragruntIsAddedInUserAgent(t *testing.T) {
	t.Parallel()

	sess, err := aws_helper.CreateAwsSession(nil, options.NewTerragruntOptions())
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

	err := aws_helper.ValidateAwsSession(&aws_helper.AwsSessionConfig{
		Region:        "not-existing-region",
		CredsFilename: "/tmp/not-existing-file",
	}, options.NewTerragruntOptions())
	assert.Error(t, err)
}
