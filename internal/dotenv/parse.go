// Package dotenv parses the env files Pogo stores and injects.
//
// The encrypted payload is the raw dotenv text. Parsing happens only when
// Pogo needs to turn that text into environment variables. Comments, blank
// lines, and section headers are kept in the stored text and ignored when
// building the environment.
//
// Sections are a Pogo extension. Lines before the first [name] header are
// the top-level environment. A [name] header starts a section that is used
// only when that name is selected. Keys inside a section replace top-level
// keys with the same name. There is no syntax to return to the top level;
// put shared keys first.
//
// Values are not expanded. $OTHER is literal text, which avoids surprises
// and keeps secrets from being interpolated by accident.
package dotenv

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

// Pair is one KEY=VALUE assignment.
type Pair struct {
	Key   string
	Value string
}

// Document is a parsed env file.
type Document struct {
	Top      []Pair
	Sections map[string][]Pair
	Order    []string
}

// Parse reads dotenv text. It accepts a UTF-8 BOM and both LF and CRLF.
func Parse(data []byte) (*Document, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("environment file is not valid UTF-8")
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))

	doc := &Document{Sections: map[string][]Pair{}}
	current := ""
	lineNo := 1
	i := 0
	for i < len(data) {
		line, _ := peekLine(data, i)
		trim := bytes.TrimSpace(line)
		if len(trim) == 0 || trim[0] == '#' {
			i, lineNo = skipLine(data, i, lineNo)
			continue
		}
		if name, ok := sectionName(trim); ok {
			if _, exists := doc.Sections[name]; !exists {
				doc.Order = append(doc.Order, name)
				doc.Sections[name] = nil
			}
			current = name
			i, lineNo = skipLine(data, i, lineNo)
			continue
		}
		key, val, ni, nl, err := parseAssignment(data, i, lineNo)
		if err != nil {
			return nil, err
		}
		p := Pair{Key: key, Value: val}
		if current == "" {
			doc.Top = append(doc.Top, p)
		} else {
			doc.Sections[current] = append(doc.Sections[current], p)
		}
		i, lineNo = ni, nl
	}
	return doc, nil
}

// Names returns section names in the order they first appeared.
func (d *Document) Names() []string {
	out := make([]string, len(d.Order))
	copy(out, d.Order)
	return out
}

// Select returns the environment to inject.
// An empty name returns only top-level keys.
// A non-empty name merges that section over the top level.
// Duplicate keys: the later assignment wins, and the key keeps its first position.
func (d *Document) Select(name string) ([]Pair, error) {
	top := dedupe(d.Top)
	if name == "" {
		return top, nil
	}
	sec, ok := d.Sections[name]
	if !ok {
		return nil, fmt.Errorf("environment %q not found", name)
	}
	return merge(top, dedupe(sec)), nil
}

