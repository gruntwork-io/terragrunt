package view

import (
	"fmt"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
)

// Writer is the base layer for command views, encapsulating a set of I/O streams, a colorize implementation, and implementing a human friendly view for diagnostics.
type Writer struct {
	io.Writer
	render Render
}

func NewWriter(writer io.Writer, render Render) *Writer {
	return &Writer{
		Writer: writer,
		render: render,
	}
}

func (writer *Writer) List(controls strict.Controls) error {
	output, err := writer.render.List(controls)
	if err != nil {
		return err
	}

	return writer.output(output)
}

func (writer *Writer) DetailControl(control strict.Control) error {
	output, err := writer.render.DetailControl(control)
	if err != nil {
		return err
	}

	return writer.output(output)
}

func (writer *Writer) output(output string) error {
	if _, err := fmt.Fprint(writer, output); err != nil {
		return errors.New(err)
	}

	return nil
}
