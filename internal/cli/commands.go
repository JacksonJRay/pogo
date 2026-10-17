package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"pogo/internal/dotenv"
	"pogo/internal/editfile"
	"pogo/internal/filefmt"
	"pogo/internal/passphrase"
	"pogo/internal/securemem"
	"pogo/internal/skill"
)

const initTemplate = `# pogo environment file
# Lines before the first [section] are loaded by every "pogo run".
# A [name] section is loaded only when you pass --env name.
# Keys in the section replace top-level keys with the same name.
#
# DATABASE_URL=postgres://localhost/dev
#
# [production]
# DATABASE_URL=postgres://prod.example/app
`

func dispatch(cmd string, opts options) (int, error) {
	switch cmd {
	case "init":
		return cmdInit(opts)
	case "edit":
		return cmdEdit(opts)
	case "run":
		return cmdRun(opts)
	case "encrypt":
		return cmdEncrypt(opts)
	case "decrypt":
		return cmdDecrypt(opts)
	case "convert":
		return cmdConvert(opts)
	case "rekey":
		return cmdRekey(opts)
	case "skill":
		return cmdSkill(opts)
	case "docker":
		return cmdDocker(opts)
	default:
		return 1, unknownCommand(cmd)
	}
}

func cmdDocker(opts options) (int, error) {
	if len(opts.positionals) > 0 || opts.hasDD {
		return 1, fmt.Errorf("pogo docker prints the Docker guide and does not start a container.\nRun `pogo help docker` or `pogo docker --help`.\nTo start a program, use `pogo run -- docker ...`.")
	}
	fmt.Fprint(os.Stdout, render(dockerHelp()))
	return 0, nil
}

func cmdInit(opts options) (int, error) {
	warnMin(opts)
	enc, err := oneEnc(opts.file, opts.fileSet, opts.positionals)
	if err != nil {
		return 1, err
	}
	ok, err := fileExists(enc)
	if err != nil {
		return 1, err
	}
	if ok && !opts.force {
		return 1, refuseOverwrite(enc)
	}
	pass, code, err := acquire(opts, true, enterPrompt(opts), confirmPrompt())
	if err != nil {
		return code, err
	}
	defer securemem.Wipe(pass)
	edited, err := editfile.Edit([]byte(initTemplate), editfile.EditorCommand())
	if err != nil {
		return explainEditor(err, false)
	}
	defer securemem.Wipe(edited)
	if err := writeEncrypted(enc, pass, edited); err != nil {
		return 1, err
	}
	fmt.Fprintf(os.Stderr, "Wrote %s\nRun a program with:\n  %s\n", enc, runExample(enc))
	return 0, nil
}

func cmdEdit(opts options) (int, error) {
	warnMin(opts)
	enc, err := oneEnc(opts.file, opts.fileSet, opts.positionals)
	if err != nil {
		return 1, err
	}
	ok, err := fileExists(enc)
	if err != nil {
		return 1, err
	}
	if !ok {
		return 1, missingEnc(enc)
	}
	pass, code, err := acquire(opts, false, enterPrompt(opts), "")
	if err != nil {
		return code, err
	}
	defer securemem.Wipe(pass)
	plain, err := decryptFile(enc, pass)
	if err != nil {
		return explainDecrypt(err)
	}
	defer securemem.Wipe(plain)
	edited, err := editfile.Edit(plain, editfile.EditorCommand())
	if err != nil {
		return explainEditor(err, true)
	}
	defer securemem.Wipe(edited)
	if bytes.Equal(plain, edited) {
		fmt.Fprintf(os.Stderr, "No changes. Left %s unchanged.\n", enc)
		return 0, nil
	}
	if err := writeEncrypted(enc, pass, edited); err != nil {
		return 1, err
	}
	fmt.Fprintf(os.Stderr, "Updated %s\n", enc)
	return 0, nil
}

