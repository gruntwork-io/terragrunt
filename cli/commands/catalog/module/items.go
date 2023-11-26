package module

import (
	"github.com/pkg/browser"
)

type Items []*Item

type Item struct {
	dir         string
	url         string
	title       string
	description string
	readme      string
}

func (item *Item) Title() string {
	return item.title
}

func (item *Item) Description() string {
	return item.description
}

func (item *Item) Readme() string {
	return item.readme
}

func (item *Item) FilterValue() string {
	return item.title
}

func (item *Item) ViewInBrowser() error {
	return browser.OpenURL(item.url)
}

func (item *Item) ScaffoldCommand() *ScaffoldCommand {
	return NewScaffoldCommand(item.dir)
}
