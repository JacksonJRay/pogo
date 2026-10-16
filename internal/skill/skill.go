// Package skill is the document printed by `pogo skill`.
//
// The text combines the core guide (SKILL.md), the command reference
// (reference.md), and per-framework notes (frameworks.md). Add a framework
// by editing frameworks.md.
package skill

import (
	"strings"

	_ "embed"
)

//go:embed SKILL.md
var core []byte

//go:embed reference.md
var reference []byte

//go:embed frameworks.md
var frameworks []byte

// Document is the exact bytes `pogo skill` writes to stdout.
var Document = concat(core, reference, frameworks)

func concat(parts ...[]byte) []byte {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// Framework describes one `## framework <id>` section.
type Framework struct {
	ID   string
	Body string
}

// Frameworks returns sections in document order.
func Frameworks() []Framework {
	text := string(Document)
	const marker = "## framework "
	var out []Framework
	rest := text
	for {
		i := strings.Index(rest, marker)
		if i < 0 {
			break
		}
		rest = rest[i+len(marker):]
		lineEnd := strings.IndexByte(rest, '\n')
		if lineEnd < 0 {
			break
		}
		id := strings.TrimSpace(rest[:lineEnd])
		rest = rest[lineEnd+1:]
		next := strings.Index(rest, marker)
		body := rest
		if next >= 0 {
			body = rest[:next]
		}
		out = append(out, Framework{ID: id, Body: "## framework " + id + "\n" + body})
		if next < 0 {
			break
		}
	}
	return out
}

// FrameworkByID returns the section including its heading.
func FrameworkByID(id string) (string, bool) {
	for _, f := range Frameworks() {
		if f.ID == id {
			return f.Body, true
		}
	}
	return "", false
}

// IDs returns framework ids.
func IDs() []string {
	fs := Frameworks()
	ids := make([]string, len(fs))
	for i, f := range fs {
		ids[i] = f.ID
	}
	return ids
}
