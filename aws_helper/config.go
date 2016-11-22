package aws_helper

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/gruntwork-io/terragrunt/errors"
)

// Returns an AWS config object for the given region, ensuring that the config has credentials
func CreateAwsConfig(awsRegion string) (*aws.Config, error) {
	config := defaults.Get().Config.WithRegion(awsRegion)

	_, err := config.Credentials.Get()
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?)")
	}

	return config, nil
}