func cmdRun(opts options) (int, error) {
	warnMin(opts)
	if !opts.hasDD || len(opts.runCmd) == 0 {
		if len(opts.positionals) > 0 && !opts.hasDD {
			return 1, fmt.Errorf("pogo run needs the program after --.\nExample: pogo run -- %s\nWithout --, those words are not a program. They would be read as an encrypted file name.", strings.Join(opts.positionals, " "))
		}
		return 1, fmt.Errorf("pogo run needs a program after --.\nExample: pogo run -- npm start")
	}
	enc, err := oneEnc(opts.file, opts.fileSet, opts.positionals)
	if err != nil {
		return 1, err
	}
	ok, err := fileExists(enc)
	if err != nil {
		return 1, err
	}
	if !ok {
		return 1, missingEnc(enc)
	}
	pass, code, err := acquire(opts, false, enterPrompt(opts), "")
	if err != nil {
		return code, err
	}
	defer securemem.Wipe(pass)
	plain, err := decryptFile(enc, pass)
	if err != nil {
		return explainDecrypt(err)
	}
	defer securemem.Wipe(plain)
	doc, err := dotenv.Parse(plain)
	if err != nil {
		return 1, fmt.Errorf("Could not parse the decrypted environment file: %s.\nEdit it with `pogo edit` and fix that line. Comments start with #. Sections look like [production]. Put spaces and newlines inside double quotes.", err.Error())
	}
	name := ""
	if opts.envSet {
		name = opts.envName
	}
	pairs, err := doc.Select(name)
	if err != nil {
		return 1, sectionError(doc, name)
	}
	drop := map[string]struct{}{}
	if opts.keyEnv != "" {
		drop[opts.keyEnv] = struct{}{}
	}
	envv := buildEnv(os.Environ(), pairs, drop, !opts.noOverride)
	argv := append([]string(nil), opts.runCmd...)
	resolved, err := exec.LookPath(argv[0])
	if err != nil {
		return 1, fmt.Errorf("Could not find command %q on PATH.\nInstall it, or pass a full path such as /usr/bin/npm.\nPogo looks up the command the same way your shell does.", argv[0])
	}
	// exec does not run defers. Wipe before the process image is replaced.
	securemem.Wipe(pass)
	securemem.Wipe(plain)
	if err := execCommand(resolved, argv, envv); err != nil {
		return 1, fmt.Errorf("Could not start %s: %s.\nCheck that the file is executable and that you are allowed to run it.", resolved, err.Error())
	}
	return 0, nil
}

func cmdEncrypt(opts options) (int, error) {
	return encryptLike(opts, false)
}

func cmdConvert(opts options) (int, error) {
	return encryptLike(opts, true)
}

func encryptLike(opts options, convert bool) (int, error) {
	warnMin(opts)
	usage := "pogo encrypt needs the plaintext file to read.\nExample: pogo encrypt .env\nExample with a custom output name: pogo encrypt .env -f secrets.enc"
	if convert {
		usage = "pogo convert needs the plaintext env file.\nExample: pogo convert .env\nExample with a custom output name: pogo convert .env -f .env.production.enc"
	}
	plainPath, enc, err := plainAndEnc(opts.file, opts.fileSet, opts.positionals, usage)
	if err != nil {
		return 1, err
	}
	ok, err := fileExists(plainPath)
	if err != nil {
		return 1, err
	}
	if !ok {
		return 1, fmt.Errorf("No plaintext file at %s.\nPass the path of the env file you want to encrypt.\nExample: pogo convert .env", plainPath)
	}
	ok, err = fileExists(enc)
	if err != nil {
		return 1, err
	}
	if ok && !opts.force {
		return 1, refuseOverwrite(enc)
	}
	data, err := readLimited(plainPath, filefmt.MaxPlaintext)
	if err != nil {
		return 1, err
	}
	defer securemem.Wipe(data)
	pass, code, err := acquire(opts, true, enterPrompt(opts), confirmPrompt())
	if err != nil {
		return code, err
	}
	defer securemem.Wipe(pass)
	if err := writeEncrypted(enc, pass, data); err != nil {
		return 1, err
	}
	if convert {
		fmt.Fprintf(os.Stderr, "Wrote %s\nLeft the plaintext file in place: %s\nAfter you confirm the encrypted file works, delete the plaintext yourself:\n  rm %s\n", enc, plainPath, quotePath(plainPath))
	} else {
		fmt.Fprintf(os.Stderr, "Wrote %s\nLeft the plaintext file in place: %s\nDelete it yourself after you have checked the encrypted file:\n  rm %s\n", enc, plainPath, quotePath(plainPath))
	}
	return 0, nil
}

