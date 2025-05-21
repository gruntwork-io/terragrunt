package getproviders_test

import (
	"context"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// manualMockProvider is a manually implemented mock for the Provider interface.
type manualMockProvider struct {
	addr             string
	ver              string
	pkgDir           string
	docSHA256Sums    []byte
	docSHA256SumsErr error
	logger           log.Logger
}

func (m *manualMockProvider) Address() string {
	return m.addr
}

func (m *manualMockProvider) Version() string {
	return m.ver
}

func (m *manualMockProvider) DocumentSHA256Sums(ctx context.Context) ([]byte, error) {
	return m.docSHA256Sums, m.docSHA256SumsErr
}

func (m *manualMockProvider) PackageDir() string {
	return m.pkgDir
}

func (m *manualMockProvider) Logger() log.Logger {
	return m.logger
}
