package plaintext

const controlTemplate = `{{ .Name }}{{ "\t" }}{{ .Status.StringWithANSIColor }}{{ "\t" }}{{ if .Description }}{{ .Description }}{{ else }}{{ .Warning }}{{ end }}`

const rangeControlsTemplate = `{{ range $index, $control := .Sort }}{{ if $index }}
   {{ end }}{{ template "controlTemplate" $control }}{{ end }}`

const subcontrolTemplate = `{{ .Name }}{{ "\t" }}{{ if .Description }}{{ .Description }}{{ else }}{{ .Warning }}{{ end }}`

const rangeSubcontrolsTemplate = `{{ range $index, $control := .Sort }}{{ if $index }}
   {{ end }}{{ template "subcontrolTemplate" $control }}{{ end }}`

const listTemplate = `
   {{ $controls := .controls }}{{ $categories := $controls.GetCategories.FilterNotHidden.Sort }}{{ range $index, $category := $categories }}{{ if $index }}
   {{ end }}{{ $category.Name }}:
   {{ $categoryControls := $controls.FilterByCategories $category }}{{ template "rangeControlsTemplate" $categoryControls }}
   {{ end }}{{ $noCategoryControls := $controls.FilterByCategories }}{{ if $noCategoryControls }}
   {{ template "rangeControlsTemplate" $noCategoryControls }}
   {{ end }}
`
const detailControlTemplate = `
   {{ $controls := .control.GetSubcontrols.RemoveDuplicates }}{{ $categories := $controls.GetCategories.FilterNotHidden.Sort }}{{ range $index, $category := $categories }}{{ if $index }}
   {{ end }}{{ $category.Name }}:
   {{ $categoryControls := $controls.FilterByCategories $category }}{{ template "rangeSubcontrolsTemplate" $categoryControls }}
   {{ end }}{{ $noCategoryControls := $controls.FilterByCategories }}{{ if and $categories $noCategoryControls }}
   {{ end }}{{ if $noCategoryControls }}{{ template "rangeSubcontrolsTemplate" $noCategoryControls }}
   {{ end }}
`
