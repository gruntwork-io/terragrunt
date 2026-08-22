package azurerm_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
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

			client, err := azurerm.NewStateBlobClient(t.Context(), logger.CreateLogger(), cfg)
			if testCase.wantErr {
				require.ErrorIs(t, err, azurerm.ErrStateClientSetup)
				assert.Contains(t, err.Error(), "AuthorizationFailed")
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
	requests   []*http.Request
	armStatus  int
	blobStatus int
}

func (transport *stateTransport) Do(req *http.Request) (*http.Response, error) {
	transport.requests = append(transport.requests, req.Clone(req.Context()))

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
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := stateAzureConfig(&stateTransport{armStatus: testCase.armStatus})

			_, err := azurerm.NewStateBlobClient(t.Context(), logger.CreateLogger(), cfg)
			require.Error(t, err)

			// The phase marker is always present so callers can tell setup from an absent blob.
			require.ErrorIs(t, err, azurerm.ErrStateClientSetup)
			assert.Equal(t, testCase.wantGuidance, errors.Is(err, azurerm.ErrStateClientCoordinates),
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

	_, err := azurerm.NewStateBlobClient(ctx, logger.CreateLogger(), stateAzureConfig(&stateTransport{}))
	require.Error(t, err)
	require.ErrorIs(t, err, azurerm.ErrStateClientSetup)
	assert.NotErrorIs(t, err, azurerm.ErrStateClientCoordinates,
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

	t.Run("uncached context resolves every time", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 3, listKeyCalls(t, t.Context(), nil))
	})

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

	t.Run("a different identity does not reuse the key", func(t *testing.T) {
		t.Parallel()

		ctx := azurerm.WithStateClientCache(t.Context())
		identities := 0

		assert.Equal(t, 3, listKeyCalls(t, ctx, func(cfg *azurehelper.AzureConfig) {
			identities++
			cfg.ClientID = fmt.Sprintf("client-%d", identities)
		}))
	})
}
