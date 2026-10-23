package helpers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// providerVersionsJSON approximates a registry's provider versions listing, which
// grows with the number of published versions and the platforms each one targets.
// Its size is what the decoding cost scales with.
func providerVersionsJSON(tb testing.TB, versions int) []byte {
	tb.Helper()

	type platform struct {
		OS   string `json:"os"`
		Arch string `json:"arch"`
	}

	type version struct {
		Version   string     `json:"version"`
		Protocols []string   `json:"protocols"`
		Platforms []platform `json:"platforms"`
	}

	oses := []string{"darwin", "linux", "windows"}
	arches := []string{"amd64", "arm64"}

	body := struct {
		Versions []version `json:"versions"`
	}{Versions: make([]version, 0, versions)}

	for i := range versions {
		v := version{
			Version:   "5." + strconv.Itoa(i) + ".0",
			Protocols: []string{"5.0", "6.0"},
		}

		for _, os := range oses {
			for _, arch := range arches {
				v.Platforms = append(v.Platforms, platform{OS: os, Arch: arch})
			}
		}

		body.Versions = append(body.Versions, v)
	}

	data, err := json.Marshal(body)
	require.NoError(tb, err)

	return data
}

// BenchmarkClientDo measures a cache miss, where the response is read off the
// wire and decoded. A fresh client per iteration keeps every iteration a miss
// without letting the per-URL cache grow for the length of the run.
func BenchmarkClientDo(b *testing.B) {
	for _, versions := range []int{10, 200, 2000} {
		b.Run(strconv.Itoa(versions)+"versions", func(b *testing.B) {
			body := providerVersionsJSON(b, versions)

			c := vhttp.NewMemClient(
				func(_ context.Context, _ *http.Request) (*http.Response, error) {
					return vhttp.Respond(http.StatusOK, body, nil), nil
				},
			)

			ctx := b.Context()

			b.ReportAllocs()
			b.SetBytes(int64(len(body)))

			for b.Loop() {
				var value map[string]any

				client := helpers.NewClient(c, nil)
				require.NoError(b, client.Do(ctx, http.MethodGet, "https://registry.test/v1/providers/hashicorp/aws/versions", &value))
			}
		})
	}
}

// BenchmarkClientDoConcurrent measures a cache miss that many callers hit at
// once, the shape a `run --all` gives the cache server when its units start
// together and ask for the same provider. The registry takes a millisecond to
// answer, so the callers overlap. requests/op reports how many registry
// requests each round of callers cost.
func BenchmarkClientDoConcurrent(b *testing.B) {
	const callers = 64

	body := providerVersionsJSON(b, 200)

	var requests atomic.Int64

	c := vhttp.NewMemClient(
		func(_ context.Context, _ *http.Request) (*http.Response, error) {
			requests.Add(1)
			time.Sleep(time.Millisecond)

			return vhttp.Respond(http.StatusOK, body, nil), nil
		},
	)

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		client := helpers.NewClient(c, nil)

		var wg sync.WaitGroup

		for range callers {
			wg.Go(func() {
				var value map[string]any

				assert.NoError(b, client.Do(ctx, http.MethodGet, "https://registry.test/v1/providers/hashicorp/aws/versions", &value))
			})
		}

		wg.Wait()
	}

	b.ReportMetric(float64(requests.Load())/float64(b.N), "requests/op")
}
