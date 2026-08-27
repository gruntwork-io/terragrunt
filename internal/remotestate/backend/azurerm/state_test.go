package azurerm_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStateBlobClientPreservesAzurermAuthorizationPolicy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		wantBlobAuthPrefix string
		armStatus          int
		wantErr            bool
	}{
		{
			name:               "account key lookup succeeds",
			armStatus:          http.StatusOK,
			wantBlobAuthPrefix: "SharedKey stateaccount:",
		},
		{
			name:      "account key lookup failure remains fatal",
			armStatus: http.StatusForbidden,
			wantErr:   true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			transport := &stateTransport{armStatus: testCase.armStatus}
			cfg := stateAzureConfig(transport)

			ctx := azurerm.WithStateClientCache(t.Context())
			client, err := azurerm.NewStateBlobClient(ctx, logger.CreateLogger(), cfg)

			if testCase.wantErr {
				var setupErr *azurerm.StateClientSetupError
				require.ErrorAs(t, err, &setupErr)

				var responseErr *azcore.ResponseError
				require.ErrorAs(t, err, &responseErr)
				assert.Equal(t, "AuthorizationFailed", responseErr.ErrorCode)
				assert.Equal(t, http.StatusForbidden, responseErr.StatusCode)
				require.Len(t, transport.Requests(), 1)

				return
			}

			require.NoError(t, err)

			body, err := client.Container("state-container").GetBlob(
				t.Context(),
				"environment/terraform.tfstate",
			)
			require.NoError(t, err)

			got, err := io.ReadAll(body)
			require.NoError(t, err)
			require.NoError(t, body.Close())
			assert.JSONEq(t, azureStateBody, string(got))

			requests := transport.Requests()
			require.Len(t, requests, 2)
			assert.Contains(t, requests[0].URL.Path, "listKeys")
			assert.Contains(t, requests[1].URL.Path, "/state-container/environment/terraform.tfstate")
			assert.True(t, strings.HasPrefix(
				requests[1].Header.Get("Authorization"),
				testCase.wantBlobAuthPrefix,
			))
		})
	}
}

func TestNewStateBlobClientPreservesNotFoundError(t *testing.T) {
	t.Parallel()

	transport := &stateTransport{blobStatus: http.StatusNotFound}
	cfg := stateAzureConfig(transport)
	cfg.UseAzureADAuth = true

	client, err := azurerm.NewStateBlobClient(t.Context(), logger.CreateLogger(), cfg)
	require.NoError(t, err)

	_, err = client.Container("state-container").GetBlob(t.Context(), "missing.tfstate")
	require.Error(t, err)
	assert.True(t, azurehelper.IsNotFound(err))
}

func TestNewStateBlobClientRequiresConfig(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, azurehelper.ErrAzureConfigRequired, func() {
		_, _ = azurerm.NewStateBlobClient(t.Context(), logger.CreateLogger(), nil)
	})
}

func TestNewStateBlobClientRequiresStateClientCache(t *testing.T) {
	t.Parallel()

	transport := &stateTransport{}

	_, err := azurerm.NewStateBlobClient(t.Context(), logger.CreateLogger(), stateAzureConfig(transport))
	require.Error(t, err)

	var setupErr *azurerm.StateClientSetupError
	require.ErrorAs(t, err, &setupErr)

	var cacheErr azurerm.StateClientCacheRequiredError
	require.ErrorAs(t, err, &cacheErr)
	assert.Empty(t, transport.Requests(), "an unscoped state client must not make an ARM request")
}

const azureStateBody = `{"version":4,"outputs":{"value":{"sensitive":false,"type":"string","value":"azure"}}}`

type stateCredential struct{}

func (stateCredential) GetToken(
	_ context.Context,
	_ policy.TokenRequestOptions,
) (azcore.AccessToken, error) {
	return azcore.AccessToken{
		Token:     "test-token",
		ExpiresOn: time.Now().Add(time.Hour),
	}, nil
}

