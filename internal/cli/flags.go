package cli

import (
	"fmt"
	"strings"
)

const defaultEnc = ".env.enc"

var commandNames = []string{
	"init", "edit", "run", "encrypt", "decrypt", "convert", "rekey", "skill", "help", "version", "upgrade", "docker", "doctor",
}

// valueFlags take an argument. Bool flags do not.
var valueFlags = map[string]bool{
	"-f": true, "--file": true,
	"--key-env":     true,
	"--env":         true,
	"--new-key-env": true,
	"--framework":   true,
}

var boolFlags = map[string]bool{
	"--key-stdin": true, "--force": true, "--no-override": true, "--list": true,
	"--allow-short-passphrase": true,
	"--check":                  true, "--yes": true,
	"--delete-original": true, "--keep-original": true,
	"-h": true, "--help": true, "-V": true, "--version": true,
}

// flags a command accepts, besides the universal help flags.
var commandFlags = map[string][]string{
	"init":    {"-f", "--file", "--key-env", "--key-stdin", "--allow-short-passphrase", "--force"},
	"edit":    {"-f", "--file", "--key-env", "--key-stdin", "--allow-short-passphrase"},
	"run":     {"-f", "--file", "--key-env", "--key-stdin", "--allow-short-passphrase", "--env", "--no-override"},
	"encrypt": {"-f", "--file", "--key-env", "--key-stdin", "--allow-short-passphrase", "--force"},
	"decrypt": {"-f", "--file", "--key-env", "--key-stdin", "--allow-short-passphrase", "--env", "--force"},
	"convert": {"-f", "--file", "--key-env", "--key-stdin", "--allow-short-passphrase", "--force", "--delete-original", "--keep-original"},
	"rekey":   {"-f", "--file", "--key-env", "--key-stdin", "--allow-short-passphrase", "--new-key-env"},
	"skill":   {"--list", "--framework"},
	"help":    {},
	"version": {},
	"upgrade": {"--check", "--yes"},
	"docker":  {},
	"doctor":  {},
}

type options struct {
	file           string
	fileSet        bool
	keyEnv         string
	keyStdin       bool
	allowShort     bool
	envName        string
	envSet         bool
	force          bool
	noOverride     bool
	deleteOriginal bool
	keepOriginal   bool
	newKeyEnv      string
	framework      string
	list           bool
	help           bool
	version        bool
	upgradeCheck   bool
	upgradeYes     bool
	pinVersion     string
	positionals    []string
	runCmd         []string
	hasDD          bool
}

type parsed struct {
	cmd      string
	opts     options
	showHelp bool
	helpText string
	showVer  bool
}

func parseArgs(args []string) (parsed, error) {
	pre, cmd, rest, err := splitCommand(args)
	if err != nil {
		return parsed{}, err
	}
	if cmd == "" {
		opts, _, _, err := parseFlags(pre, "")
		if err != nil {
			return parsed{}, err
		}
		if opts.help {
			return parsed{showHelp: true, helpText: globalHelp()}, nil
		}
		if opts.version {
			return parsed{showVer: true}, nil
		}
		return parsed{}, fmt.Errorf("No command given.\nRun `pogo --help` for the quick start, or `pogo help` for the same page.")
	}
	if !isCommand(cmd) {
		return parsed{}, unknownCommand(cmd)
	}
	combined := append(append([]string{}, pre...), rest...)
	opts, runCmd, hasDD, err := parseFlags(combined, cmd)
	if err != nil {
		return parsed{}, err
	}
	p := parsed{cmd: cmd, opts: opts}
	if cmd == "help" {
		switch {
		case len(opts.positionals) > 1:
			return parsed{}, fmt.Errorf("pogo help takes one command name.\nExample: pogo help run")
		case len(opts.positionals) == 1:
			topic := opts.positionals[0]
			text, err := helpFor(topic)
			if err != nil {
				return parsed{}, err
			}
			p.showHelp = true
			p.helpText = text
			return p, nil
		case opts.help:
			p.showHelp = true
			p.helpText = mustHelp("help")
			return p, nil
		default:
			p.showHelp = true
			p.helpText = globalHelp()
			return p, nil
		}
	}
	if opts.help {
		p.showHelp = true
		p.helpText = mustHelp(cmd)
		return p, nil
	}
	if opts.version {
		if cmd == "upgrade" {
			return parsed{}, fmt.Errorf("-V prints the running version only as `pogo -V` or `pogo --version` with no command.\nTo install a release, use `pogo upgrade --version vX.Y.Z`.")
		}
		return parsed{}, fmt.Errorf("--version is not a flag for %s.\nRun `pogo version` or `pogo --version`.", cmd)
	}
	opts.runCmd = runCmd
	opts.hasDD = hasDD
	if err := validateOptions(cmd, opts); err != nil {
		return parsed{}, err
	}
	p.opts = opts
	return p, nil
}

