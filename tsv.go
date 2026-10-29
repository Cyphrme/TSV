// Package tsv reads and writes TSV with escaping.
//
// Fields are separated by tabs and records by newlines. Carriage return is
// ordinary data. A backslash escapes '\\', tab, and newline; any other
// escape is an error. The format, including the ABNF grammar, is specified
// in the repository README.
package tsv

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrInvalidEscape is returned when a field contains a trailing backslash
// or a backslash followed by an octet other than '\\', 't', or 'n'.
var ErrInvalidEscape = errors.New("tsv: invalid escape")

// ErrEmptyRecord is returned when encoding a record that has no fields.
// A blank line is not an empty record: it is one empty field, written as
// []string{""}. The text format has no encoding of a zero-field record.
var ErrEmptyRecord = errors.New("tsv: empty record")

// EscapeError is an invalid escape sequence.
// Offset is the byte index of the backslash.
// Next is the following byte, or -1 when the backslash is the last byte.
type EscapeError struct {
	Offset int
	Next   int
}

func (e *EscapeError) Error() string {
	if e.Next < 0 {
		return fmt.Sprintf("tsv: trailing backslash at offset %d", e.Offset)
	}
	if e.Next > 0x20 && e.Next < 0x7f {
		return fmt.Sprintf("tsv: invalid escape \\%c at offset %d", e.Next, e.Offset)
	}
	return fmt.Sprintf("tsv: invalid escape \\x%02x at offset %d", e.Next, e.Offset)
}

func (e *EscapeError) Unwrap() error { return ErrInvalidEscape }

// ParseError is a decoding error in one field.
// Record and Field are 1-based. Offset is the 0-based byte index of the
// bad backslash within the raw field.
type ParseError struct {
	Record int
	Field  int
	Offset int
	Err    error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("tsv: record %d, field %d: %v", e.Record, e.Field, e.Err)
}

func (e *ParseError) Unwrap() error { return e.Err }

// Escape returns the TSV encoding of one field.
// In one left-to-right pass, a backslash becomes two backslashes, a tab
// becomes a backslash followed by 't', and a newline becomes a backslash
// followed by 'n'. Inserted bytes are not scanned again. Carriage return
// is unchanged.
func Escape(field string) string {
	if !strings.ContainsAny(field, "\\\t\n") {
		return field
	}
	var b strings.Builder
	b.Grow(len(field) + 2)
	for i := 0; i < len(field); i++ {
		switch field[i] {
		case '\\':
			b.WriteString(`\\`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteByte(field[i])
		}
	}
	return b.String()
}

// Unescape reverses [Escape] with one left-to-right scan.
// A backslash followed by another backslash and then the letter t decodes
// to one backslash and the letter t, not to a tab. The same rule applies
// to n.
//
// A trailing backslash, or a backslash followed by any byte other than
// '\\', 't', or 'n', returns an error that unwraps to [ErrInvalidEscape].
// On error the result string is empty.
//
// Raw tab and newline bytes are copied unchanged. Split records and fields
// before calling Unescape; [Decode] and [Reader] do that.
func Unescape(s string) (string, error) {
	if strings.IndexByte(s, '\\') < 0 {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			return "", &EscapeError{Offset: i, Next: -1}
		}
		switch s[i+1] {
		case '\\':
			b.WriteByte('\\')
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		default:
			return "", &EscapeError{Offset: i, Next: int(s[i+1])}
		}
		i++
	}
	return b.String(), nil
}

// Encode writes records as TSV with escaping.
// Each record, including the last, is terminated by a newline.
// Encode returns [ErrEmptyRecord] if any record has no fields.
// A nil or empty records slice encodes to an empty byte slice.
func Encode(records [][]string) ([]byte, error) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteAll(records); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode parses TSV with escaping.
// A trailing newline does not create an extra record.
// A final record with no newline is returned.
// Empty input returns nil, nil.
// A blank line is a record of one empty field.
// If any record is invalid, Decode returns nil and a [ParseError].
func Decode(data []byte) ([][]string, error) {
	return NewReader(bytes.NewReader(data)).ReadAll()
}

// Reader reads records from TSV with escaping.
//
// A record is one line. Carriage return is kept as field data.
// A blank line is one empty field. The last record may omit its newline.
// Clean [io.EOF] ends that final record. Any other read error does not:
// Read returns the error and does not yield a partial record.
//
// After Read returns an error other than [io.EOF], later calls return the
// same error. Each successful Read returns a newly allocated slice.
// A Reader is not safe for concurrent use.
type Reader struct {
	r   *bufio.Reader
	n   int
	err error
}

// NewReader returns a Reader that reads from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: bufio.NewReader(r)}
}

// Read reads one record, a slice of fields.
// At clean end of input, Read returns nil, [io.EOF].
// A parse error is returned as a [ParseError] and no record.
func (r *Reader) Read() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	line, err := r.r.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
		return nil, err
	}
	if len(line) == 0 {
		return nil, io.EOF
	}
	if line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	r.n++
	rec, perr := decodeRecord(line, r.n)
	if perr != nil {
		r.err = perr
		return nil, perr
	}
	return rec, nil
}

// ReadAll reads the remaining records.
// A successful call returns err == nil, not [io.EOF].
// If any record is invalid, ReadAll returns nil and that error.
func (r *Reader) ReadAll() ([][]string, error) {
	var records [][]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			return records, nil
		}
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
}

func decodeRecord(line []byte, record int) ([]string, error) {
	parts := bytes.Split(line, []byte{'\t'})
	out := make([]string, len(parts))
	for i, p := range parts {
		s, err := Unescape(string(p))
		if err != nil {
			pe := &ParseError{Record: record, Field: i + 1, Err: err}
			var ee *EscapeError
			if errors.As(err, &ee) {
				pe.Offset = ee.Offset
			}
			return nil, pe
		}
		out[i] = s
	}
	return out, nil
}

// Writer writes records as TSV with escaping.
//
// Each record is terminated by a newline, never by CRLF.
// Writes are buffered. Call [Writer.Flush] after the last [Writer.Write]
// to push buffered data to the underlying writer, and [Writer.Error] to
// see whether Flush failed. [Writer.WriteAll] flushes.
// A Writer is not safe for concurrent use.
type Writer struct {
	w *bufio.Writer
}

// NewWriter returns a Writer that writes to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: bufio.NewWriter(w)}
}

// Write writes one record followed by a newline.
// A nil or empty record returns [ErrEmptyRecord] and writes nothing.
// Fields are escaped. The record may contain any number of fields.
func (w *Writer) Write(record []string) error {
	if len(record) == 0 {
		return ErrEmptyRecord
	}
	for i, field := range record {
		if i > 0 {
			if err := w.w.WriteByte('\t'); err != nil {
				return err
			}
		}
		if _, err := w.w.WriteString(Escape(field)); err != nil {
			return err
		}
	}
	return w.w.WriteByte('\n')
}

// Flush writes buffered data to the underlying [io.Writer].
// Call [Writer.Error] to see whether Flush failed.
func (w *Writer) Flush() {
	w.w.Flush()
}

// Error reports any error from a previous [Writer.Write] or [Writer.Flush].
func (w *Writer) Error() error {
	_, err := w.w.Write(nil)
	return err
}

// WriteAll writes every record and then flushes.
// It returns the first error from [Writer.Write] or from the flush.
func (w *Writer) WriteAll(records [][]string) error {
	for _, record := range records {
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return w.w.Flush()
}
