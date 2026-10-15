package helpers_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The AWS client constructors read the process environment, so these tests point config files at missing paths.

//nolint:paralleltest // pkgTIsolateAWSConfig calls t.Setenv on the process environment.
func TestCreateS3ClientForTest(t *testing.T) {
	pkgTIsolateAWSConfig(t)

	applied := false
	withRole := func(opts *options.TerragruntOptions) {
		applied = true
		opts.IAMRoleOptions.RoleARN = "arn:aws:iam::123456789012:role/terragrunt-test"
	}

	client := helpers.CreateS3ClientForTest(t, "eu-central-1", withRole)

	require.NotNil(t, client)
	assert.True(t, applied, "every option func is applied to the options")
	assert.Equal(t, "eu-central-1", client.Options().Region)
	assert.NotNil(t, client.Options().Credentials)
}

//nolint:paralleltest // pkgTIsolateAWSConfig calls t.Setenv on the process environment.
func TestCreateDynamoDBClientForTest(t *testing.T) {
	pkgTIsolateAWSConfig(t)

	testCases := []struct {
		name    string
		region  string
		roleArn string
	}{
		{
			name:   "default credentials",
			region: "us-east-2",
		},
		{
			name:    "assumed role",
			region:  "ap-southeast-1",
			roleArn: "arn:aws:iam::123456789012:role/terragrunt-test",
		},
	}

	for _, tc := range testCases {
		client := helpers.CreateDynamoDBClientForTest(t, tc.region, "", tc.roleArn)

		require.NotNil(t, client, tc.name)
		assert.Equal(t, tc.region, client.Options().Region, tc.name)
	}
}

func TestDeleteS3Bucket(t *testing.T) {
	const bucket = "tg-test-bucket"

	firstPage := pkgTS3Page{
		versions: []pkgTS3Entry{{Key: "a", VersionID: "1"}, {Key: "a", VersionID: "2"}},
		markers:  []pkgTS3Entry{{Key: "b", VersionID: "3"}},
	}
	lastPage := pkgTS3Page{versions: []pkgTS3Entry{{Key: "c", VersionID: "4"}}}
	refused := pkgTS3Entry{Key: "c", VersionID: "4"}

	testCases := []struct {
		s3        *pkgTFakeS3
		name      string
		wantCalls []string
	}{
		{
			name:      "missing bucket needs no cleanup",
			s3:        &pkgTFakeS3{headStatus: http.StatusNotFound},
			wantCalls: []string{"HeadBucket"},
		},
		{
			name: "versions and delete markers are removed page by page",
			s3:   &pkgTFakeS3{headStatus: http.StatusOK, listPages: []pkgTS3Page{firstPage, lastPage}},
			wantCalls: []string{
				"HeadBucket", "ListObjectVersions", "DeleteObjects", "ListObjectVersions", "DeleteObjects", "DeleteBucket",
			},
		},
		{
			name: "head failure is tolerated and a bucket gone before cleanup is done",
			s3: &pkgTFakeS3{
				headStatus:        http.StatusForbidden,
				listCode:          "NoSuchBucket",
				deleteBucketCodes: []string{"NoSuchBucket"},
			},
			wantCalls: []string{"HeadBucket", "ListObjectVersions", "DeleteBucket"},
		},
		{
			name: "bucket reported not empty is cleaned and deleted again",
			s3:   &pkgTFakeS3{headStatus: http.StatusOK, deleteBucketCodes: []string{"BucketNotEmpty", ""}},
			wantCalls: []string{
				"HeadBucket", "ListObjectVersions", "DeleteBucket", "ListObjectVersions", "DeleteBucket",
			},
		},
		{
			name: "a version S3 refuses to delete once is retried after the bucket reports not empty",
			s3:   &pkgTFakeS3{headStatus: http.StatusOK, listPages: []pkgTS3Page{lastPage}, refuseOnce: []pkgTS3Entry{refused}},
			wantCalls: []string{
				"HeadBucket", "ListObjectVersions", "DeleteObjects", "DeleteBucket",
				"ListObjectVersions", "DeleteObjects", "DeleteBucket",
			},
		},
		{
			name: "bucket gone during the second delete is done",
			s3:   &pkgTFakeS3{headStatus: http.StatusOK, deleteBucketCodes: []string{"BucketNotEmpty", "NoSuchBucket"}},
			wantCalls: []string{
				"HeadBucket", "ListObjectVersions", "DeleteBucket", "ListObjectVersions", "DeleteBucket",
			},
		},
		{
			name: "bucket gone while deleting ends the cleanup",
			s3: &pkgTFakeS3{
				headStatus:        http.StatusOK,
				listPages:         []pkgTS3Page{firstPage},
				deleteObjectsCode: "NoSuchBucket",
				deleteBucketCodes: []string{"NoSuchBucket"},
			},
			wantCalls: []string{"HeadBucket", "ListObjectVersions", "DeleteObjects", "DeleteBucket"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pkgTIsolateAWSConfig(t)
			t.Setenv("AWS_ACCESS_KEY_ID", "test")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

			tc.s3.t = t
			tc.s3.bucket = bucket

			server := httptest.NewServer(tc.s3)
			t.Cleanup(server.Close)

			// Both endpoint settings are pinned so no inherited value wins, the ignore flag would switch them
			// off, and FIPS or dual-stack cannot be combined with a custom endpoint.
			t.Setenv("AWS_ENDPOINT_URL", server.URL)
			t.Setenv("AWS_ENDPOINT_URL_S3", server.URL)
			t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "false")
			t.Setenv("AWS_USE_FIPS_ENDPOINT", "false")
			t.Setenv("AWS_USE_DUALSTACK_ENDPOINT", "false")

			require.NoError(t, helpers.DeleteS3Bucket(t, "us-east-1", bucket))
			assert.Equal(t, tc.wantCalls, tc.s3.recorded(), "S3 calls in order")
			assert.Empty(t, tc.s3.pendingDeletes(), "every listed object was deleted")
		})
	}
}

