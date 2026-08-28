package view

import (
	"fmt"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/strict"
)

// Writer is the base layer for command views, encapsulating a set of I/O streams and implementing a human friendly view for strict controls.
type Writer struct {
	io.Writer
	render Render
}

// NewWriter returns a new Writer instance that uses the provided io.Writer for output
// and the Render interface for formatting controls.
func NewWriter(writer io.Writer, render Render) *Writer {
	return &Writer{
		Writer: writer,
		render: render,
	}
}

// List renders the given list of controls.
func (writer *Writer) List(controls strict.Controls) error {
	output, err := writer.render.List(controls)
	if err != nil {
		return err
	}

	return writer.output(output)
}

// DetailSubcontrols renders the given subcontrols of a single control.
func (writer *Writer) DetailSubcontrols(subcontrols strict.Controls) error {
	output, err := writer.render.DetailSubcontrols(subcontrols)
	if err != nil {
		return err
	}

	return writer.output(output)
}

func (writer *Writer) output(output string) error {
	if _, err := fmt.Fprint(writer, output); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}