// Format writes pairs as dotenv text. Values that contain spaces, quotes,
// newlines, or comment markers are double-quoted.
func Format(pairs []Pair) []byte {
	var b bytes.Buffer
	for _, p := range pairs {
		b.WriteString(p.Key)
		b.WriteByte('=')
		b.WriteString(quote(p.Value))
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func dedupe(in []Pair) []Pair {
	idx := map[string]int{}
	var out []Pair
	for _, p := range in {
		if i, ok := idx[p.Key]; ok {
			out[i].Value = p.Value
			continue
		}
		idx[p.Key] = len(out)
		out = append(out, p)
	}
	return out
}

func merge(base, over []Pair) []Pair {
	out := append([]Pair(nil), base...)
	idx := map[string]int{}
	for i, p := range out {
		idx[p.Key] = i
	}
	for _, p := range over {
		if i, ok := idx[p.Key]; ok {
			out[i].Value = p.Value
			continue
		}
		idx[p.Key] = len(out)
		out = append(out, p)
	}
	return out
}

func quote(v string) string {
	if v == "" || !needsQuote(v) {
		return v
	}
	var b bytes.Buffer
	b.WriteByte('"')
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(v[i])
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(v[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}

func needsQuote(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '"' || c == '\'' || c == '\\' || c == '#' || c == ' ' || c == '\t' || c == '\n' || c == '\r' || c < 0x20 {
			return true
		}
	}
	return false
}

func sectionName(trim []byte) (string, bool) {
	if len(trim) == 0 || trim[0] != '[' {
		return "", false
	}
	end := bytes.IndexByte(trim, ']')
	if end <= 1 {
		return "", false
	}
	name := trim[1:end]
	if !validSection(name) {
		return "", false
	}
	rest := bytes.TrimSpace(trim[end+1:])
	if len(rest) == 0 || rest[0] == '#' {
		return string(name), true
	}
	return "", false
}

func validSection(name []byte) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for i, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '_' || c == '-' || c == '.':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func parseAssignment(data []byte, i, lineNo int) (key, value string, newIdx, newLine int, err error) {
	n := len(data)
	for i < n && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	if bytes.HasPrefix(data[i:], []byte("export")) {
		after := i + len("export")
		if after < n && (data[after] == ' ' || data[after] == '\t') {
			i = after
			for i < n && (data[i] == ' ' || data[i] == '\t') {
				i++
			}
		}
	}
	if i >= n || !isKeyStart(data[i]) {
		return "", "", 0, 0, parseErr(lineNo)
	}
	keyStart := i
	i++
	for i < n && isKeyCont(data[i]) {
		i++
	}
	key = string(data[keyStart:i])
	for i < n && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	if i >= n || data[i] != '=' {
		return "", "", 0, 0, parseErr(lineNo)
	}
	i++
	for i < n && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	if i >= n || data[i] == '\n' || data[i] == '#' {
		ni, nl := skipLine(data, i, lineNo)
		return key, "", ni, nl, nil
	}
	if data[i] == '"' || data[i] == '\'' {
		val, ni, nl, err := scanQuoted(data, i, lineNo, data[i])
		if err != nil {
			return "", "", 0, 0, err
		}
		return key, val, ni, nl, nil
	}
	lineEnd := i
	for lineEnd < n && data[lineEnd] != '\n' {
		lineEnd++
	}
	raw := stripInlineComment(data[i:lineEnd])
	raw = bytes.TrimRight(raw, " \t")
	ni, nl := skipLine(data, lineEnd, lineNo)
	return key, string(raw), ni, nl, nil
}

func scanQuoted(data []byte, i, lineNo int, quote byte) (string, int, int, error) {
	i++
	var b bytes.Buffer
	n := len(data)
	for i < n {
		c := data[i]
		if quote == '"' && c == '\\' {
			if i+1 >= n {
				return "", 0, 0, fmt.Errorf("line %d: quoted value ends with a trailing backslash", lineNo)
			}
			nxt := data[i+1]
			switch nxt {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\\', '"', '\'', '$':
				b.WriteByte(nxt)
			case '\n':
				lineNo++
			default:
				b.WriteByte(nxt)
			}
			i += 2
			continue
		}
		if c == quote {
			i++
			j := i
			for j < n && (data[j] == ' ' || data[j] == '\t') {
				j++
			}
			if j < n && data[j] != '\n' && data[j] != '#' {
				return "", 0, 0, fmt.Errorf("line %d: unexpected text after the closing quote", lineNo)
			}
			ni, nl := skipLine(data, j, lineNo)
			return b.String(), ni, nl, nil
		}
		if c == '\n' {
			lineNo++
		}
		b.WriteByte(c)
		i++
	}
	return "", 0, 0, fmt.Errorf("line %d: missing closing quote", lineNo)
}

func stripInlineComment(raw []byte) []byte {
	for i := 0; i < len(raw); i++ {
		if raw[i] == '#' && i > 0 && (raw[i-1] == ' ' || raw[i-1] == '\t') {
			return raw[:i]
		}
	}
	return raw
}

func parseErr(lineNo int) error {
	return fmt.Errorf("line %d: expected a name, then =, then a value", lineNo)
}

func peekLine(data []byte, i int) ([]byte, int) {
	j := i
	for j < len(data) && data[j] != '\n' {
		j++
	}
	return data[i:j], j
}

func skipLine(data []byte, i, lineNo int) (int, int) {
	for i < len(data) && data[i] != '\n' {
		i++
	}
	if i < len(data) && data[i] == '\n' {
		return i + 1, lineNo + 1
	}
	return i, lineNo
}

func isKeyStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isKeyCont(c byte) bool {
	return isKeyStart(c) || (c >= '0' && c <= '9')
}