// pkgTIsolateAWSConfig points the AWS SDK at missing config files and clears the profile, so local setup is ignored.
func pkgTIsolateAWSConfig(t *testing.T) {
	t.Helper()

	dir := t.TempDir()

	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
}

// pkgTS3Entry is one object version or delete marker, as S3 lists it and as DeleteObjects names it.
type pkgTS3Entry struct {
	Key       string `xml:"Key"`
	VersionID string `xml:"VersionId"`
}

// pkgTS3Page is the content of one ListObjectVersions result.
type pkgTS3Page struct {
	versions []pkgTS3Entry
	markers  []pkgTS3Entry
}

// last returns the entry the page ends with, which S3 names as the marker for the next page, or a zero
// entry for an empty page.
func (p pkgTS3Page) last() pkgTS3Entry {
	if len(p.markers) > 0 {
		return p.markers[len(p.markers)-1]
	}

	if len(p.versions) > 0 {
		return p.versions[len(p.versions)-1]
	}

	return pkgTS3Entry{}
}

// pkgTS3ListResult is the ListObjectVersions response document.
type pkgTS3ListResult struct {
	XMLName             xml.Name      `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListVersionsResult"`
	NextKeyMarker       string        `xml:"NextKeyMarker,omitempty"`
	NextVersionIDMarker string        `xml:"NextVersionIdMarker,omitempty"`
	Versions            []pkgTS3Entry `xml:"Version"`
	DeleteMarkers       []pkgTS3Entry `xml:"DeleteMarker"`
	IsTruncated         bool          `xml:"IsTruncated"`
}

// pkgTS3DeleteRequest is the DeleteObjects request document.
type pkgTS3DeleteRequest struct {
	XMLName xml.Name      `xml:"Delete"`
	Objects []pkgTS3Entry `xml:"Object"`
}

// pkgTS3DeleteResult is the DeleteObjects response document.
type pkgTS3DeleteResult struct {
	XMLName xml.Name            `xml:"http://s3.amazonaws.com/doc/2006-03-01/ DeleteResult"`
	Deleted []pkgTS3Entry       `xml:"Deleted"`
	Errors  []pkgTS3DeleteError `xml:"Error"`
}

// pkgTS3DeleteError is one object a DeleteObjects call refused to delete.
type pkgTS3DeleteError struct {
	pkgTS3Entry

	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// pkgTFakeS3 answers the calls DeleteS3Bucket makes on one bucket with scripted outcomes and records their order.
// A call for another bucket, a listing that does not continue from the last page, or a DeleteObjects that does
// not name exactly the objects of the last page fails the test. Objects in refuseOnce are answered with a
// per-object error the first time, as S3 does for a version the caller may not delete; they stay in the
// bucket, are listed again once the scripted pages run out, and keep DeleteBucket answering BucketNotEmpty.
type pkgTFakeS3 struct {
	t *testing.T
	// bucket is the only bucket the fake serves.
	bucket string
	// listCode, when set, is the error code every ListObjectVersions fails with.
	listCode string
	// deleteObjectsCode, when set, is the error code every DeleteObjects fails with.
	deleteObjectsCode string
	// wantKeyMarker and wantVersionMarker are the markers the next ListObjectVersions must carry.
	wantKeyMarker     string
	wantVersionMarker string
	calls             []string
	// listPages are the ListObjectVersions results in order; once used up the listing is empty.
	listPages []pkgTS3Page
	// wantDeletes are the object sets the next DeleteObjects calls must name, in order.
	wantDeletes [][]pkgTS3Entry
	// refuseOnce are objects whose next DeleteObjects answers a per-object error instead of deleting them.
	refuseOnce []pkgTS3Entry
	// leftover holds the refused objects still in the bucket.
	leftover pkgTS3Page
	// deleteBucketCodes are the DeleteBucket outcomes in order, where an empty code succeeds.
	deleteBucketCodes []string
	// headStatus is the HeadBucket response status.
	headStatus int
	mu         sync.Mutex
}

func (f *pkgTFakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// smithy-go releases disagree on whether a bucket-level path ends in a slash.
	assert.Equal(f.t, "/"+f.bucket, strings.TrimSuffix(r.URL.Path, "/"), "every call targets the bucket under cleanup")

	switch {
	case r.Method == http.MethodHead:
		f.calls = append(f.calls, "HeadBucket")
		w.WriteHeader(f.headStatus)
	case r.Method == http.MethodGet && r.URL.Query().Has("versions"):
		f.calls = append(f.calls, "ListObjectVersions")
		f.listVersions(w, r)
	case r.Method == http.MethodPost && r.URL.Query().Has("delete"):
		f.calls = append(f.calls, "DeleteObjects")
		f.deleteObjects(w, r)
	case r.Method == http.MethodDelete:
		f.calls = append(f.calls, "DeleteBucket")

		code := ""
		if len(f.deleteBucketCodes) > 0 {
			code, f.deleteBucketCodes = f.deleteBucketCodes[0], f.deleteBucketCodes[1:]
		}

		if code == "" && len(f.leftover.versions) > 0 {
			code = "BucketNotEmpty"
		}

		if code != "" {
			f.writeError(w, code)

			return
		}

		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.String(), http.StatusNotImplemented)
	}
}

