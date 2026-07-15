package frontmatter

import (
	"reflect"
	"testing"
)

func TestParseYAMLFrontmatter(t *testing.T) {
	got, body, err := Parse([]byte("---\nname: demo\ndescription: >-\n  A folded description.\ntags:\n  - one\n  - two\n---\n\n# Demo\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := map[string]any{
		"name":        "demo",
		"description": "A folded description.",
		"tags":        []any{"one", "two"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("frontmatter = %#v, want %#v", got, want)
	}
	if body != "\n# Demo\n" {
		t.Fatalf("body = %q, want %q", body, "\n# Demo\n")
	}
}

func TestParseRejectsAmbiguousYAML(t *testing.T) {
	for name, input := range map[string]string{
		"duplicate key": "---\nname: one\nname: two\n---\nbody",
		"alias":         "---\ndefaults: &defaults\n  name: demo\ncopy: *defaults\n---\nbody",
		"timestamp":     "---\ncreated: 2026-07-13\n---\nbody",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Parse([]byte(input)); err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
		})
	}
}

func TestParseRejectsInvalidUTF8(t *testing.T) {
	input := append([]byte("# Demo\n"), 0xff)
	if _, _, err := Parse(input); err == nil {
		t.Fatal("Parse() error = nil, want invalid UTF-8 error")
	}
}

func TestParseWithoutFrontmatter(t *testing.T) {
	const input = "# Demo\n"
	got, body, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got) != 0 || body != input {
		t.Fatalf("Parse() = %#v, %q; want empty frontmatter and original body", got, body)
	}
}
