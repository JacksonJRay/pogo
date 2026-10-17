package passphrase

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Desktop helpers are probed by executable name. Distro identity is not read
// from a release file. PATH is searched first, which is how NixOS, Alpine,
// Void, and Gentoo expose these programs. The extra directories cover
// Debian, Ubuntu, Pop!_OS, Fedora, RHEL, Rocky, Alma, Arch, and openSUSE
// libexec layouts, plus /usr/bin and the NixOS profile path when PATH was
// emptied. Nothing is installed and no privilege helper is called.
//
// GNOME, ubuntu, and pop prefer the gcr family. KDE prefers ksshaskpass,
// then kdialog. An unknown desktop uses the first askpass-protocol binary,
// then zenity, kdialog, or yad.

type helperKind int

const (
	kindAskpass helperKind = iota
	kindZenity
	kindKdialog
	kindYad
)

type helperSpec struct {
	label     string
	pathNames []string
	files     []string
	dirs      []string
	kind      helperKind
}

type foundHelper struct {
	label string
	path  string
	kind  helperKind
}

func (h foundHelper) argv(prompt string) []string {
	switch h.kind {
	case kindZenity:
		return []string{h.path, "--password", "--title=Pogo", "--text", prompt}
	case kindKdialog:
		return []string{h.path, "--title", "Pogo", "--password", prompt}
	case kindYad:
		return []string{h.path, "--entry", "--hide-text", "--title=Pogo", "--text", prompt}
	default:
		return []string{h.path, prompt}
	}
}

func findHelpers() []foundHelper {
	var found []foundHelper
	seen := map[string]bool{}
	for _, spec := range helperOrder(desktopFamily(os.Getenv("XDG_CURRENT_DESKTOP"))) {
		path := locate(spec)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		found = append(found, foundHelper{label: spec.label, path: path, kind: spec.kind})
	}
	return found
}

// helperBlock reports why a found helper cannot show a prompt.
// gcr4-ssh-askpass and gcr-ssh-askpass are relays. GNOME's prompter sets
// GCR_SSH_ASKPASS_SOCKET and then execs them. Run directly, they exit
// without a dialog ("not meant to be run directly"). Skip them unless that
// socket exists, and try the next helper (zenity, kdialog, gnome-ssh-askpass).
func helperBlock(h foundHelper) string {
	switch h.label {
	case "gcr4-ssh-askpass", "gcr-ssh-askpass":
		if !gcrSocketReady() {
			return h.label + " skipped (GCR askpass socket is not available)"
		}
	}
	return ""
}

func gcrSocketReady() bool {
	path := os.Getenv("GCR_SSH_ASKPASS_SOCKET")
	return path != "" && usableSocket(path)
}

func desktopFamily(xdg string) string {
	parts := strings.FieldsFunc(xdg, func(r rune) bool {
		return r == ':' || r == ';' || r == ','
	})
	for _, part := range parts {
		s := strings.ToLower(strings.TrimSpace(part))
		switch {
		case s == "gnome" || s == "ubuntu" || s == "pop" || strings.Contains(s, "gnome") || strings.HasPrefix(s, "pop"):
			return "gnome"
		case s == "kde" || strings.Contains(s, "kde") || strings.Contains(s, "plasma"):
			return "kde"
		}
	}
	return ""
}

func helperOrder(family string) []helperSpec {
	all := defaultHelpers()
	var first []string
	switch family {
	case "gnome":
		first = []string{"gcr4-ssh-askpass", "gcr-ssh-askpass", "seahorse/ssh-askpass", "gnome-ssh-askpass", "gnome-ssh-askpass3"}
	case "kde":
		first = []string{"ksshaskpass", "kdialog"}
	default:
		return all
	}
	rank := map[string]int{}
	for i, label := range first {
		rank[label] = i
	}
	front := make([]helperSpec, len(first))
	var rest []helperSpec
	used := map[string]bool{}
	for _, spec := range all {
		if i, ok := rank[spec.label]; ok && !used[spec.label] {
			front[i] = spec
			used[spec.label] = true
			continue
		}
		rest = append(rest, spec)
	}
	out := make([]helperSpec, 0, len(all))
	for _, spec := range front {
		if spec.label != "" {
			out = append(out, spec)
		}
	}
	return append(out, rest...)
}

