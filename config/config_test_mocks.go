package config

import (
	"os"
	"time"
)

type MockFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	sys     interface{}
}

func (mock MockFileInfo) Name() string {
	return mock.name
}

func (mock MockFileInfo) Size() int64 {
	return mock.size
}

func (mock MockFileInfo) Mode() os.FileMode {
	return mock.mode
}

func (mock MockFileInfo) ModTime() time.Time {
	return mock.modTime
}

func (mock MockFileInfo) IsDir() bool {
	return mock.isDir
}

func (mock MockFileInfo) Sys() interface{} {
	return mock.sys
}