func cmdDecrypt(opts options) (int, error) {
	warnMin(opts)
	enc, outfile, toStdout, err := decryptPaths(opts.file, opts.fileSet, opts.positionals)
	if err != nil {
		return 1, err
	}
	ok, err := fileExists(enc)
	if err != nil {
		return 1, err
	}
	if !ok {
		return 1, missingEnc(enc)
	}
	if !toStdout {
		ok, err = fileExists(outfile)
		if err != nil {
			return 1, err
		}
		if ok && !opts.force {
			return 1, refuseOverwrite(outfile)
		}
	}
	pass, code, err := acquire(opts, false, enterPrompt(opts), "")
	if err != nil {
		return code, err
	}
	defer securemem.Wipe(pass)
	plain, err := decryptFile(enc, pass)
	if err != nil {
		return explainDecrypt(err)
	}
	defer func() { securemem.Wipe(plain) }()
	if opts.envSet {
		doc, err := dotenv.Parse(plain)
		if err != nil {
			return 1, fmt.Errorf("Could not parse the decrypted environment file: %s.\nEdit it with `pogo edit` and fix that line.", err.Error())
		}
		pairs, err := doc.Select(opts.envName)
		if err != nil {
			return 1, sectionError(doc, opts.envName)
		}
		formatted := dotenv.Format(pairs)
		old := plain
		plain = formatted
		securemem.Wipe(old)
	}
	if toStdout {
		fmt.Fprintf(os.Stderr, "Warning: printing decrypted secrets as plaintext.\nPrefer `pogo run` so the secrets stay in memory and are not shown on screen.\n")
		_, err = os.Stdout.Write(plain)
		if err != nil {
			return 1, err
		}
		return 0, nil
	}
	fmt.Fprintf(os.Stderr, "Warning: writing decrypted secrets to %s as plaintext.\nDelete that file when you are done. Prefer `pogo run` so secrets stay in memory.\n", outfile)
	if err := writeAtomic(outfile, plain); err != nil {
		return 1, err
	}
	return 0, nil
}

func cmdRekey(opts options) (int, error) {
	warnMin(opts)
	enc, err := oneEnc(opts.file, opts.fileSet, opts.positionals)
	if err != nil {
		return 1, err
	}
	ok, err := fileExists(enc)
	if err != nil {
		return 1, err
	}
	if !ok {
		return 1, missingEnc(enc)
	}
	oldPass, code, err := acquire(opts, false, "Enter the current passphrase: ", "")
	if err != nil {
		return code, err
	}
	defer securemem.Wipe(oldPass)
	plain, err := decryptFile(enc, oldPass)
	if err != nil {
		return explainDecrypt(err)
	}
	defer securemem.Wipe(plain)
	newOpts := opts
	newOpts.keyStdin = false
	newOpts.keyEnv = ""
	if opts.newKeyEnv != "" {
		newOpts.keyEnv = opts.newKeyEnv
	}
	newPass, code, err := acquire(newOpts, true, newPassPrompt(newOpts), newConfirmPrompt())
	if err != nil {
		return code, err
	}
	defer securemem.Wipe(newPass)
	if err := writeEncrypted(enc, newPass, plain); err != nil {
		return 1, err
	}
	fmt.Fprintf(os.Stderr, "Updated the passphrase for %s\nThe previous passphrase will no longer open this file.\n", enc)
	return 0, nil
}

func cmdSkill(opts options) (int, error) {
	if opts.list && opts.framework != "" {
		return 1, fmt.Errorf("Use either `pogo skill --list` or `pogo skill --framework <name>`, not both.")
	}
	if opts.list {
		for _, id := range skill.IDs() {
			fmt.Fprintln(os.Stdout, id)
		}
		return 0, nil
	}
	if opts.framework != "" {
		body, ok := skill.FrameworkByID(opts.framework)
		if !ok {
			msg := fmt.Sprintf("Unknown framework %q.", opts.framework)
			if s := suggest(opts.framework, skill.IDs()); s != "" {
				msg += "\nDid you mean " + s + "?"
			}
			msg += "\nRun `pogo skill --list` to see framework names."
			return 1, fmt.Errorf("%s", msg)
		}
		if _, err := os.Stdout.WriteString(body); err != nil {
			return 1, err
		}
		return 0, nil
	}
	if _, err := os.Stdout.Write(skill.Document); err != nil {
		return 1, err
	}
	return 0, nil
}

