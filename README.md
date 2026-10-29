# TSV
This document specifies **TSV**, a line-oriented format for records of fields
with escaping. 

Tab is the column delimiter, newline is the record delimiter and backslash is
the escaper.  (Carriage return is not special and is treated as ordinary data.)

To avoid escaping, your data may simply preclude tab, newline, and backslash as
characters.

**Example**

```tsv
Name	Age	City	Email
Alice	30	New York	alice@example.com
Bob	25	London	bob@example.com
Charlie	35	Paris	charlie@example.com
```

## TSV definition

- **Column delimiter:** tab (`\t`)
- **Record delimiter:** newline (`\n`)  
- **Escape character:** backslash (`\`)

### Escaping 
**Escaping algorithm**
Apply the following replacements in order:

1. Replace `\` with `\\`
2. Replace tab with `\t`
3. Replace newline with `\n`

**Unescaping algorithm**
Apply the following replacements in order:

1. Replace `\\` with `\`
2. Replace `\t` with tab
3. Replace `\n` with newline

## Escape Example
**Source input**
```
Name,Note,City
Alice,"Says ""hi""",New York
Bob,Has a\backslash,London
Charlie,Line1
Line2,Paris
```

**Escaped TSV**
```tsv
Name	Note	City
Alice	Says "hi"	New York
Bob	Has a\\backslash	London
Charlie	Line1\nLine2	Paris
```

## Go
The Go package in this repository is one implementation.
```
go get github.com/Cyphrme/TSV
```

The module path is `github.com/Cyphrme/TSV`, the same capitalization as the
repository (Go module paths are case-sensitive; GitHub's HTTP URL is not). The
package name is `tsv`.

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

`Escape` and `Unescape` convert one field. `Reader` and `Writer` stream records,
in the style of `encoding/csv`. `Writer` buffers and ends every record with
`LF`; call `Flush` or `WriteAll`.

`Encode` rejects a record with no fields (`ErrEmptyRecord`). A blank line is
`[]string{""}`. `Unescape` and `Decode` reject a bad backslash
(`ErrInvalidEscape`). `Decode` and `ReadAll` return no records if any record is
invalid.

## Appendix
### ABNF Grammar

Normative octet grammar ([RFC 5234](https://www.rfc-editor.org/rfc/rfc5234)).
`HTAB` and `LF` are core rules. Escape letters are lowercase only (`%x74`,
`%x6E`).

```abnf
file                = *terminated-record [unterminated-record]
terminated-record   = fields LF
unterminated-record = nonempty-fields
fields              = field *(*HTAB field)
nonempty-fields     = 1*field-unit *(*HTAB field)
                    / HTAB fields
field               = *field-unit
field-unit          = unescaped / escape
unescaped           = %x00-08 / %x0B-5B / %x5D-FF
escape              = %x5C.5C / %x5C.74 / %x5C.6E
```

`unescaped` is any octet except HTAB, LF, and `\`. The two `nonempty-fields`
alternatives do not overlap.

- Empty file: zero records.
- `LF` alone is one empty field. A trailing `LF` does not add a record. `LF LF`
  is two records.
- `unterminated-record` requires at least one octet and has no trailing `LF`.
- A trailing `\` or unrecognized escape (e.g. `\q`) is not a file.

**Decode:** split each record on `HTAB`, then unescape each field.  
**Encode:** escape each field, join with `HTAB`, terminate every record with
`LF`.

### Prior art

Tab-separated values:

- https://www.loc.gov/preservation/digital/formats/fdd/fdd000533.shtml
- https://www.iana.org/assignments/media-types/text/tab-separated-values
