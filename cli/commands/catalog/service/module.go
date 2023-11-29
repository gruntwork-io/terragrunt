package service

type Modules []*Module

type Module struct {
	path        string
	url         string
	title       string
	description string
	content     string
}

func (module *Module) Title() string {
	return module.title
}

func (module *Module) Description() string {
	return module.description
}

func (module *Module) Content() string {
	return module.content
}

func (module *Module) FilterValue() string {
	return module.title
}

func (module *Module) URL() string {
	return module.url
}

func (module *Module) Path() string {
	return module.path
}
