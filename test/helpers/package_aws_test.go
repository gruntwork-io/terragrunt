package helpers_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	markerPage := pkgTS3Page{markers: []pkgTS3Entry{{Key: "d", VersionID: "5"}}}

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
				"HeadBucket", "ListObjectVersions", "DeleteObjects", "DeleteObjects",
				"ListObjectVersions", "DeleteObjects", "DeleteBucket",
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
			name: "bucket gone during the second delete is done",
			s3:   &pkgTFakeS3{headStatus: http.StatusOK, deleteBucketCodes: []string{"BucketNotEmpty", "NoSuchBucket"}},
			wantCalls: []string{
				"HeadBucket", "ListObjectVersions", "DeleteBucket", "ListObjectVersions", "DeleteBucket",
			},
		},
		{
			name: "bucket gone while deleting versions ends the cleanup",
			s3: &pkgTFakeS3{
				headStatus:        http.StatusOK,
				listPages:         []pkgTS3Page{lastPage},
				deleteObjectsCode: "NoSuchBucket",
			},
			wantCalls: []string{"HeadBucket", "ListObjectVersions", "DeleteObjects", "DeleteBucket"},
		},
		{
			name: "bucket gone while deleting markers ends the cleanup",
			s3: &pkgTFakeS3{
				headStatus:        http.StatusOK,
				listPages:         []pkgTS3Page{markerPage},
				deleteObjectsCode: "NoSuchBucket",
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

			// The S3-specific setting outranks the general one, and the ignore flag would switch both off.
			t.Setenv("AWS_ENDPOINT_URL", server.URL)
			t.Setenv("AWS_ENDPOINT_URL_S3", server.URL)
			t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "false")

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

// last returns the entry the page ends with, which S3 names as the marker for the next page.
func (p pkgTS3Page) last() pkgTS3Entry {
	if len(p.markers) > 0 {
		return p.markers[len(p.markers)-1]
	}

	return p.versions[len(p.versions)-1]
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

// pkgTFakeS3 answers the calls DeleteS3Bucket makes on one bucket with scripted outcomes and records their order.
// A call for another bucket, a listing that does not continue from the last page, or a DeleteObjects that does
// not name exactly the objects of the last page fails the test.
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
	// deleteBucketCodes are the DeleteBucket outcomes in order, where an empty code succeeds.
	deleteBucketCodes []string
	// headStatus is the HeadBucket response status.
	headStatus int
	mu         sync.Mutex
}

func (f *pkgTFakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	assert.Equal(f.t, "/"+f.bucket+"/", r.URL.Path, "every call targets the bucket under cleanup")

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

	page := pkgTS3Page{}
	if len(f.listPages) > 0 {
		page, f.listPages = f.listPages[0], f.listPages[1:]
	}

	if len(page.versions) > 0 {
		f.wantDeletes = append(f.wantDeletes, page.versions)
	}

	if len(page.markers) > 0 {
		f.wantDeletes = append(f.wantDeletes, page.markers)
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

	f.write(w, struct {
		XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ DeleteResult"`
	}{})
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

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statuses[code])

	_, err := fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message></Error>`, code, code)
	assert.NoError(f.t, err, "write fake S3 error")
}
