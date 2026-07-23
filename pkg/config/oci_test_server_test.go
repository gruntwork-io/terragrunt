package config

// Minimal OCI Distribution Spec v2 server for config-package tests.
// Mirrors internal/getter/oci_test_server_test.go but lives here to avoid
// exposing test helpers across package boundaries.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

type configOCIManifest struct {
	SchemaVersion int                  `json:"schemaVersion"`
	MediaType     string               `json:"mediaType"`
	Config        configOCIDescriptor  `json:"config"`
	Layers        []configOCIDescriptor `json:"layers"`
}

type configOCIDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type configOCITestServer struct {
	manifestDigest string
	manifestBytes  []byte
	layerDigest    string
	layerBytes     []byte
}

var configManifestRE = regexp.MustCompile(`^/v2/(.+)/manifests/(.+)$`)
var configBlobRE = regexp.MustCompile(`^/v2/(.+)/blobs/(.+)$`)

func newConfigOCITestServer(t *testing.T, files map[string]string) (*httptest.Server, *configOCITestServer) {
	t.Helper()

	state := &configOCITestServer{}
	state.layerBytes = configMakeTarGz(t, files)
	state.layerDigest = "sha256:" + configHexSHA256(state.layerBytes)

	configBytes := []byte("{}")
	configDigest := "sha256:" + configHexSHA256(configBytes)

	m := configOCIManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: configOCIDescriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    configDigest,
			Size:      int64(len(configBytes)),
		},
		Layers: []configOCIDescriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    state.layerDigest,
				Size:      int64(len(state.layerBytes)),
			},
		},
	}

	var err error
	state.manifestBytes, err = json.Marshal(m)
	require.NoError(t, err)
	state.manifestDigest = "sha256:" + configHexSHA256(state.manifestBytes)

	srv := httptest.NewServer(state)
	t.Cleanup(srv.Close)

	return srv, state
}

func (s *configOCITestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v2/" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "{}")

		return
	}

	if configManifestRE.MatchString(r.URL.Path) {
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Docker-Content-Digest", s.manifestDigest)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.manifestBytes)))
		w.WriteHeader(http.StatusOK)

		if r.Method != http.MethodHead {
			_, _ = w.Write(s.manifestBytes)
		}

		return
	}

	if m := configBlobRE.FindStringSubmatch(r.URL.Path); m != nil {
		if m[2] == s.layerDigest {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.layerBytes)))
			w.WriteHeader(http.StatusOK)

			if r.Method != http.MethodHead {
				_, _ = w.Write(s.layerBytes)
			}

			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func configMakeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, content := range files {
		body := []byte(content)
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write(body)
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	return buf.Bytes()
}

func configHexSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