func mustHelp(cmd string) string {
	text, err := helpFor(cmd)
	if err != nil {
		return globalHelp()
	}
	return text
}

func removedPassphraseFlag(name string) error {
	switch name {
	case "--key-file":
		return fmt.Errorf("--key-file was removed on purpose.\nA file that can decrypt the encrypted env would be another secret stored on the system. Pogo will not read one.\nType the passphrase when asked, or pipe it with --key-stdin.\nExample: printf '%%s' \"$POGO_PASS\" | pogo run --key-stdin -- npm start")
	case "--new-key-file":
		return fmt.Errorf("--new-key-file was removed on purpose.\nPogo will not read a passphrase from a file.\nType the new passphrase when asked, or pass --new-key-env NAME.\nExample: pogo rekey --new-key-env POGO_NEW")
	default:
		return nil
	}
}

func isCommand(name string) bool {
	for _, c := range commandNames {
		if c == name {
			return true
		}
	}
	return false
}

func unknownCommand(cmd string) error {
	msg := fmt.Sprintf("Unknown command %q.", cmd)
	if s := suggest(cmd, commandNames); s != "" {
		msg += "\nDid you mean " + s + "?"
	}
	msg += "\nRun `pogo --help` to see all commands."
	return fmt.Errorf("%s", msg)
}

func splitCommand(args []string) (pre []string, cmd string, rest []string, err error) {
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			return nil, "", nil, fmt.Errorf("Missing command before --.\nExample: pogo run -- npm start\nRun `pogo --help` to see the commands.")
		}
		if isFlagToken(a) {
			name, _, hasVal := splitEq(a)
			if err := removedPassphraseFlag(name); err != nil {
				return nil, "", nil, err
			}
			canon, ok := canonicalFlag(name)
			if !ok {
				return nil, "", nil, unknownFlag(name, "")
			}
			pre = append(pre, a)
			i++
			if !hasVal && valueFlags[canon] {
				if i >= len(args) {
					return nil, "", nil, missingFlagValue(canon)
				}
				pre = append(pre, args[i])
				i++
			}
			continue
		}
		return pre, a, args[i+1:], nil
	}
	return pre, "", nil, nil
}

func parseFlags(args []string, cmd string) (options, []string, bool, error) {
	var opts options
	var runCmd []string
	hasDD := false
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			hasDD = true
			runCmd = append([]string{}, args[i+1:]...)
			break
		}
		if !isFlagToken(a) {
			opts.positionals = append(opts.positionals, a)
			continue
		}
		name, inline, hasInline := splitEq(a)
		if err := removedPassphraseFlag(name); err != nil {
			return options{}, nil, false, err
		}
		canon, ok := canonicalFlag(name)
		if !ok {
			return options{}, nil, false, unknownFlag(name, cmd)
		}
		if cmd != "" && !flagAllowed(cmd, canon) {
			return options{}, nil, false, flagNotForCommand(canon, cmd)
		}
		if seen[canon] || (canon == "-f" && seen["--file"]) || (canon == "--file" && seen["-f"]) {
			return options{}, nil, false, fmt.Errorf("You passed %s more than once.\nUse it once. Run `pogo %s --help` for the flags this command accepts.", canon, orGlobal(cmd))
		}
		seen[canon] = true
		takeValue := valueFlags[canon]
		if cmd == "upgrade" && canon == "--version" {
			takeValue = true
		}
		var val string
		if takeValue {
			if hasInline {
				val = inline
			} else {
				i++
				if i >= len(args) {
					return options{}, nil, false, missingFlagValue(canon)
				}
				val = args[i]
			}
			if val == "" {
				return options{}, nil, false, fmt.Errorf("%s needs a value.\nExample: %s .env.enc", canon, exampleFor(canon))
			}
		}
		switch canon {
		case "-f", "--file":
			opts.file = val
			opts.fileSet = true
		case "--key-env":
			opts.keyEnv = val
		case "--key-stdin":
			opts.keyStdin = true
		case "--allow-short-passphrase":
			opts.allowShort = true
		case "--env":
			opts.envName = val
			opts.envSet = true
		case "--force":
			opts.force = true
		case "--delete-original":
			opts.deleteOriginal = true
		case "--keep-original":
			opts.keepOriginal = true
		case "--no-override":
			opts.noOverride = true
		case "--new-key-env":
			opts.newKeyEnv = val
		case "--framework":
			opts.framework = val
		case "--list":
			opts.list = true
		case "--check":
			opts.upgradeCheck = true
		case "--yes":
			opts.upgradeYes = true
		case "-h", "--help":
			opts.help = true
		case "--version":
			if cmd == "upgrade" {
				opts.pinVersion = val
			} else {
				opts.version = true
			}
		case "-V":
			opts.version = true
		}
	}
	return opts, runCmd, hasDD, nil
}

