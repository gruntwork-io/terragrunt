package helpers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
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
	if err != nil {
		tb.Fatal(err)
	}

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
				if err := client.Do(ctx, http.MethodGet, "https://registry.test/v1/providers/hashicorp/aws/versions", &value); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
