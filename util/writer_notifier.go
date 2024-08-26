package util

import (
	"io"
	"sync"
)

type writerNotifier struct {
	io.Writer
	notifyFn func(p []byte)
	once     sync.Once
}

// WriterNotifier fires `notifyFn` once when the first data comes at `Writer(p []byte)` and forwards data further to the specified `writer`.
func WriterNotifier(writer io.Writer, notifyFn func(p []byte)) io.Writer {
	return &writerNotifier{
		Writer:   writer,
		notifyFn: notifyFn,
	}
}

func (notifier *writerNotifier) Write(p []byte) (int, error) {
	if len(p) > 0 {
		notifier.once.Do(func() {
			notifier.notifyFn(p)
		})
	}

	return notifier.Writer.Write(p)
}