func validateOptions(cmd string, opts options) error {
	n := 0
	if opts.keyEnv != "" {
		n++
	}
	if opts.keyStdin {
		n++
	}
	if n > 1 {
		return fmt.Errorf("Use only one of --key-env and --key-stdin.\nYou passed more than one. Type the passphrase instead, or pick one of those two.\nRun `pogo %s --help` for examples.", cmd)
	}
	if cmd == "skill" {
		if opts.list && opts.framework != "" {
			return fmt.Errorf("Use either `pogo skill --list` or `pogo skill --framework <name>`, not both.")
		}
	}
	if cmd == "convert" && opts.deleteOriginal && opts.keepOriginal {
		return fmt.Errorf("Use only one of --delete-original and --keep-original.\n--delete-original removes the plaintext after the encrypted file is written and checked.\n--keep-original leaves the plaintext file and does not ask.")
	}
	return nil
}

func orGlobal(cmd string) string {
	if cmd == "" {
		return ""
	}
	return cmd
}

func exampleFor(flag string) string {
	switch flag {
	case "-f", "--file":
		return "--file"
	case "--key-env":
		return "--key-env"
	case "--env":
		return "--env"
	case "--new-key-env":
		return "--new-key-env"
	case "--framework":
		return "--framework"
	case "--version":
		return "--version v1.2.3"
	default:
		return flag
	}
}

func isFlagToken(a string) bool {
	return strings.HasPrefix(a, "-") && a != "-"
}

func splitEq(a string) (name, val string, has bool) {
	if strings.HasPrefix(a, "--") {
		if i := strings.IndexByte(a, '='); i >= 0 {
			return a[:i], a[i+1:], true
		}
		return a, "", false
	}
	if strings.HasPrefix(a, "-f=") {
		return "-f", a[3:], true
	}
	if strings.HasPrefix(a, "-f") && len(a) > 2 {
		return "-f", a[2:], true
	}
	return a, "", false
}

func canonicalFlag(name string) (string, bool) {
	if valueFlags[name] || boolFlags[name] {
		return name, true
	}
	return "", false
}

func flagAllowed(cmd, flag string) bool {
	if flag == "-h" || flag == "--help" || flag == "-V" || flag == "--version" {
		return true
	}
	for _, f := range commandFlags[cmd] {
		if f == flag {
			return true
		}
	}
	// -f and --file are aliases
	if flag == "-f" {
		return flagAllowed(cmd, "--file")
	}
	return false
}

func unknownFlag(name, cmd string) error {
	all := allFlagNames()
	msg := fmt.Sprintf("Unknown flag %q.", name)
	if s := suggest(name, all); s != "" {
		msg += "\nDid you mean " + s + "?"
	}
	if cmd != "" {
		msg += fmt.Sprintf("\nRun `pogo %s --help` to see the flags for %s.", cmd, cmd)
	} else {
		msg += "\nRun `pogo --help` to see the flags."
	}
	return fmt.Errorf("%s", msg)
}

