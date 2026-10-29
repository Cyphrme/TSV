# TSV with escaping

This document specifies **TSV with escaping**, a line-oriented format for records of fields. Tab and newline are the delimiters. A backslash escape lets a field contain tab, newline, and backslash. The Go package in this repository is one implementation.

## Format

- **Field delimiter:** tab, octet `%x09`.
- **Record delimiter:** newline, octet `%x0A` (`LF`). A newline ends a record. It does not start another one.
- **Carriage return (`%x0D`) is ordinary data.** It is not a delimiter, it is not removed before a newline, and it is not an escape.
- **Escape character:** backslash, octet `%x5C`.
- No other octet is special. Quotation marks, `#`, spaces, NULs, and non-UTF-8 bytes are literal. The format has no comment syntax, quoting, required header, or BOM. A leading `U+FEFF` is data.

Writers end every record with `LF`, including the last. Readers also accept a last record that has no `LF`, but only when that record contains at least one octet. The empty input is zero records, not an empty record.

## Escaping

Escape one field at a time, from the original octets, in this order. Do not rescan bytes inserted by an earlier substitution.

1. `%x5C` (`\`) becomes `%x5C.5C` (`\\`).
2. `%x09` (tab) becomes `%x5C.74` (`\t`).
3. `%x0A` (newline) becomes `%x5C.6E` (`\n`).

A single left-to-right pass that emits those encodings is the same rule. Every other octet, including carriage return, is copied unchanged.

## Unescaping

Split the file into records and fields first (raw `%x0A`, then raw `%x09`). Then unescape each field with one left-to-right scan. Do not apply sequential string replacements, and do not unescape before splitting: `\t` would become a real tab and look like a delimiter.

- `%x5C.5C` (`\\`) becomes `%x5C`
- `%x5C.74` (`\t`) becomes `%x09`
- `%x5C.6E` (`\n`) becomes `%x0A`

`\\t` is a backslash followed by the letter `t`, not a tab. `\\n` is a backslash followed by the letter `n`.

A backslash at the end of a field, or a backslash followed by any octet other than `%x5C`, `%x74`, or `%x6E`, is invalid. That includes `\r`, `\T`, `\N`, `\u`, `\x`, and a backslash before a raw tab. Decoders reject the input. They do not pass the backslash through, drop it, or guess.

## Records

- A record is one or more fields joined by tabs. The format has no zero-field record.
- A field may be empty. `a HTAB HTAB b` is `a`, empty, `b`. A tab at either end of a line is an empty field. A single tab is two empty fields.
- A line with no tab is one field. A blank line (`LF` only) is a record of one empty field.
- Records in one file may have different numbers of fields. The format does not pad columns.
- `LF LF` is two records, the second with one empty field. `a LF LF` is `a`, then an empty field. `a LF` and `a` are the same single record. A file of no octets has no records.

## Grammar

The following ABNF ([RFC 5234](https://www.rfc-editor.org/rfc/rfc5234)) is normative for the encoded file. It is an octet grammar. `HTAB` (`%x09`) and `LF` (`%x0A`) are RFC 5234 core rules. ABNF quoted strings are case-insensitive, so the escape letters are written as octets: `t` is `%x74` and `n` is `%x6E`, not `T` or `N`.

```abnf
file = *terminated-record [unterminated-record]

terminated-record   = fields LF
unterminated-record = nonempty-fields

fields          = field *(*HTAB field)
nonempty-fields = 1*field-unit *(*HTAB field)
                / HTAB fields

field           = *field-unit
field-unit      = unescaped / escape

; Any octet except HTAB, LF, and backslash.
; Carriage return (%x0D) is in this set.
unescaped       = %x00-08 / %x0B-5B / %x5D-FF

; "\\" , "\t" , "\n" — lowercase t and n only.
escape          = %x5C.5C / %x5C.74 / %x5C.6E
```

The two alternatives of `nonempty-fields` do not overlap: the first starts with a field unit, the second with `HTAB`. A `field-unit` is one unescaped octet or one two-octet escape.

Consequences, restated from the productions:

- A file with no octets matches `file` with nothing after it: zero records.
- `terminated-record` always has at least one `field`. That field may match `*field-unit` zero times, so `LF` alone is one empty field, never zero fields.
- Each `LF` ends the record in front of it. A final `LF` does not add a further record. `LF LF` is two `terminated-record`s.
- `unterminated-record` is a final record with no `LF` and at least one octet (`a`, or `HTAB`, or `a HTAB b`). That is why `a` and `a LF` are the same records, and why a blank line must include its `LF`.
- An octet sequence that does not match, including a trailing backslash or `\q`, is not a file. Decoders return an error.

Decoding a match: take each record's fields (split on `HTAB` only), then apply the unescape scan to each field. Encoding: escape each field, join a record's fields with `HTAB`, and end every record with `LF`.

## Example

Three records: `id`, `text`; then `1` and a field whose value is `a`, a tab, and `b`; then `2` and a field whose value is `two`, one backslash, `lines?`, a newline, and `no`.

The encoded file, written as a Go string:

```
"id\ttext\n1\ta\\tb\n2\ttwo\\\\lines?\\nno\n"
```

## Go

```
go get github.com/Cyphrme/TSV
```

The module path is `github.com/Cyphrme/TSV`, the same capitalization as the repository (Go module paths are case-sensitive; GitHub's HTTP URL is not). The package name is `tsv`.

```go
import "github.com/Cyphrme/TSV"

records := [][]string{
    {"id", "text"},
    {"1", "a\tb"},
    {"2", "two\\lines?\nno"},
}
b, err := tsv.Encode(records)
got, err := tsv.Decode(b)
```

`Escape` and `Unescape` convert one field. `Reader` and `Writer` stream records, in the style of `encoding/csv`. `Writer` buffers and ends every record with `LF`; call `Flush` or `WriteAll`.

`Encode` rejects a record with no fields (`ErrEmptyRecord`). A blank line is `[]string{""}`. `Unescape` and `Decode` reject a bad backslash (`ErrInvalidEscape`). `Decode` and `ReadAll` return no records if any record is invalid.

## Prior art

Tab-separated values without this escaping:

- https://www.loc.gov/preservation/digital/formats/fdd/fdd000533.shtml
- https://www.iana.org/assignments/media-types/text/tab-separated-values
