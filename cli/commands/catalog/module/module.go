package module

type Modules []*Module

type Module struct {
	path        string
	url         string
	title       string
	description string
	content     string
}

// Title implements /github.com/charmbracelet/bubbles.list.DefaultItem.Title
func (module *Module) Title() string {
	return module.title
}

// Description implements /github.com/charmbracelet/bubbles.list.DefaultItem.Description
func (module *Module) Description() string {
	return module.description
}

func (module *Module) Content() string {
	return module.content
}

// FilterValue implements /github.com/charmbracelet/bubbles.list.Item.FilterValue
func (module *Module) FilterValue() string {
	return module.title
}

func (module *Module) URL() string {
	return module.url
}

func (module *Module) Path() string {
	return module.path
}
