package view

import (
	"fmt"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/util"
)

type Render interface {
	// Diagnostics renders early diagnostics, resulting from argument parsing.
	Diagnostics(diags diagnostic.Diagnostics) (string, error)

	// ShowConfigPath renders paths to configurations that contain errors.
	ShowConfigPath(filenames []string) (string, error)
}

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

func (writer *Writer) Diagnostics(diags diagnostic.Diagnostics) error {
	output, err := writer.render.Diagnostics(diags)
	if err != nil {
		return err
	}

	return writer.output(output)
}

func (writer *Writer) ShowConfigPath(diags diagnostic.Diagnostics) error {
	var filenames []string

	for _, diag := range diags {
		if diag.Range != nil && diag.Range.Filename != "" && !util.ListContainsElement(filenames, diag.Range.Filename) {
			filenames = append(filenames, diag.Range.Filename)
		}
	}

	output, err := writer.render.ShowConfigPath(filenames)
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