type stateTransport struct {
	// beforeARM runs before a listKeys response, letting a test hold every caller
	// at the same point to prove concurrent misses coalesce.
	beforeARM  func()
	requests   []*http.Request
	armStatus  int
	blobStatus int
	mu         sync.Mutex
}

func (transport *stateTransport) Do(req *http.Request) (*http.Response, error) {
	transport.mu.Lock()
	transport.requests = append(transport.requests, req.Clone(req.Context()))
	transport.mu.Unlock()

	if transport.beforeARM != nil && strings.Contains(req.URL.Path, "listKeys") {
		transport.beforeARM()
	}

	if strings.Contains(req.URL.Path, "listKeys") {
		status := transport.armStatus
		if status == 0 {
			status = http.StatusOK
		}

		if status != http.StatusOK {
			return stateResponse(
				req,
				status,
				`{"error":{"code":"AuthorizationFailed","message":"denied"}}`,
				"AuthorizationFailed",
			), nil
		}

		key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x2a}, 32))

		return stateResponse(
			req,
			http.StatusOK,
			`{"keys":[{"permissions":"FULL","value":"`+key+`"}]}`,
			"",
		), nil
	}

	status := transport.blobStatus
	if status == 0 {
		status = http.StatusOK
	}

	if status == http.StatusNotFound {
		return stateResponse(
			req,
			status,
			`<?xml version="1.0" encoding="utf-8"?><Error><Code>BlobNotFound</Code><Message>missing</Message></Error>`,
			"BlobNotFound",
		), nil
	}

	return stateResponse(req, status, azureStateBody, ""), nil
}

func (transport *stateTransport) Requests() []*http.Request {
	transport.mu.Lock()
	defer transport.mu.Unlock()

	return append([]*http.Request(nil), transport.requests...)
}

func stateAzureConfig(transport policy.Transporter) *azurehelper.AzureConfig {
	return &azurehelper.AzureConfig{
		Credential:     stateCredential{},
		SubscriptionID: "00000000-0000-0000-0000-000000000000",
		ResourceGroup:  "state-group",
		AccountName:    "stateaccount",
		CloudConfig:    cloud.AzurePublic,
		ClientOptions: policy.ClientOptions{
			Transport: transport,
			Cloud:     cloud.AzurePublic,
			// The stub answers immediately; retrying a fixed status only adds wall time.
			Retry: policy.RetryOptions{MaxRetries: -1},
		},
		Method: azurehelper.AuthMethodAzureAD,
	}
}

func stateResponse(
	req *http.Request,
	status int,
	body string,
	errorCode string,
) *http.Response {
	header := http.Header{"Content-Type": []string{"application/json"}}
	if errorCode != "" {
		header.Set("X-Ms-Error-Code", errorCode)
	}

	return &http.Response{
		StatusCode:    status,
		Status:        http.StatusText(status),
		Header:        header,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

// TestNewStateBlobClientSeparatesTransientFromCoordinateFailures pins that only
// causes pointing at configuration or permissions carry the coordinate guidance:
// throttling, a 5xx, or a cancelled context say nothing about the config.
func TestNewStateBlobClientSeparatesTransientFromCoordinateFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		armStatus    int
		wantGuidance bool
	}{
		{name: "missing resource group", armStatus: http.StatusNotFound, wantGuidance: true},
		{name: "permission denied", armStatus: http.StatusForbidden, wantGuidance: true},
		{name: "bad request", armStatus: http.StatusBadRequest, wantGuidance: true},
		{name: "throttled", armStatus: http.StatusTooManyRequests},
		{name: "service unavailable", armStatus: http.StatusServiceUnavailable},
		{name: "internal server error", armStatus: http.StatusInternalServerError},
		{name: "request timeout", armStatus: http.StatusRequestTimeout},
		{name: "unauthorized", armStatus: http.StatusUnauthorized, wantGuidance: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := stateAzureConfig(&stateTransport{armStatus: testCase.armStatus})

			ctx := azurerm.WithStateClientCache(t.Context())
			_, err := azurerm.NewStateBlobClient(ctx, logger.CreateLogger(), cfg)
			require.Error(t, err)

			var setupErr *azurerm.StateClientSetupError
			require.ErrorAs(t, err, &setupErr)
			_, hasGuidance := errors.AsType[*azurerm.StateClientCoordinatesError](err)
			assert.Equal(t, testCase.wantGuidance, hasGuidance,
				"coordinate guidance must appear only for configuration or permission causes")
		})
	}
}

