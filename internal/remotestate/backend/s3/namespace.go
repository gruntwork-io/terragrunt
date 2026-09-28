package s3

import (
	"regexp"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// accountRegionalSuffix ends every bucket name in an account regional namespace.
const accountRegionalSuffix = "-an"

// accountRegionalNameRegexp matches the naming convention S3 imposes on buckets in an account
// regional namespace: a prefix the caller chooses, the 12-digit ID of the account that owns the
// namespace, the region code, and the reserved suffix.
var accountRegionalNameRegexp = regexp.MustCompile(
	`^[a-z0-9][a-z0-9.-]*-\d{12}-([a-z0-9-]+)` + accountRegionalSuffix + `$`,
)

// BucketNamespace returns the S3 namespace that CreateBucket has to place the named bucket in.
//
// An empty return is the shared global namespace. It leaves the namespace out of the request
// entirely, so S3-compatible object stores never receive a header they don't understand.
//
// A name following the account regional convention is the exception. S3 reserves that namespace's
// suffix, so the name alone settles the choice. The region in the name has to be the one the bucket
// is created in, and any other region fails here, with an explanation of the convention, before S3
// sees the request.
func BucketNamespace(bucket, region string) (types.BucketNamespace, error) {
	match := accountRegionalNameRegexp.FindStringSubmatch(bucket)
	if match == nil {
		return "", nil
	}

	if match[1] != region {
		return "", InvalidAccountRegionalBucketName{Bucket: bucket, Region: region}
	}

	return types.BucketNamespaceAccountRegional, nil
}
