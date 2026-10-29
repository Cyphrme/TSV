package tsv

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"strconv"
	"strings"
	"testing"
)

func TestEscape(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"a", "a"},
		{" ", " "},
		{`\`, `\\`},
		{"\t", `\t`},
		{"\n", `\n`},
		{"\r", "\r"},
		{"a\tb\nc\\", `a\tb\nc\\`},
		{"\t\n\\", `\t\n\\`},
		{"héllo", "héllo"},
		{"世界", "世界"},
		{"\xff", "\xff"},
		{"\x00", "\x00"},
		{`quote " #`, `quote " #`},
	}
	for _, tt := range tests {
		got := Escape(tt.in)
		if got != tt.want {
			t.Errorf("Escape(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if strings.ContainsAny(got, "\t\n") {
			t.Errorf("Escape(%q) = %q contains a raw delimiter", tt.in, got)
		}
	}

	// Substitution order: backslash, then tab, then newline, without rescan.
	in := "\\\t\n\\"
	want := `\\` + `\t` + `\n` + `\\`
	if got := Escape(in); got != want {
		t.Errorf("Escape order = %q, want %q", got, want)
	}
}

func TestUnescape(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
		next    int
		offset  int
	}{
		{name: "empty", in: "", want: ""},
		{name: "plain", in: "abc", want: "abc"},
		{name: "tab", in: `\t`, want: "\t"},
		{name: "newline", in: `\n`, want: "\n"},
		{name: "backslash", in: `\\`, want: `\`},
		{name: "backslash then t", in: `\\t`, want: `\t`},
		{name: "backslash then n", in: `\\n`, want: `\n`},
		{name: "four slashes", in: `\\\\`, want: `\\`},
		{name: "slash then escaped tab", in: `\\\t`, want: "\\\t"},
		{name: "cr copied", in: "a\rb", want: "a\rb"},
		{name: "raw tab copied", in: "a\tb", want: "a\tb"},
		{name: "unicode", in: "héllo\\n世界", want: "héllo\n世界"},
		{name: "nul", in: "a\x00b", want: "a\x00b"},
		{name: "invalid utf-8", in: "\xff", want: "\xff"},
		{name: "trailing", in: `\`, wantErr: true, next: -1, offset: 0},
		{name: "trailing later", in: `ab\`, wantErr: true, next: -1, offset: 2},
		{name: "bad q", in: `\q`, wantErr: true, next: int('q'), offset: 0},
		{name: "bad q later", in: `ab\qcd`, wantErr: true, next: int('q'), offset: 2},
		{name: "uppercase T", in: `\T`, wantErr: true, next: int('T')},
		{name: "uppercase N", in: `\N`, wantErr: true, next: int('N')},
		{name: "backslash r", in: `\r`, wantErr: true, next: int('r')},
		{name: "hex style", in: `\x6e`, wantErr: true, next: int('x')},
		{name: "unicode style", in: `\u0009`, wantErr: true, next: int('u')},
		{name: "backslash then raw tab", in: "\\\t", wantErr: true, next: int('\t')},
		{name: "backslash then nul", in: "\\\x00", wantErr: true, next: 0},
		{name: "backslash then high", in: "\\\xff", wantErr: true, next: 0xff},
		{name: "odd slashes", in: `\\\`, wantErr: true, next: -1, offset: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Unescape(tt.in)
			if tt.wantErr {
				if got != "" {
					t.Fatalf("result %q with error %v", got, err)
				}
				var ee *EscapeError
				if !errors.As(err, &ee) {
					t.Fatalf("error %v, want *EscapeError", err)
				}
				if !errors.Is(err, ErrInvalidEscape) {
					t.Fatalf("errors.Is ErrInvalidEscape: %v", err)
				}
				if ee.Next != tt.next || ee.Offset != tt.offset {
					t.Fatalf("Next=%d Offset=%d, want Next=%d Offset=%d", ee.Next, ee.Offset, tt.next, tt.offset)
				}
				msg := err.Error()
				if tt.next < 0 && !strings.Contains(msg, "trailing backslash") {
					t.Fatalf("message %q", msg)
				}
				if tt.next >= 0 && !strings.Contains(msg, "invalid escape") {
					t.Fatalf("message %q", msg)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEscapeRoundTrip(t *testing.T) {
	fields := []string{
		"", "a", `\`, "\t", "\n", "\r", "a\tb\nc\\d\r",
		"\\\\t", "\\t", "\t\n\\", "héllo\n世界", "\xff\x00",
		" #\"", strings.Repeat("x", 100),
	}
	for _, f := range fields {
		got, err := Unescape(Escape(f))
		if err != nil {
			t.Fatalf("Unescape(Escape(%q)): %v", f, err)
		}
		if got != f {
			t.Fatalf("round trip %q -> %q", f, got)
		}
	}
}

func TestSpecExample(t *testing.T) {
	records := [][]string{
		{"id", "text"},
		{"1", "a\tb"},
		{"2", "two\\lines?\nno"},
	}
	got, err := Encode(records)
	if err != nil {
		t.Fatal(err)
	}
	want := "id\ttext\n1\ta\\tb\n2\ttwo\\\\lines?\\nno\n"
	if string(got) != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if !matchesGrammar(got) {
		t.Fatal("example is not in the grammar")
	}
	back, err := Decode(got)
	if err != nil {
		t.Fatal(err)
	}
	if !sameRecords(back, records) {
		t.Fatalf("decode = %#v", back)
	}
}

func TestGrammarAndDecode(t *testing.T) {
	// Hand-checked against the README. ok is not derived from the decoder.
	tests := []struct {
		name string
		in   string
		ok   bool
		want [][]string
	}{
		{name: "empty", in: "", ok: true, want: nil},
		{name: "one lf", in: "\n", ok: true, want: [][]string{{""}}},
		{name: "two lf", in: "\n\n", ok: true, want: [][]string{{""}, {""}}},
		{name: "no nl", in: "a", ok: true, want: [][]string{{"a"}}},
		{name: "with nl", in: "a\n", ok: true, want: [][]string{{"a"}}},
		{name: "two records nl", in: "a\nb\n", ok: true, want: [][]string{{"a"}, {"b"}}},
		{name: "two records no final nl", in: "a\nb", ok: true, want: [][]string{{"a"}, {"b"}}},
		{name: "blank between", in: "a\n\nb\n", ok: true, want: [][]string{{"a"}, {""}, {"b"}}},
		{name: "leading blank", in: "\na\n", ok: true, want: [][]string{{""}, {"a"}}},
		{name: "fields", in: "a\tb\n", ok: true, want: [][]string{{"a", "b"}}},
		{name: "empty fields", in: "a\t\tb\n", ok: true, want: [][]string{{"a", "", "b"}}},
		{name: "leading tab", in: "\ta\n", ok: true, want: [][]string{{"", "a"}}},
		{name: "trailing tab", in: "a\t\n", ok: true, want: [][]string{{"a", ""}}},
		{name: "only tab", in: "\t", ok: true, want: [][]string{{"", ""}}},
		{name: "only tab nl", in: "\t\n", ok: true, want: [][]string{{"", ""}}},
		{name: "two tabs", in: "\t\t", ok: true, want: [][]string{{"", "", ""}}},
		{name: "crlf is cr then end", in: "a\r\nb\n", ok: true, want: [][]string{{"a\r"}, {"b"}}},
		{name: "cr inside", in: "a\rb\tc", ok: true, want: [][]string{{"a\rb", "c"}}},
		{name: "only cr", in: "\r", ok: true, want: [][]string{{"\r"}}},
		{name: "escaped tab", in: "a\\tb\n", ok: true, want: [][]string{{"a\tb"}}},
		{name: "escaped nl", in: "a\\nb\tc\n", ok: true, want: [][]string{{"a\nb", "c"}}},
		{name: "escaped backslash", in: "a\\\\b\n", ok: true, want: [][]string{{`a\b`}}},
		{name: "backslash then letter t", in: "\\\\t\n", ok: true, want: [][]string{{`\t`}}},
		{name: "backslash then letter n", in: "\\\\n\n", ok: true, want: [][]string{{`\n`}}},
		{name: "scan slash then tab", in: "\\\\\\t\n", ok: true, want: [][]string{{"\\\t"}}},
		{name: "unicode", in: "héllo\t世界\n", ok: true, want: [][]string{{"héllo", "世界"}}},
		{name: "nul", in: "a\x00b\tc\n", ok: true, want: [][]string{{"a\x00b", "c"}}},
		{name: "bad utf-8", in: "\xff\n", ok: true, want: [][]string{{"\xff"}}},
		{name: "varied columns", in: "a\nb\tc\n", ok: true, want: [][]string{{"a"}, {"b", "c"}}},
		{name: "spaces", in: " a \t b \n", ok: true, want: [][]string{{" a ", " b "}}},
		{name: "hash and quote", in: "#\t\"\n", ok: true, want: [][]string{{"#", `"`}}},
		{name: "bom is data", in: "\uFEFFa\n", ok: true, want: [][]string{{"\uFEFFa"}}},
		{name: "field then backslash", in: "a\tb\\\\\n", ok: true, want: [][]string{{"a", `b\`}}},
		{name: "trailing backslash", in: "\\\n", ok: false},
		{name: "bad q", in: "\\q\n", ok: false},
		{name: "uppercase T", in: "\\T\n", ok: false},
		{name: "backslash r", in: "\\r\n", ok: false},
		{name: "backslash then raw tab", in: "\\\t\n", ok: false},
		{name: "backslash nul", in: "\\\x00\n", ok: false},
		{name: "only backslash", in: "\\", ok: false},
		{name: "backslash at eof", in: "ab\\", ok: false},
		{name: "hex escape", in: "\\x6e\n", ok: false},
		{name: "bad second field", in: "a\tb\\\n", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesGrammar([]byte(tt.in)); got != tt.ok {
				t.Fatalf("matchesGrammar(%q) = %v, want %v", tt.in, got, tt.ok)
			}
			recs, err := Decode([]byte(tt.in))
			if !tt.ok {
				if err == nil {
					t.Fatalf("Decode succeeded: %#v", recs)
				}
				if recs != nil {
					t.Fatalf("partial records %#v", recs)
				}
				if !errors.Is(err, ErrInvalidEscape) {
					t.Fatalf("err %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !sameRecords(recs, tt.want) {
				t.Fatalf("Decode = %#v, want %#v", recs, tt.want)
			}
			out, err := Encode(recs)
			if err != nil {
				t.Fatal(err)
			}
			if !matchesGrammar(out) {
				t.Fatalf("re-encode %q not in grammar", out)
			}
			again, err := Decode(out)
			if err != nil {
				t.Fatal(err)
			}
			if !sameRecords(again, tt.want) {
				t.Fatalf("unstable %#v", again)
			}
			if tt.in == "" || strings.HasSuffix(tt.in, "\n") {
				if string(out) != tt.in {
					t.Fatalf("canonical encode %q, input %q", out, tt.in)
				}
			}
		})
	}
}

func TestParseErrorPosition(t *testing.T) {
	tests := []struct {
		in                 string
		record, field, off int
		next               int
	}{
		{"ok\tfine\nbad\\q\tzzz\n", 2, 1, 3, int('q')},
		{"a\t\\q\n", 1, 2, 0, int('q')},
		{"ab\\\n", 1, 1, 2, -1},
		{"a\tb\\\n", 1, 2, 1, -1},
	}
	for _, tt := range tests {
		_, err := Decode([]byte(tt.in))
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Fatalf("%q: %v", tt.in, err)
		}
		if pe.Record != tt.record || pe.Field != tt.field || pe.Offset != tt.off {
			t.Fatalf("%q: record %d field %d offset %d", tt.in, pe.Record, pe.Field, pe.Offset)
		}
		var ee *EscapeError
		if !errors.As(err, &ee) || ee.Next != tt.next {
			t.Fatalf("%q: escape %#v", tt.in, ee)
		}
		if !strings.Contains(err.Error(), "record "+strconv.Itoa(tt.record)) {
			t.Fatalf("message %q", err.Error())
		}
	}
}

func TestReadStopsOnBadRecord(t *testing.T) {
	r := NewReader(strings.NewReader("a\tb\nbad\\q\n"))
	rec, err := r.Read()
	if err != nil || !sameRecords([][]string{rec}, [][]string{{"a", "b"}}) {
		t.Fatalf("first %q %v", rec, err)
	}
	rec, err = r.Read()
	if rec != nil || !errors.Is(err, ErrInvalidEscape) {
		t.Fatalf("second %q %v", rec, err)
	}
	_, err2 := r.Read()
	if err2 != err {
		t.Fatalf("sticky %v, want %v", err2, err)
	}
}

func TestReadEOF(t *testing.T) {
	r := NewReader(strings.NewReader(""))
	rec, err := r.Read()
	if rec != nil || !errors.Is(err, io.EOF) {
		t.Fatalf("%q %v", rec, err)
	}
	_, err = r.Read()
	if !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	recs, err := NewReader(bytes.NewReader(nil)).ReadAll()
	if err != nil || recs != nil {
		t.Fatalf("%#v %v", recs, err)
	}
}

func TestCleanEOFEndsRecord(t *testing.T) {
	r := NewReader(bytes.NewReader([]byte("a\tb")))
	rec, err := r.Read()
	if err != nil || !sameRecords([][]string{rec}, [][]string{{"a", "b"}}) {
		t.Fatalf("%q %v", rec, err)
	}
	_, err = r.Read()
	if !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

func TestReadErrorDoesNotEndRecord(t *testing.T) {
	r := NewReader(io.MultiReader(bytes.NewReader([]byte("a\tb")), failReader{}))
	rec, err := r.Read()
	if rec != nil || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("%q %v", rec, err)
	}
	_, err2 := r.Read()
	if !errors.Is(err2, io.ErrClosedPipe) {
		t.Fatal(err2)
	}

	r = NewReader(io.MultiReader(bytes.NewReader([]byte("a\tb\n")), failReader{}))
	rec, err = r.Read()
	if err != nil || !sameRecords([][]string{rec}, [][]string{{"a", "b"}}) {
		t.Fatalf("first %q %v", rec, err)
	}
	rec, err = r.Read()
	if rec != nil || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("second %q %v", rec, err)
	}
}

func TestReaderRetainsRecords(t *testing.T) {
	r := NewReader(strings.NewReader("abc\tdef\nxyz\n"))
	a, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if a[0] != "abc" || a[1] != "def" || b[0] != "xyz" {
		t.Fatalf("%q %q", a, b)
	}
}

func TestDecodeDoesNotAliasInput(t *testing.T) {
	in := []byte("abc\tdef\n")
	got, err := Decode(in)
	if err != nil {
		t.Fatal(err)
	}
	for i := range in {
		in[i] = 'X'
	}
	if got[0][0] != "abc" || got[0][1] != "def" {
		t.Fatalf("%#v", got)
	}
}

func TestLargeField(t *testing.T) {
	const n = 128 * 1024
	field := strings.Repeat("x", n) + "\t\n\\" + "\r"
	records := [][]string{{field, ""}, {"", "y"}}
	b, err := Encode(records)
	if err != nil {
		t.Fatal(err)
	}
	if !matchesGrammar(b) {
		t.Fatal("grammar")
	}
	got, err := NewReader(bytes.NewReader(b)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if !sameRecords(got, records) {
		t.Fatal("mismatch")
	}
}

func TestManyFields(t *testing.T) {
	fields := make([]string, 1000)
	for i := range fields {
		if i%3 == 0 {
			fields[i] = ""
		} else {
			fields[i] = "a\tb"
		}
	}
	b, err := Encode([][]string{fields})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !sameRecords(got, [][]string{fields}) {
		t.Fatal("mismatch")
	}
}

func TestEncodeEmptyRecord(t *testing.T) {
	for _, records := range [][][]string{
		{{}},
		{nil},
		{{"a"}, {}},
		{{"a"}, nil, {"b"}},
	} {
		b, err := Encode(records)
		if !errors.Is(err, ErrEmptyRecord) || b != nil {
			t.Fatalf("Encode(%#v) = %q, %v", records, b, err)
		}
	}
	b, err := Encode(nil)
	if err != nil || len(b) != 0 {
		t.Fatalf("%q %v", b, err)
	}
	b, err = Encode([][]string{})
	if err != nil || len(b) != 0 {
		t.Fatalf("%q %v", b, err)
	}
}

func TestWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if w.Error() != nil {
		t.Fatal(w.Error())
	}
	if err := w.Write([]string{"a", "b\tc"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Write([]string{""}); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(nil); !errors.Is(err, ErrEmptyRecord) {
		t.Fatal(err)
	}
	if err := w.Write([]string{}); !errors.Is(err, ErrEmptyRecord) {
		t.Fatal(err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "a\tb\\tc\n\n" {
		t.Fatalf("%q", buf.String())
	}

	buf.Reset()
	w = NewWriter(&buf)
	if err := w.WriteAll([][]string{{"a"}, {}}); !errors.Is(err, ErrEmptyRecord) {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("flushed on error: %q", buf.Bytes())
	}
	w.Flush()
	if buf.String() != "a\n" {
		t.Fatalf("%q", buf.String())
	}
}

func TestWriterIOError(t *testing.T) {
	w := NewWriter(failWriter{})
	if err := w.Write(nil); !errors.Is(err, ErrEmptyRecord) {
		t.Fatal(err)
	}
	if w.Error() != nil {
		t.Fatal(w.Error())
	}
	if err := w.Write([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if !errors.Is(w.Error(), io.ErrClosedPipe) {
		t.Fatal(w.Error())
	}

	w = NewWriter(failWriter{})
	err := w.Write([]string{strings.Repeat("a", 8192)})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if !errors.Is(w.Error(), io.ErrClosedPipe) {
		t.Fatal(w.Error())
	}
}

func TestWriterMatchesEncode(t *testing.T) {
	records := [][]string{{"a", "b"}, {"c\n", `d\`}, {"\r"}}
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteAll(records); err != nil {
		t.Fatal(err)
	}
	enc, err := Encode(records)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), enc) {
		t.Fatalf("writer %q encode %q", buf.Bytes(), enc)
	}
	got, err := Decode(buf.Bytes())
	if err != nil || !sameRecords(got, records) {
		t.Fatalf("%#v %v", got, err)
	}
}

func TestRandomRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(20261029))
	atoms := []string{
		"", "a", "b", " ", `\`, "\t", "\n", "\r", "\x00", "\xff",
		"#", `"`, "é", "世", `\t`, `\n`, "\\", "\t\n\\",
	}
	for n := 0; n < 200; n++ {
		nrec := rng.Intn(6)
		records := make([][]string, nrec)
		for i := 0; i < nrec; i++ {
			nf := 1 + rng.Intn(5)
			fields := make([]string, nf)
			for j := 0; j < nf; j++ {
				var b strings.Builder
				for k := 0; k < rng.Intn(8); k++ {
					b.WriteString(atoms[rng.Intn(len(atoms))])
				}
				fields[j] = b.String()
			}
			records[i] = fields
		}
		out, err := Encode(records)
		if err != nil {
			t.Fatal(err)
		}
		if !matchesGrammar(out) {
			t.Fatalf("not in grammar: %q", out)
		}
		got, err := Decode(out)
		if err != nil {
			t.Fatal(err)
		}
		if !sameRecords(got, records) {
			t.Fatalf("got %#v want %#v encoded %q", got, records, out)
		}
	}
}

func FuzzGrammarAgrees(f *testing.F) {
	for _, s := range []string{
		"", "\n", "\n\n", "a", "a\n", "a\tb", "a\tb\n", "\\", "\\t",
		"\\\\t", "\\q", "a\\\n", "\t", "\t\n", "\r\n", "a\r\nb", "\t\t",
		"a\n\nb", "\\\x00", "\\T", "\\n", "\\\\n", "\xff", "#\t\"\n",
		"id\ttext\n1\ta\\tb\n2\ttwo\\\\lines?\\nno\n",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data string) {
		ok := matchesGrammar([]byte(data))
		recs, err := Decode([]byte(data))
		if ok {
			if err != nil {
				t.Fatalf("grammar accepts %q but decode: %v", data, err)
			}
			out, err := Encode(recs)
			if err != nil {
				t.Fatal(err)
			}
			if !matchesGrammar(out) {
				t.Fatalf("encode not in grammar: %q", out)
			}
			recs2, err := Decode(out)
			if err != nil {
				t.Fatal(err)
			}
			if !sameRecords(recs, recs2) {
				t.Fatalf("data %q recs %#v vs %#v", data, recs, recs2)
			}
			return
		}
		if err == nil {
			t.Fatalf("grammar rejects %q but decode produced %#v", data, recs)
		}
		if recs != nil {
			t.Fatalf("partial records on error: %#v", recs)
		}
		if !errors.Is(err, ErrInvalidEscape) {
			t.Fatalf("unexpected err %v for %q", err, data)
		}
	})
}

func FuzzRoundTrip(f *testing.F) {
	f.Add("a\x00b\x01c\t\n\\\r")
	f.Add("")
	f.Add("\x01")
	f.Add("é\x00世\x01\r\n\\")
	f.Fuzz(func(t *testing.T, raw string) {
		parts := strings.Split(raw, "\x01")
		records := make([][]string, len(parts))
		for i, p := range parts {
			records[i] = strings.Split(p, "\x00")
		}
		out, err := Encode(records)
		if err != nil {
			t.Fatal(err)
		}
		if !matchesGrammar(out) {
			t.Fatalf("not in grammar: %q", out)
		}
		got, err := Decode(out)
		if err != nil {
			t.Fatalf("decode %q: %v", out, err)
		}
		if !sameRecords(got, records) {
			t.Fatalf("got %#v want %#v encoded %q", got, records, out)
		}
	})
}

func FuzzEscape(f *testing.F) {
	f.Add("")
	f.Add("\\\t\n\r")
	f.Add("\\t\\n\\\\")
	f.Add("héllo\x00\xff")
	f.Fuzz(func(t *testing.T, field string) {
		enc := Escape(field)
		if strings.ContainsAny(enc, "\t\n") {
			t.Fatalf("raw delimiter in %q", enc)
		}
		got, err := Unescape(enc)
		if err != nil || got != field {
			t.Fatalf("got %q err %v want %q (enc %q)", got, err, field, enc)
		}
		if !matchesGrammar([]byte(enc + "\n")) {
			t.Fatalf("escaped field not in grammar: %q", enc)
		}
	})
}

type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func sameRecords(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

// matchesGrammar reports whether data is a file in the README ABNF.
// terminated-record is fields LF. A remaining tail with no LF is an
// unterminated-record only when it is nonempty-fields.
func matchesGrammar(data []byte) bool {
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			return len(data) > 0 && validFields(data)
		}
		if !validFields(data[:i]) {
			return false
		}
		data = data[i+1:]
	}
	return true
}

func validFields(line []byte) bool {
	start := 0
	for i := 0; i <= len(line); i++ {
		if i == len(line) || line[i] == '\t' {
			if !validField(line[start:i]) {
				return false
			}
			start = i + 1
		}
	}
	return true
}

func validField(f []byte) bool {
	for i := 0; i < len(f); i++ {
		c := f[i]
		if c == '\\' {
			if i+1 >= len(f) {
				return false
			}
			switch f[i+1] {
			case '\\', 't', 'n':
				i++
			default:
				return false
			}
			continue
		}
		if c == '\t' || c == '\n' {
			return false
		}
	}
	return true
}