// TestNewStateBlobClientCancelledContextOmitsGuidance pins that a cancelled run
// is not reported as a misconfiguration.
func TestNewStateBlobClientCancelledContextOmitsGuidance(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	ctx = azurerm.WithStateClientCache(ctx)
	_, err := azurerm.NewStateBlobClient(ctx, logger.CreateLogger(), stateAzureConfig(&stateTransport{}))
	require.Error(t, err)

	var setupErr *azurerm.StateClientSetupError
	require.ErrorAs(t, err, &setupErr)

	var coordinateErr *azurerm.StateClientCoordinatesError
	assert.NotErrorAs(t, err, &coordinateErr,
		"a cancelled context must not be reported as a configuration problem")
}

// TestStateClientCacheReusesSharedKey pins that repeated reads against one
// storage account cost a single ARM ListKeys call, and that a different account
// or identity never reuses another's key.
func TestStateClientCacheReusesSharedKey(t *testing.T) {
	t.Parallel()

	listKeyCalls := func(t *testing.T, ctx context.Context, mutate func(*azurehelper.AzureConfig)) int {
		t.Helper()

		transport := &stateTransport{}

		for range 3 {
			cfg := stateAzureConfig(transport)
			if mutate != nil {
				mutate(cfg)
			}

			_, err := azurerm.NewStateBlobClient(ctx, logger.CreateLogger(), cfg)
			require.NoError(t, err)
		}

		calls := 0

		for _, req := range transport.Requests() {
			if strings.Contains(req.URL.Path, "listKeys") {
				calls++
			}
		}

		return calls
	}

	t.Run("cached context resolves once", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 1, listKeyCalls(t, azurerm.WithStateClientCache(t.Context()), nil))
	})

	t.Run("a different account does not reuse the key", func(t *testing.T) {
		t.Parallel()

		ctx := azurerm.WithStateClientCache(t.Context())
		accounts := 0

		assert.Equal(t, 3, listKeyCalls(t, ctx, func(cfg *azurehelper.AzureConfig) {
			accounts++
			cfg.AccountName = fmt.Sprintf("stateaccount%d", accounts)
		}))
	})

	t.Run("a different credential does not reuse the key", func(t *testing.T) {
		t.Parallel()

		ctx := azurerm.WithStateClientCache(t.Context())
		secrets := 0

		// Same principal, different secret: the second caller must prove its own
		// credential rather than inherit the first one's success.
		assert.Equal(t, 3, listKeyCalls(t, ctx, func(cfg *azurehelper.AzureConfig) {
			secrets++
			cfg.CredentialFingerprint = fmt.Sprintf("fingerprint-%d", secrets)
		}))
	})

	t.Run("a different identity does not reuse the key", func(t *testing.T) {
		t.Parallel()

		ctx := azurerm.WithStateClientCache(t.Context())
		identities := 0

		assert.Equal(t, 3, listKeyCalls(t, ctx, func(cfg *azurehelper.AzureConfig) {
			identities++
			cfg.ClientID = fmt.Sprintf("client-%d", identities)
		}))
	})

	t.Run("a different managed identity resource ID does not reuse the key", func(t *testing.T) {
		t.Parallel()

		ctx := azurerm.WithStateClientCache(t.Context())
		identities := 0

		assert.Equal(t, 3, listKeyCalls(t, ctx, func(cfg *azurehelper.AzureConfig) {
			identities++
			cfg.Method = azurehelper.AuthMethodMSI
			cfg.ClientID = ""
			cfg.MSIResourceID = fmt.Sprintf("/subscriptions/test/identities/%d", identities)
		}))
	})
}