// listVersions serves the next scripted page to a request that continues from where the previous page stopped.
func (f *pkgTFakeS3) listVersions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	assert.Equal(f.t, f.wantKeyMarker, query.Get("key-marker"), "key marker continues the listing")
	assert.Equal(f.t, f.wantVersionMarker, query.Get("version-id-marker"), "version marker continues the listing")

	if f.listCode != "" {
		f.writeError(w, f.listCode)

		return
	}

	page := f.nextPage()

	if listed := slices.Concat(page.versions, page.markers); len(listed) > 0 {
		f.wantDeletes = append(f.wantDeletes, listed)
	}

	result := pkgTS3ListResult{
		Versions:      page.versions,
		DeleteMarkers: page.markers,
		IsTruncated:   len(f.listPages) > 0,
	}

	// S3 continues a truncated listing after the last entry it returned.
	f.wantKeyMarker, f.wantVersionMarker = "", ""

	if result.IsTruncated {
		last := page.last()
		result.NextKeyMarker, result.NextVersionIDMarker = last.Key, last.VersionID
		f.wantKeyMarker, f.wantVersionMarker = last.Key, last.VersionID
	}

	f.write(w, result)
}

// nextPage returns the next scripted page, or the refused objects still in the bucket once the script runs out.
func (f *pkgTFakeS3) nextPage() pkgTS3Page {
	if len(f.listPages) > 0 {
		page := f.listPages[0]
		f.listPages = f.listPages[1:]

		return page
	}

	page := f.leftover
	f.leftover = pkgTS3Page{}

	return page
}

// deleteObjects checks that the request names exactly the objects of the last listed page, then answers it.
func (f *pkgTFakeS3) deleteObjects(w http.ResponseWriter, r *http.Request) {
	var request pkgTS3DeleteRequest

	assert.NoError(f.t, xml.NewDecoder(r.Body).Decode(&request), "decode DeleteObjects body")

	want := []pkgTS3Entry(nil)
	if assert.NotEmpty(f.t, f.wantDeletes, "DeleteObjects follows a listed page") {
		want, f.wantDeletes = f.wantDeletes[0], f.wantDeletes[1:]
	}

	assert.Equal(f.t, want, request.Objects, "DeleteObjects names the listed objects")

	if f.deleteObjectsCode != "" {
		f.writeError(w, f.deleteObjectsCode)

		return
	}

	result := pkgTS3DeleteResult{}

	for _, object := range request.Objects {
		if i := slices.Index(f.refuseOnce, object); i >= 0 {
			f.refuseOnce = slices.Delete(f.refuseOnce, i, i+1)
			f.leftover.versions = append(f.leftover.versions, object)
			result.Errors = append(result.Errors, pkgTS3DeleteError{pkgTS3Entry: object, Code: "AccessDenied", Message: "Access Denied"})

			continue
		}

		result.Deleted = append(result.Deleted, object)
	}

	f.write(w, result)
}

// recorded returns the calls answered so far, in order.
func (f *pkgTFakeS3) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.calls...)
}

// pendingDeletes returns the listed object sets no DeleteObjects call has named yet.
func (f *pkgTFakeS3) pendingDeletes() [][]pkgTS3Entry {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([][]pkgTS3Entry(nil), f.wantDeletes...)
}

// write sends result as the XML document of a successful call.
func (f *pkgTFakeS3) write(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/xml")

	_, err := fmt.Fprint(w, xml.Header)
	assert.NoError(f.t, err, "write fake S3 response header")
	assert.NoError(f.t, xml.NewEncoder(w).Encode(result), "write fake S3 response")
}

// writeError sends an S3 error document with the status that code carries on the real service.
func (f *pkgTFakeS3) writeError(w http.ResponseWriter, code string) {
	statuses := map[string]int{"NoSuchBucket": http.StatusNotFound, "BucketNotEmpty": http.StatusConflict}

	status, ok := statuses[code]
	if !assert.True(f.t, ok, "scripted S3 error code %q", code) {
		status = http.StatusBadRequest
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)

	_, err := fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message></Error>`, code, code)
	assert.NoError(f.t, err, "write fake S3 error")
}
