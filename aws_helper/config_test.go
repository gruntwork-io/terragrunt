package aws_helper

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"gotest.tools/assert"
)

func TestCreateCustomResolver(t *testing.T) {
	testCases := []struct {
		name        string
		region      string
		stsEndpoint string
		s3Endpoint  string
	}{
		{

			"CustomSTSEndpoint",
			"us-west-2",
			"http://localhost",
			"",
		},
		{
			"CustomS3Endpoint",
			"us-east-1",
			"",
			"http://localhost",
		},
		{
			"CustomStsAndS3Endpoints",
			"us-east-1",
			"http://localhost:8080",
			"http://localhost:9090",
		},
	}

	for _, testCase := range testCases {
		// Capture range variable so that it is brought into the scope within the for loop, so that it is stable even
		// when subtests are run in parallel.
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			resolver := createCustomResolver(&AwsSessionConfig{
				Region:            testCase.region,
				CustomStsEndpoint: testCase.stsEndpoint,
				CustomS3Endpoint:  testCase.s3Endpoint,
			})

			// Build default S3 region-specific endpoint
			defaultS3Endpoint := fmt.Sprintf("https://s3.%s.amazonaws.com", testCase.region)
			// STS does not have region-specific endpoints by default
			// https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_enable-regions.html
			defaultStsEndpoint := "https://sts.amazonaws.com"

			// Grab the endpoint for S3 that our custom resolver is returning
			resolvedS3Endpoint, err := resolver("s3", testCase.region)
			require.NoError(t, err)

			// Grab the endpoint for STS that our custom resolver is returning
			resolvedStsEndpoint, err := resolver("sts", testCase.region)
			require.NoError(t, err)

			// If we defined a custom endpoint for STS, let's check it. Otherwise,
			// check that the value is the default provided by the SDK
			if testCase.stsEndpoint != "" {
				assert.Equal(t, testCase.stsEndpoint, resolvedStsEndpoint.URL)
			} else {
				assert.Equal(t, defaultStsEndpoint, resolvedStsEndpoint.URL)
			}

			// If we defined a custom endpoint for S3, let's check it. Otherwise,
			// check that the value is the default provided by the SDK
			if testCase.s3Endpoint != "" {
				assert.Equal(t, testCase.s3Endpoint, resolvedS3Endpoint.URL)
			} else {
				assert.Equal(t, defaultS3Endpoint, resolvedS3Endpoint.URL)
			}
		})
	}
}
