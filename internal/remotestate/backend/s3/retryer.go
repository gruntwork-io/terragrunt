package s3

import (
	"net/http"

	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/request"
)

type Retryer struct {
	client.DefaultRetryer
}

func (retryer Retryer) ShouldRetry(req *request.Request) bool {
	if req.HTTPResponse.StatusCode == http.StatusBadRequest {
		return true
	}

	return retryer.DefaultRetryer.ShouldRetry(req)
}
