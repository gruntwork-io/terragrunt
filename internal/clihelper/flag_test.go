package clihelper_test

func mockDestValue[T any](val T) *T {
	return &val
}