func acquire(opts options, confirm bool, prompt, confirmPrompt string) ([]byte, int, error) {
	src := passphrase.Source{
		Env:    opts.keyEnv,
		Stdin:  opts.keyStdin,
		Prompt: prompt,
	}
	var (
		pass []byte
		err  error
	)
	if confirm {
		pass, err = passphrase.AcquireConfirmed(src, prompt, confirmPrompt)
	} else {
		pass, err = passphrase.Acquire(src)
	}
	if err != nil {
		if errors.Is(err, passphrase.ErrMismatch) {
			return nil, 1, fmt.Errorf("The two passphrases did not match.\nRun the command again and enter the same passphrase both times.")
		}
		if strings.Contains(err.Error(), "not a terminal") {
			return nil, 1, fmt.Errorf("%s.\nIf this is Docker, add -it (docker run -it), or use docker compose run, or docker compose up with stdin_open: true and tty: true and then docker attach.\nOr pass the passphrase with --key-stdin (docker run -i, or docker compose run -T; the program cannot also read stdin) or --key-env NAME.\nPogo does not read a passphrase from a file.", err.Error())
		}
		return nil, 1, err
	}
	min := effectiveMin(opts)
	if relaxedLength(opts) && passphrase.IsBlank(pass) {
		n := passphrase.Length(pass)
		securemem.Wipe(pass)
		return nil, 4, fmt.Errorf("Passphrase is empty or only whitespace (%d characters). A passphrase is still required.\n--allow-short-passphrase and --min-length can skip the 15-character minimum. They do not accept a blank passphrase.", n)
	}
	if err := passphrase.CheckLength(pass, min); err != nil {
		n := passphrase.Length(pass)
		securemem.Wipe(pass)
		if errors.Is(err, passphrase.ErrTooShort) {
			return nil, 4, fmt.Errorf("Passphrase is too short (%d characters). Pogo requires at least %d.\nPick a longer passphrase and try again. To set a different minimum on purpose, pass --min-length N (the lowest allowed value is 1), or pass --allow-short-passphrase to skip the 15-character minimum.", n, min)
		}
		if errors.Is(err, passphrase.ErrTooLong) {
			return nil, 1, fmt.Errorf("Passphrase is longer than %d bytes.\nUse a shorter passphrase.", passphrase.MaxBytes)
		}
		return nil, 1, err
	}
	return pass, 0, nil
}

func effectiveMin(opts options) int {
	// --min-length already accepts a floor as low as 1. The boolean flag
	// is that same floor when you do not pick a number. An explicit
	// --min-length still wins, including a number of 15 or more.
	if opts.allowShort && !opts.minSet {
		return passphrase.AbsoluteMin
	}
	return opts.minLength
}

func relaxedLength(opts options) bool {
	if opts.allowShort {
		return true
	}
	return opts.minSet && opts.minLength < passphrase.DefaultMin
}

func enterPrompt(opts options) string {
	if opts.allowShort && !opts.minSet {
		return "Enter passphrase: "
	}
	return fmt.Sprintf("Enter passphrase (min %d characters): ", effectiveMin(opts))
}

func confirmPrompt() string {
	return "Enter it again to confirm: "
}

func newPassPrompt(opts options) string {
	if opts.allowShort && !opts.minSet {
		return "Enter a new passphrase: "
	}
	return fmt.Sprintf("Enter a new passphrase (min %d characters): ", effectiveMin(opts))
}

func newConfirmPrompt() string {
	return "Enter the new passphrase again to confirm: "
}

func warnMin(opts options) {
	if opts.allowShort {
		fmt.Fprintf(os.Stderr, "warning: short passphrases are much easier to guess.\n")
	}
	if opts.minSet && opts.minLength < passphrase.DefaultMin {
		fmt.Fprintf(os.Stderr, "warning: --min-length is %d, below the recommended %d. Shorter passphrases are easier to guess.\n", opts.minLength, passphrase.DefaultMin)
	}
}

