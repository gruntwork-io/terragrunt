package plaintext

import (
	"bytes"
	"text/tabwriter"
	"text/template"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/view"
)

var _ = view.Render(new(Render))

type Render struct{}

func NewRender() *Render {
	return &Render{}
}

// List implements view.Render interface.
func (render *Render) List(controls strict.Controls) (string, error) {
	return render.executeTemplate(listTemplate, controls, nil)
}

// DetailControl implements view.Render interface.
func (render *Render) DetailControl(control strict.Control) (string, error) {
	return render.executeTemplate(detailControlTemplate, map[string]any{"control": control}, nil)
}

func (render *Render) executeTemplate(templ string, data any, customFuncs map[string]any) (string, error) {
	funcMap := template.FuncMap{}

	for key, value := range customFuncs {
		funcMap[key] = value
	}

	t := template.Must(template.New("template").Funcs(funcMap).Parse(templ))
	templates := map[string]string{
		"subcontrolTemplate":       subcontrolTemplate,
		"controlTemplate":          controlTemplate,
		"rangeSubcontrolsTemplate": rangeSubcontrolsTemplate,
		"rangeControlsTemplate":    rangeControlsTemplate,
	}

	for name, value := range templates {
		if _, err := t.New(name).Parse(value); err != nil {
			return "", errors.New(err)
		}
	}

	out := new(bytes.Buffer)

	var minwidth, tabwidth, padding = 1, 8, 2

	tabOut := tabwriter.NewWriter(out, minwidth, tabwidth, padding, ' ', 0)

	if err := t.Execute(tabOut, data); err != nil {
		return "", errors.New(err)
	}

	if err := tabOut.Flush(); err != nil {
		return "", errors.New(err)
	}

	return out.String(), nil
}
