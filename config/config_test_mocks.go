package config

import (
	"os"
	"time"

	"github.com/hashicorp/hcl/v2"
)

type MockOsFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	sys     interface{}
}

func (mock MockOsFileInfo) Name() string {
	return mock.name
}

func (mock MockOsFileInfo) Size() int64 {
	return mock.size
}

func (mock MockOsFileInfo) Mode() os.FileMode {
	return mock.mode
}

func (mock MockOsFileInfo) ModTime() time.Time {
	return mock.modTime
}

func (mock MockOsFileInfo) IsDir() bool {
	return mock.isDir
}

func (mock MockOsFileInfo) Sys() interface{} {
	return mock.sys
}

type MockHclBody struct {
	bodyContent *hcl.BodyContent
	diagnostics hcl.Diagnostics
	attributes hcl.Attributes
	itemRange hcl.Range
}

func (mock MockHclBody) Content(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Diagnostics) {
	return mock.bodyContent, mock.diagnostics
}

func (mock MockHclBody) PartialContent(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	inlineBody := MockHclBody{}

	return mock.bodyContent, inlineBody, mock.diagnostics
}

func (mock MockHclBody) JustAttributes() (hcl.Attributes, hcl.Diagnostics) {
	return mock.attributes, mock.diagnostics
}

func (mock MockHclBody) MissingItemRange() hcl.Range {
	return mock.itemRange
}