func flagNotForCommand(flag, cmd string) error {
	owners := commandsWithFlag(flag)
	msg := fmt.Sprintf("%s is not a flag for %s.", flag, cmd)
	if len(owners) > 0 {
		msg += "\nIt is valid for: " + strings.Join(owners, ", ") + "."
	}
	if s := suggest(flag, commandFlags[cmd]); s != "" {
		msg += "\nDid you mean " + s + "?"
	}
	msg += fmt.Sprintf("\nRun `pogo %s --help` to see the flags for %s.", cmd, cmd)
	return fmt.Errorf("%s", msg)
}

func missingFlagValue(flag string) error {
	return fmt.Errorf("%s needs a value after it.\nExample: %s .env.enc\nRun `pogo --help` if you are not sure which flags take a value.", flag, exampleFor(flag))
}

func allFlagNames() []string {
	var out []string
	for k := range valueFlags {
		out = append(out, k)
	}
	for k := range boolFlags {
		out = append(out, k)
	}
	return out
}

func commandsWithFlag(flag string) []string {
	var out []string
	for _, cmd := range commandNames {
		if flagAllowed(cmd, flag) && cmd != "help" && cmd != "version" {
			// help/version only have help and version flags; skip those for "valid for"
			if flag == "-h" || flag == "--help" || flag == "-V" || flag == "--version" {
				continue
			}
			out = append(out, cmd)
		}
	}
	return out
}

func oneEnc(flag string, flagSet bool, pos []string) (string, error) {
	if len(pos) > 1 {
		return "", fmt.Errorf("Unexpected extra argument %q.\nThis command takes at most one encrypted-file path.\nThe program and its arguments, if any, go after --.\nExample: pogo run -- npm start", pos[1])
	}
	if len(pos) == 1 {
		if flagSet && pos[0] != flag {
			return "", conflict(flag, pos[0])
		}
		if flagSet {
			return flag, nil
		}
		return pos[0], nil
	}
	if flagSet {
		return flag, nil
	}
	return defaultEnc, nil
}

func plainAndEnc(flag string, flagSet bool, pos []string, usage string) (string, string, error) {
	if len(pos) < 1 {
		return "", "", fmt.Errorf("%s", usage)
	}
	if len(pos) > 2 {
		return "", "", fmt.Errorf("Unexpected extra argument %q.\n%s", pos[2], usage)
	}
	plain := pos[0]
	var enc string
	switch {
	case len(pos) == 2 && flagSet && flag != pos[1]:
		return "", "", conflict(flag, pos[1])
	case len(pos) == 2 && flagSet:
		enc = flag
	case len(pos) == 2:
		enc = pos[1]
	case flagSet:
		enc = flag
	default:
		enc = defaultEnc
	}
	if plain == enc {
		return "", "", fmt.Errorf("The plaintext path and the encrypted path are the same (%s).\nPass a different -f name. Example: pogo convert .env -f .env.enc", plain)
	}
	return plain, enc, nil
}

func decryptPaths(flag string, flagSet bool, pos []string) (enc, outfile string, toStdout bool, err error) {
	if len(pos) > 2 {
		return "", "", false, fmt.Errorf("Unexpected extra argument %q.\nUsage: pogo decrypt [file] [outfile]\nExample: pogo decrypt -f .env.enc ./secrets.env", pos[2])
	}
	if flagSet {
		enc = flag
		switch len(pos) {
		case 0:
			toStdout = true
		case 1:
			outfile = pos[0]
		case 2:
			if pos[0] != flag {
				return "", "", false, conflict(flag, pos[0])
			}
			outfile = pos[1]
		}
	} else {
		switch len(pos) {
		case 0:
			enc = defaultEnc
			toStdout = true
		case 1:
			enc = pos[0]
			toStdout = true
		case 2:
			enc = pos[0]
			outfile = pos[1]
		}
	}
	if outfile == "-" {
		toStdout = true
		outfile = ""
	}
	if !toStdout && outfile == enc {
		return "", "", false, fmt.Errorf("Refusing to write decrypted plaintext onto the encrypted file %s.\nPick a different output path, or omit it to print to the terminal.", enc)
	}
	return enc, outfile, toStdout, nil
}

func conflict(flag, positional string) error {
	return fmt.Errorf("You gave two different encrypted-file paths: -f says %q and the positional argument says %q.\nUse only one of them.\nExample: pogo edit -f .env.enc", flag, positional)
}
