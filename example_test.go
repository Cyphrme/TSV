package tsv

import "fmt"

func ExampleEncode() {
	b, err := Encode([][]string{
		{"id", "text"},
		{"1", "a\tb"},
		{"2", "two\\lines?\nno"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%q\n", b)
	// Output:
	// "id\ttext\n1\ta\\tb\n2\ttwo\\\\lines?\\nno\n"
}

func ExampleUnescape() {
	for _, in := range []string{`\t`, `\n`, `\\`, `\\t`, `\\n`} {
		out, err := Unescape(in)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%q\n", out)
	}
	// Output:
	// "\t"
	// "\n"
	// "\\"
	// "\\t"
	// "\\n"
}
