package errors

type InvalidArgError struct {
	s string
}

func (e InvalidArgError) Error() string {
	return e.s
}

func NewInvalidArgError(message string) error {
	return InvalidArgError{s: message}
}
