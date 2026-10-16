package dotenv

import (
	"bytes"
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) *Document {
	t.Helper()
	doc, err := Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func vals(t *testing.T, doc *Document, env string) map[string]string {
	t.Helper()
	pairs, err := doc.Select(env)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, p := range pairs {
		m[p.Key] = p.Value
	}
	return m
}

func TestCommentsBlanksExportEquals(t *testing.T) {
	doc := mustParse(t, `
# comment
export API_KEY=pogo_test_api_key_example

EQUALS_VALUE=a=b=c
EMPTY=
`)
	m := vals(t, doc, "")
	if m["API_KEY"] != "pogo_test_api_key_example" {
		t.Fatalf("API_KEY %q", m["API_KEY"])
	}
	if m["EQUALS_VALUE"] != "a=b=c" {
		t.Fatalf("EQUALS %q", m["EQUALS_VALUE"])
	}
	if m["EMPTY"] != "" {
		t.Fatalf("EMPTY %q", m["EMPTY"])
	}
}

func TestQuotesEscapesAndInlineComment(t *testing.T) {
	doc := mustParse(t, "QUOTED_VALUE=\"say \\\"hello\\\" world\"\nSPACED_VALUE=\"hello world\"\nSINGLE='a\\nb'\nPLAIN=hello # comment\nHASH=foo#bar\n")
	m := vals(t, doc, "")
	if m["QUOTED_VALUE"] != `say "hello" world` {
		t.Fatalf("quoted %q", m["QUOTED_VALUE"])
	}
	if m["SPACED_VALUE"] != "hello world" {
		t.Fatalf("spaced %q", m["SPACED_VALUE"])
	}
	if m["SINGLE"] != `a\nb` {
		t.Fatalf("single %q", m["SINGLE"])
	}
	if m["PLAIN"] != "hello" {
		t.Fatalf("plain %q", m["PLAIN"])
	}
	if m["HASH"] != "foo#bar" {
		t.Fatalf("hash %q", m["HASH"])
	}
}

func TestMultilineDoubleQuotes(t *testing.T) {
	doc := mustParse(t, "MULTILINE_VALUE=\"line one\nline two\"\nNEXT=1\n")
	m := vals(t, doc, "")
	if m["MULTILINE_VALUE"] != "line one\nline two" {
		t.Fatalf("%q", m["MULTILINE_VALUE"])
	}
	if m["NEXT"] != "1" {
		t.Fatalf("next %q", m["NEXT"])
	}
}

func TestEscapedNewlineAndSpacesAroundEquals(t *testing.T) {
	doc := mustParse(t, "A = \"a\\nb\"\n  export   B\t=\tvalue\n")
	m := vals(t, doc, "")
	if m["A"] != "a\nb" {
		t.Fatalf("A %q", m["A"])
	}
	if m["B"] != "value" {
		t.Fatalf("B %q", m["B"])
	}
}

func TestSectionsMerge(t *testing.T) {
	doc := mustParse(t, `
PORT=1
API_KEY=top

[production]
API_KEY=prod
EXTRA=1

[dev]
API_KEY=dev
`)
	top := vals(t, doc, "")
	if top["PORT"] != "1" || top["API_KEY"] != "top" {
		t.Fatalf("top %#v", top)
	}
	if _, ok := top["EXTRA"]; ok {
		t.Fatal("section key leaked into top")
	}
	prod := vals(t, doc, "production")
	if prod["PORT"] != "1" || prod["API_KEY"] != "prod" || prod["EXTRA"] != "1" {
		t.Fatalf("prod %#v", prod)
	}
	if _, err := doc.Select("missing"); err == nil {
		t.Fatal("expected missing section error")
	}
	if strings.Join(doc.Names(), ",") != "production,dev" {
		t.Fatalf("names %v", doc.Names())
	}
}

func TestCRLFAndBOM(t *testing.T) {
	raw := []byte("\xEF\xBB\xBFA=1\r\nB=2\r\n")
	doc, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	m := vals(t, doc, "")
	if m["A"] != "1" || m["B"] != "2" {
		t.Fatalf("%#v", m)
	}
}

func TestLastDuplicateWins(t *testing.T) {
	doc := mustParse(t, "A=1\nA=2\n")
	pairs, err := doc.Select("")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 || pairs[0].Value != "2" {
		t.Fatalf("%#v", pairs)
	}
}

func TestFormatRoundTrip(t *testing.T) {
	in := []Pair{
		{Key: "QUOTED_VALUE", Value: "say \"hello\" world"},
		{Key: "SPACED_VALUE", Value: "hello world"},
		{Key: "EQUALS_VALUE", Value: "a=b=c"},
		{Key: "MULTILINE_VALUE", Value: "line one\nline two"},
		{Key: "PLAIN", Value: "ok"},
	}
	doc, err := Parse(Format(in))
	if err != nil {
		t.Fatal(err)
	}
	got, err := doc.Select("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(in) {
		t.Fatalf("%#v", got)
	}
	for i := range in {
		if got[i] != in[i] {
			t.Fatalf("pair %d got %#v want %#v", i, got[i], in[i])
		}
	}
}

func TestErrors(t *testing.T) {
	if _, err := Parse([]byte("NOT VALID\n")); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Parse([]byte("A=\"unterminated\n")); err == nil {
		t.Fatal("expected unterminated")
	}
	if _, err := Parse([]byte{0xff, 0xfe}); err == nil {
		t.Fatal("expected utf-8 error")
	}
}

func TestSectionInsideQuotesIsValue(t *testing.T) {
	doc := mustParse(t, "A=\"[production]\"\n[production]\nB=1\n")
	m := vals(t, doc, "")
	if m["A"] != "[production]" {
		t.Fatalf("%q", m["A"])
	}
	if _, ok := m["B"]; ok {
		t.Fatal("B should be in the section only")
	}
}

func TestPayloadPreservedConcept(t *testing.T) {
	raw := []byte("# keep me\n\nA=1\n")
	if !bytes.Contains(raw, []byte("# keep me")) {
		t.Fatal("fixture")
	}
	doc := mustParse(t, string(raw))
	m := vals(t, doc, "")
	if m["A"] != "1" || len(m) != 1 {
		t.Fatalf("%#v", m)
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte("A=b\n"))
	f.Add([]byte("export A=\"x\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil || doc == nil {
			return
		}
		if _, err := doc.Select(""); err != nil {
			t.Fatal(err)
		}
	})
}
