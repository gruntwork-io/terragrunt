package log

// Writer redirects Write requests to configured logger and level
type Writer struct {
	Logger Logger
	Level  Level
}

func (w *Writer) Write(p []byte) (n int, err error) {
	w.Logger.Log(w.Level, string(p))
	return len(p), nil
}
