// common integration test functions
package integration_test

import (
	"html/template"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

var testCLIConfigTemplate = `
{{ if or (gt (len .FilesystemMirrorMethods) 0) (gt (len .NetworkMirrorMethods) 0) (gt (len .DirectMethods) 0) }}
provider_installation {
{{ if gt (len .FilesystemMirrorMethods) 0 }}{{ range $method := .FilesystemMirrorMethods }}
  filesystem_mirror {
    path    = "{{ $method.Path }}"
{{ if gt (len $method.Include) 0 }}
    include = [{{ range $index, $include := $method.Include }}{{ if $index }},{{ end }}"{{ $include }}"{{ end }}]
{{ end }}{{ if gt (len $method.Exclude) 0 }}
    exclude = [{{ range $index, $exclude := $method.Exclude }}{{ if $index }},{{ end }}"{{ $exclude }}"{{ end }}]
{{ end }}
  }
{{ end }}{{ end }}
{{ if gt (len .NetworkMirrorMethods) 0 }}{{ range $method := .NetworkMirrorMethods }}
  network_mirror {
    url    = "{{ $method.URL }}"
{{ if gt (len $method.Include) 0 }}
    include = [{{ range $index, $include := $method.Include }}{{ if $index }},{{ end }}"{{ $include }}"{{ end }}]
{{ end }}{{ if gt (len $method.Exclude) 0 }}
    exclude = [{{ range $index, $exclude := $method.Exclude }}{{ if $index }},{{ end }}"{{ $exclude }}"{{ end }}]
{{ end }}
  }
{{ end }}{{ end }}
{{ if gt (len .DirectMethods) 0 }}{{ range $method := .DirectMethods }}
  direct {
{{ if gt (len $method.Include) 0 }}
    include = [{{ range $index, $include := $method.Include }}{{ if $index }},{{ end }}"{{ $include }}"{{ end }}]
{{ end }}{{ if gt (len $method.Exclude) 0 }}
    exclude = [{{ range $index, $exclude := $method.Exclude }}{{ if $index }},{{ end }}"{{ $exclude }}"{{ end }}]
{{ end }}
  }
{{ end }}{{ end }}
}
{{ end }}
`

type CLIConfigProviderInstallationFilesystemMirror struct {
	Path             string
	Include, Exclude []string
}

type CLIConfigProviderInstallationNetworkMirror struct {
	URL              string
	Include, Exclude []string
}

type CLIConfigProviderInstallationDirect struct {
	Include, Exclude []string
}

type CLIConfigSettings struct {
	FilesystemMirrorMethods []CLIConfigProviderInstallationFilesystemMirror
	NetworkMirrorMethods    []CLIConfigProviderInstallationNetworkMirror
	DirectMethods           []CLIConfigProviderInstallationDirect
}

func createCLIConfig(t *testing.T, file *os.File, settings *CLIConfigSettings) {
	tmp, err := template.New("cliconfig").Parse(testCLIConfigTemplate)
	require.NoError(t, err)

	err = tmp.Execute(file, settings)
	require.NoError(t, err)
}