func TestStateClientCacheDoesNotRetainLookupErrors(t *testing.T) {
	t.Parallel()

	transport := &stateTransport{armStatus: http.StatusServiceUnavailable}
	ctx := azurerm.WithStateClientCache(t.Context())

	_, err := azurerm.NewStateBlobClient(ctx, logger.CreateLogger(), stateAzureConfig(transport))
	require.Error(t, err)

	transport.armStatus = http.StatusOK
	_, err = azurerm.NewStateBlobClient(ctx, logger.CreateLogger(), stateAzureConfig(transport))
	require.NoError(t, err)
	assert.Equal(t, 2, armRequests(transport), "a later read must retry a failed lookup")
}

// TestStateClientCacheCoalescesConcurrentMissesWithRacing pins that dependencies resolving
// in parallel against one account issue a single ARM ListKeys call. The first
// caller is held inside ARM before the others start, so none of them can observe
// a populated cache and every one must cross the miss.
func TestStateClientCacheCoalescesConcurrentMissesWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, testStateClientCacheCoalescesConcurrentMisses)
}

func testStateClientCacheCoalescesConcurrentMisses(t *testing.T) {
	const followers = 7

	var (
		released    = make(chan struct{})
		reachedARM  = make(chan struct{})
		started     sync.WaitGroup
		group       sync.WaitGroup
		releaseOnce sync.Once
	)

	started.Add(followers)

	transport := &stateTransport{beforeARM: func() {
		releaseOnce.Do(func() { close(reachedARM) })
		<-released
	}}

	ctx := azurerm.WithStateClientCache(t.Context())
	results := make([]error, followers+1)

	resolve := func(slot int) {
		defer group.Done()

		_, err := azurerm.NewStateBlobClient(ctx, logger.CreateLogger(), stateAzureConfig(transport))
		results[slot] = err
	}

	// The first caller reaches ARM and blocks there, so the cache stays empty.
	group.Add(1)

	go resolve(0)

	<-reachedARM

	// Every follower therefore starts against an empty cache and must cross the
	// miss: without coalescing each one issues its own ListKeys call.
	for i := 1; i <= followers; i++ {
		group.Add(1)

		go func() {
			started.Done()
			resolve(i)
		}()
	}

	started.Wait()
	synctest.Wait()

	assert.Equal(t, 1, armRequests(transport),
		"followers must wait for the first lookup instead of issuing their own")

	close(released)
	group.Wait()

	for i, err := range results {
		require.NoErrorf(t, err, "caller %d", i)
	}

	assert.Equal(t, 1, armRequests(transport),
		"concurrent misses on one account must resolve with a single ListKeys call")
}

// armRequests counts the ARM account-key lookups the transport has seen.
func armRequests(transport *stateTransport) int {
	count := 0

	for _, req := range transport.Requests() {
		if strings.Contains(req.URL.Path, "listKeys") {
			count++
		}
	}

	return count
}

// TestNewStateBlobClientTransportFailureOmitsGuidance pins that a DNS, TLS, or
// proxy failure is not reported as a misconfiguration: no response ever arrived,
// so nothing implicates the coordinates.
func TestNewStateBlobClientTransportFailureOmitsGuidance(t *testing.T) {
	t.Parallel()

	cfg := stateAzureConfig(transportErrorTransport{})

	ctx := azurerm.WithStateClientCache(t.Context())
	_, err := azurerm.NewStateBlobClient(ctx, logger.CreateLogger(), cfg)
	require.Error(t, err)

	var setupErr *azurerm.StateClientSetupError
	require.ErrorAs(t, err, &setupErr)

	var coordinateErr *azurerm.StateClientCoordinatesError
	assert.NotErrorAs(t, err, &coordinateErr,
		"a transport failure must not be reported as a configuration problem")
}

// transportErrorTransport fails the way a proxy or DNS problem does, with no response.
type transportErrorTransport struct{}

func (transportErrorTransport) Do(req *http.Request) (*http.Response, error) {
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
}
