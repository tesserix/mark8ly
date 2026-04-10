package csvjob

import (
	"encoding/csv"
	"io"
)

// ErrorCSVWriter writes error rows to a CSV destination. The destination
// is typically a GCS object writer, but any io.WriteCloser works (which
// makes unit testing possible with a bytes.Buffer wrapper).
type ErrorCSVWriter interface {
	// WriteHeader writes the header row (called once at the start).
	WriteHeader(headers []string) error
	// WriteRow writes a data row plus the error message.
	WriteRow(row []string, errMsg string) error
	// Close flushes and closes the underlying writer.
	Close() error
}

// NewErrorCSVWriter constructs an ErrorCSVWriter that writes to w.
func NewErrorCSVWriter(w io.WriteCloser) ErrorCSVWriter {
	return &errorCSVWriter{
		w:   w,
		csv: csv.NewWriter(w),
	}
}

type errorCSVWriter struct {
	w   io.WriteCloser
	csv *csv.Writer
}

func (e *errorCSVWriter) WriteHeader(headers []string) error {
	return e.csv.Write(append(headers, "error"))
}

func (e *errorCSVWriter) WriteRow(row []string, errMsg string) error {
	return e.csv.Write(append(row, errMsg))
}

func (e *errorCSVWriter) Close() error {
	e.csv.Flush()
	if err := e.csv.Error(); err != nil {
		return err
	}
	return e.w.Close()
}
