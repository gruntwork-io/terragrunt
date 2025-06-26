# Custom local template
terraform {
  source = "{{ .sourceUrl }}"
}

include "root" {
  path = find_in_parent_folders("root.hcl")
}

inputs = {
  # This is a custom template from a local directory
  template_type = "local"

  # Required variables would be listed here if any exist
  {{- if .requiredVariables }}
  {{- range .requiredVariables }}
  # {{ if .Description }}{{ .Description }}{{ else }}Variable: {{ .Name }}{{ end }}
  # Type: {{ .Type }}
  {{ .Name }} = {{ .DefaultValuePlaceholder }}  # TODO: fill in value
  {{- end }}
  {{- else }}
  # No required variables found for this module
  {{- end }}
}
