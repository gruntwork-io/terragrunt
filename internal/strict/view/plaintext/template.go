package plaintext

const controlTemplate = `{{ .Name }}{{ "\t" }}{{ .Status.StringWithANSIColor }}{{ "\t" }}{{ if .Description }}{{ .Description }}{{ else }}{{ .Warning }}{{ end }}`

const rangeControlsTemplate = `{{ range $index, $control := .Sort }}{{ if $index }}
   {{ end }}{{ template "controlTemplate" $control }}{{ end }}`

const subcontrolTemplate = `{{ .Name }}{{ "\t" }}{{ if .Description }}{{ .Description }}{{ else }}{{ .Warning }}{{ end }}`

const rangeSubcontrolsTemplate = `{{ range $index, $control := .Sort }}{{ if $index }}
   {{ end }}{{ template "subcontrolTemplate" $control }}{{ end }}`

const listTemplate = `
   {{ template "rangeControlsTemplate" .controls }}
`

const detailControlTemplate = `
   {{ template "rangeSubcontrolsTemplate" .control.GetSubcontrols.RemoveDuplicates }}
`