func decryptFile(path string, pass []byte) ([]byte, error) {
	blob, err := readLimited(path, maxCipher)
	if err != nil {
		return nil, err
	}
	defer securemem.Wipe(blob)
	return filefmt.Open(pass, blob)
}

func writeEncrypted(path string, pass, plain []byte) error {
	blob, err := filefmt.Seal(pass, plain)
	if err != nil {
		return err
	}
	defer securemem.Wipe(blob)
	if err := writeAtomic(path, blob); err != nil {
		return fmt.Errorf("Could not write %s: %s.\nCreate the folder if it is missing, then run the command again.", path, err.Error())
	}
	return nil
}

func explainDecrypt(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	if os.IsNotExist(err) {
		return 1, err
	}
	var fe *filefmt.Error
	if errors.As(err, &fe) {
		switch fe.Kind {
		case filefmt.KindAuth:
			return 2, fmt.Errorf("Decryption failed. The passphrase was wrong, or the file was changed since it was encrypted.\nTry the passphrase again. If you ran `pogo rekey`, use the new passphrase.")
		case filefmt.KindFormat:
			return 2, fmt.Errorf("This file is not a usable Pogo encrypted env file. It may be damaged, or it was not created by Pogo.\nCreate one with `pogo init`, or encrypt a plaintext file with `pogo convert .env`.")
		case filefmt.KindParams:
			return 2, fmt.Errorf("This file asks for key-derivation settings Pogo will not run (%s).\nIt was not created by this version of Pogo, or the file is damaged.", fe.Msg)
		}
	}
	if errors.Is(err, filefmt.ErrDecrypt) {
		return 2, fmt.Errorf("Decryption failed. The passphrase was wrong, or the file was changed since it was encrypted.\nTry the passphrase again. If you ran `pogo rekey`, use the new passphrase.")
	}
	return 1, err
}

func explainEditor(err error, updating bool) (int, error) {
	switch {
	case errors.Is(err, editfile.ErrInterrupted):
		if updating {
			return 1, fmt.Errorf("Stopped before saving. The temporary plaintext file was deleted and the encrypted file was not changed.\nRun `pogo edit` again when you want to change it.")
		}
		return 1, fmt.Errorf("Stopped before saving. The temporary plaintext file was deleted and no encrypted file was created.\nRun `pogo init` again when you want to create it.")
	case errors.Is(err, editfile.ErrEditorMissing):
		return 1, fmt.Errorf("Could not start the editor: %s.\nSet EDITOR to a program on your PATH.\nExample: EDITOR=nano pogo edit\nIn a script, set EDITOR to a program that writes the temp file and exits 0. Pogo adds the temp path as the last argument and does not use a shell.", err.Error())
	case errors.Is(err, editfile.ErrEditor):
		if updating {
			return 3, fmt.Errorf("The editor exited with an error, so Pogo did not update the encrypted file.\nThe previous encrypted file was left unchanged. Fix the editor problem and run the command again.")
		}
		return 3, fmt.Errorf("The editor exited with an error, so Pogo did not create the encrypted file.\nFix the editor problem and run `pogo init` again.")
	default:
		return 1, err
	}
}

func sectionError(doc *dotenv.Document, name string) error {
	names := doc.Names()
	if len(names) == 0 {
		return fmt.Errorf("No environment section named %q, and this file has no [section] headers.\nRemove --env to load the top-level variables, or add a [%s] section with `pogo edit`.", name, name)
	}
	return fmt.Errorf("No environment section named %q.\nSections in this file: %s.\nUse one of those names with --env, or omit --env to load only the top-level variables.", name, strings.Join(names, ", "))
}

func runExample(enc string) string {
	if enc == defaultEnc {
		return "pogo run -- <command>"
	}
	return "pogo run -f " + quotePath(enc) + " -- <command>"
}

func quotePath(p string) string {
	if p == "" {
		return "''"
	}
	if strings.ContainsAny(p, " \t\n'\"\\$`") {
		return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
	}
	return p
}