// profileDirs are stable locations that are not store hashes. PATH is still
// searched first. Alpine has no /usr/libexec; those stats simply fail.
func profileDirs(dirs []string) []string {
	extra := []string{
		"/usr/bin",
		"/bin",
		"/usr/local/bin",
		"/run/current-system/sw/bin",
		"/run/current-system/sw/libexec",
	}
	out := make([]string, 0, len(dirs)+len(extra))
	out = append(out, dirs...)
	return append(out, extra...)
}

func defaultHelpers() []helperSpec {
	lib := profileDirs([]string{
		"/usr/libexec",
		"/usr/lib",
		"/usr/lib64",
		"/usr/libexec/openssh",
		"/usr/lib/openssh",
		"/usr/lib64/openssh",
		"/usr/lib/ssh",
		"/usr/lib64/ssh",
		"/usr/libexec/ssh",
	})
	gcr := profileDirs([]string{
		"/usr/libexec",
		"/usr/libexec/gcr",
		"/usr/libexec/gcr4",
		"/usr/lib",
		"/usr/lib/gcr",
		"/usr/lib/gcr4",
		"/usr/lib/gcr-4",
		"/usr/lib64",
		"/usr/lib64/gcr",
		"/usr/lib64/gcr-4",
	})
	seahorse := profileDirs([]string{"/usr/libexec", "/usr/lib", "/usr/lib64"})
	return []helperSpec{
		{label: "gcr4-ssh-askpass", pathNames: []string{"gcr4-ssh-askpass"}, files: []string{"gcr4-ssh-askpass", "gcr-4/gcr4-ssh-askpass"}, dirs: gcr, kind: kindAskpass},
		{label: "gcr-ssh-askpass", pathNames: []string{"gcr-ssh-askpass"}, files: []string{"gcr-ssh-askpass"}, dirs: gcr, kind: kindAskpass},
		{label: "seahorse/ssh-askpass", pathNames: []string{"seahorse-ssh-askpass"}, files: []string{"seahorse/ssh-askpass", "seahorse/seahorse-ssh-askpass"}, dirs: seahorse, kind: kindAskpass},
		{label: "gnome-ssh-askpass", pathNames: []string{"gnome-ssh-askpass"}, files: []string{"gnome-ssh-askpass"}, dirs: lib, kind: kindAskpass},
		{label: "gnome-ssh-askpass3", pathNames: []string{"gnome-ssh-askpass3"}, files: []string{"gnome-ssh-askpass3"}, dirs: lib, kind: kindAskpass},
		{label: "ksshaskpass", pathNames: []string{"ksshaskpass"}, files: []string{"ksshaskpass"}, dirs: lib, kind: kindAskpass},
		{label: "lxqt-openssh-askpass", pathNames: []string{"lxqt-openssh-askpass"}, files: []string{"lxqt-openssh-askpass"}, dirs: lib, kind: kindAskpass},
		{label: "ssh-askpass", pathNames: []string{"ssh-askpass"}, files: []string{"ssh-askpass", "openssh/ssh-askpass", "ssh/ssh-askpass"}, dirs: lib, kind: kindAskpass},
		{label: "zenity", pathNames: []string{"zenity"}, files: []string{"zenity"}, dirs: lib, kind: kindZenity},
		{label: "kdialog", pathNames: []string{"kdialog"}, files: []string{"kdialog"}, dirs: lib, kind: kindKdialog},
		{label: "yad", pathNames: []string{"yad"}, files: []string{"yad"}, dirs: lib, kind: kindYad},
	}
}

func locate(spec helperSpec) string {
	// An empty PATH must not search "." the way exec.LookPath does.
	if os.Getenv("PATH") != "" {
		for _, name := range spec.pathNames {
			p, err := exec.LookPath(name)
			if err == nil && executable(p) {
				return p
			}
		}
	}
	dirs := spec.dirs
	if dirOverrideOn {
		dirs = dirOverride
	}
	for _, dir := range dirs {
		for _, file := range spec.files {
			p := filepath.Join(dir, file)
			if executable(p) {
				return p
			}
		}
	}
	return ""
}

func executable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	return fi.Mode()&0o111 != 0
}
