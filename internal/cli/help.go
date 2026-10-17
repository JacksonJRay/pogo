package cli

import "fmt"

func globalHelp() string {
	return render(commandDoc{
		name: "pogo",
		quick: `Quick start

1. Encrypt an existing .env file. The original .env stays where it is:
     pogo convert .env

2. Or create a new encrypted file and edit it in your editor:
     pogo init

3. Change a secret later:
     pogo edit

4. Start your app with those secrets loaded:
     pogo run -- npm start

When you create a file, Pogo asks for a passphrase of at least 15 characters and does not save it.
Opening an existing file accepts the passphrase that encrypted it, as long as it is not empty.
The encrypted file is named .env.enc unless you pass -f with another name.
`,
		purpose: `Pogo encrypts env files and loads them into a program at startup.
The encrypted file can sit on disk. The passphrase stays with you.
One static binary, no network access, no telemetry, no language SDK.`,
		synopsis: "pogo <command> [flags] [arguments]",
		args: `The first argument is a command: init, edit, run, encrypt, decrypt, convert, rekey, skill, help, version, docker, or doctor.
docker only prints the container guide. doctor only reports which passphrase prompt would be used.
Anything after -- is the program to run, and only for the run command.`,
		flags: commonFlags() + `
--version, -V
    Print the version and exit 0. Same as: pogo version
`,
		examples: `pogo convert .env
pogo edit
pogo run -- npm start
pogo help run
pogo skill`,
		workflows: `Convert, then run:
  pogo convert .env
  printf '%s' "$POGO_PASS" | pogo run --key-stdin -- npm start

Create, edit, run:
  pogo init
  pogo edit
  pogo run -- ./server

Use a name other than .env.enc:
  pogo convert .env -f .env.production.enc
  pogo run -f .env.production.enc -- ./server`,
		frameworks: `Any language works. Pogo starts your normal command and the variables are in the environment.
  pogo run -- npm start
  pogo run -- go run .
  pogo run -- python app.py
  pogo run -- npm run dev
Run "pogo skill" for per-framework notes (Next.js, Vite, SvelteKit, Django, Docker, and others).
Run "pogo skill --list" to see the framework names.
Run "pogo help docker" for containers: pogo as PID 1, the passphrase, and Compose.`,
		security: securityNotes(),
		exits:    exitNotes() + "\nRunning pogo with no command prints this quick start and exits 1. Run `pogo help` for the same page and exit code 0.\n",
	})
}

func helpFor(cmd string) (string, error) {
	docs := map[string]commandDoc{
		"init":    initHelp(),
		"edit":    editHelp(),
		"run":     runHelp(),
		"encrypt": encryptHelp(),
		"decrypt": decryptHelp(),
		"convert": convertHelp(),
		"rekey":   rekeyHelp(),
		"skill":   skillHelp(),
		"help":    helpHelp(),
		"version": versionHelp(),
		"docker":  dockerHelp(),
		"doctor":  doctorHelp(),
	}
	doc, ok := docs[cmd]
	if !ok {
		return "", unknownCommand(cmd)
	}
	return render(doc), nil
}

func initHelp() commandDoc {
	return commandDoc{
		name: "init",
		purpose: `Create a new encrypted env file and open it in your editor.
Pogo writes a short commented template, opens your editor, then encrypts whatever you save.
The editor is $EDITOR if that is set, otherwise $VISUAL, otherwise vim.
If the editor exits with an error, Pogo does not create the file.`,
		synopsis: "pogo init [file]",
		args: `file    Optional path of the encrypted file to create. Default: .env.enc
        Same as -f. If you pass both and they differ, Pogo stops.`,
		flags: commonFlags() + forceFlag(),
		examples: `pogo init
EDITOR=nano pogo init
EDITOR="code --wait" pogo init -f .env.production.enc`,
		workflows: `Which editor opens: $EDITOR, or $VISUAL if EDITOR is unset, or vim if both are unset.

  EDITOR=nano pogo init
  VISUAL=nano pogo init
  EDITOR="code --wait" pogo init
  EDITOR="vim -n" pogo edit

A graphical editor such as VS Code exits as soon as the window is open, unless you pass its wait flag. Pogo encrypts when that process exits, so the wait flag matters. For VS Code it is --wait. Pogo splits the editor value on spaces and adds the temp file as the last argument. It does not run a shell, so do not put pipes or quotes inside the value.

Inside vim, Ctrl-C cancels the current vim command. Quit vim to finish: :wq writes the text, :q! discards it. To abort Pogo, quit the editor or stop the pogo process. Pogo attempts to remove the temp file on a handled stop. SIGKILL, power loss, or editor-created copies can retain plaintext. Stopping init this way does not create the encrypted file.

init writes the template even if you do not change it, because that template is the new file. edit is different: if the text did not change, the encrypted file is left byte for byte as it was.

A script can fill the file without opening vim. Pogo runs the editor program and appends the temp file path as the last argument.

  cat > ./write-env.sh << 'EOF'
  #!/bin/sh
  cat > "$1" << 'ENV'
  API_KEY=replace-me
  ENV
  EOF
  chmod +x ./write-env.sh
  printf '%s' "$POGO_PASS" | EDITOR=./write-env.sh pogo init --key-stdin

Type the passphrase twice when Pogo asks. --key-env and --key-stdin are read once. Pogo will not read a passphrase from a file.`,
		frameworks: "init is the same for every language. The file you save is ordinary env text. See `pogo skill` for how each framework reads variables later.",
		security:   securityNotes() + "\nThe editor sees plaintext in a temp file. Pogo creates that file with mode 0600, in a directory of mode 0700, prefers /dev/shm when it exists, and overwrites it before deleting it. Cleanup covers normal exits and handled signals. SIGKILL, power loss, storage behavior, and editor-created copies can leave plaintext behind.\n",
		exits:      exitNotes(),
	}
}

func editHelp() commandDoc {
	return commandDoc{
		name: "edit",
		purpose: `Change an encrypted env file with your normal editor.
Pogo decrypts into a private temp file, opens your editor, encrypts the result, and deletes the temp file.
The editor is $EDITOR if that is set, otherwise $VISUAL, otherwise vim.
If the editor exits with an error, or you interrupt Pogo, the encrypted file is left unchanged.
If you quit without changing the text, the encrypted file is left unchanged.`,
		synopsis: "pogo edit [file]",
		args: `file    Optional encrypted file. Default: .env.enc
        Same as -f. If you pass both and they differ, Pogo stops.`,
		flags: commonFlags(),
		examples: `pogo edit
EDITOR=nano pogo edit
printf '%s' "$POGO_PASS" | EDITOR="vim -n" pogo edit --key-stdin`,
		workflows: `Which editor opens: $EDITOR, or $VISUAL if EDITOR is unset, or vim if both are unset.

  EDITOR=nano pogo edit
  EDITOR="code --wait" pogo edit
  EDITOR="vim -n" pogo edit

vim -n starts vim with an argument (no swap file). code --wait is the same shape: the program, its arguments, then the file Pogo adds. Use --wait for VS Code, Sublime, and other editors that return before you close the window. Without it, Pogo encrypts an empty or unfinished edit. Pogo does not run a shell.

Quit vim with :wq to save, or :q! to discard. Inside vim, Ctrl-C does not quit. If Pogo itself is stopped (Ctrl-C when Pogo is in the foreground, or SIGTERM), Pogo attempts to remove the temp file and leaves the encrypted file unchanged. SIGKILL and power loss bypass handled cleanup.

The temp file is mode 0600 in a private directory, on /dev/shm when that memory disk exists, and Pogo attempts to overwrite and remove it after a save, editor failure, or handled interrupt. A vimrc that writes backup files outside that directory can leave a copy; disable backup and persistent undo for this file, for example with EDITOR="vim -n -i NONE -u NONE". vim -n only disables swap files.

Non-interactive edit: set EDITOR to a program that rewrites the file it is given and exits 0.

  cat > ./write-env.sh << 'EOF'
  #!/bin/sh
  printf '\nEDITED=yes\n' >> "$1"
  EOF
  chmod +x ./write-env.sh
  printf '%s' "$POGO_PASS" | EDITOR=./write-env.sh pogo edit --key-stdin

"$1" is the temp file. Do not put shell redirections in EDITOR. Pogo does not run a shell.
EDITOR="code --wait" works because Pogo splits EDITOR on spaces into a program and arguments.`,
		frameworks: "After editing, start the app with `pogo run`. You do not need a different edit command per language.",
		security:   securityNotes() + "\nThe editor receives a restricted plaintext temp file. Pogo attempts to overwrite and remove it on normal exits and handled signals. SIGKILL, power loss, storage behavior, and editor-created copies can retain plaintext.\n",
		exits:      exitNotes(),
	}
}

func runHelp() commandDoc {
	return commandDoc{
		name: "run",
		purpose: `Decrypt the env file in memory and start a program with those variables set.
exec means Pogo replaces itself with your program. Your program keeps the same process id, so Ctrl-C, the terminal, and the exit code belong to your program, not to a wrapper.
No plaintext file is written.`,
		synopsis: "pogo run [file] -- <command> [args...]",
		args: `file       Optional encrypted file, before --. Default: .env.enc
command    Required. The program to start. It must come after --.
args       Arguments for that program. They are not read by Pogo.`,
		flags: commonFlags() + `
--env NAME
    Also load the [NAME] section of the file. Those keys replace top-level keys with the same name.
    Default: only the keys above the first [section] line. A file with no sections loads every key.

--no-override
    If a variable is already set in the environment, keep that value.
    Default: off. The encrypted file replaces variables that are already set, so the file is what the program sees.
`,
		examples: `pogo run -- npm start
pogo run -f .env.production.enc -- ./server
printf '%s' "$POGO_PASS" | pogo run --key-stdin -- npm start
pogo run --env production -- go run .
docker run -it --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE`,
		workflows: `Interactive, when you are at a terminal. You will see "Enter passphrase: ":
  pogo run -- npm start

Key from an environment variable. Pogo removes it from the child. If you pass that variable into Docker with -e, docker inspect still shows it:
  pogo run --key-env POGO_KEY -- ./server

Key on stdin. A pipe is read completely and trimmed, and the program sees end-of-file, so it cannot also read stdin. Do not read the passphrase from a file:
  printf '%s' "$POGO_PASS" | pogo run --key-stdin -- ./server
When stdin is a terminal, --key-stdin prints the passphrase prompt to stderr, turns echo off, and reads one line. That is the same as the interactive prompt. You do not press Ctrl-D. docker run -it with a --key-stdin entrypoint does this.

Docker, step by step (same text as pogo help docker):
` + dockerPassphraseGuide(),
		frameworks: `The program and every process it starts inherit the variables.
  pogo run -- npm start
  pogo run -- npm run dev
  pogo run -- go run .
  pogo run -- python app.py
  pogo run -- node build
Vite only exposes VITE_ names to browser code. Next.js only exposes NEXT_PUBLIC_ names to browser code. SvelteKit $env/dynamic/private sees the variables at runtime; $env/static/private is fixed at build time.
Docker does not copy the environment into a container by itself. The recipes above are the ways to pass the passphrase. Run "pogo help docker" for the same page.
Run "pogo skill" for the full per-framework guide.`,
		security: securityNotes() + `
Pogo looks up the command on PATH, the same way your shell does. Scripts with a shebang work: the operating system runs the interpreter.
--key-stdin consumes stdin. If the program needs to read stdin, type the passphrase at the prompt instead.
A terminal stays a terminal, which dev servers need.
`,
		exits: exitNotes() + "\nOnce the program has started, its own exit code is what you see. Exit 7 from the program is 7, not a Pogo code.\n",
	}
}

func encryptHelp() commandDoc {
	return commandDoc{
		name: "encrypt",
		purpose: `Encrypt a plaintext env file into a Pogo file.
The plaintext file is left in place. You delete it yourself after you have checked the encrypted file.
When Pogo asks for the passphrase, you type it twice. --key-env and --key-stdin are read once. Pogo will not read a passphrase from a file.`,
		synopsis: "pogo encrypt <plaintext> [file]",
		args: `plaintext    Required. The env file to read.
file         Optional encrypted output. Default: .env.enc
             Same as -f. If you pass both and they differ, Pogo stops.`,
		flags: commonFlags() + forceFlag(),
		examples: `pogo encrypt .env
pogo encrypt .env -f .env.production.enc
printf '%s' "$POGO_PASS" | pogo encrypt .env --key-stdin --force`,
		workflows: `Encrypt, check, then delete the plaintext yourself:
  printf '%s' "$POGO_PASS" | pogo encrypt .env --key-stdin
  printf '%s' "$POGO_PASS" | pogo run --key-stdin -- printenv API_KEY
  rm .env`,
		frameworks: "The output is the same for every language. Prefer `pogo convert` when the input is a project .env. It prints a reminder to delete the plaintext.",
		security:   securityNotes(),
		exits:      exitNotes(),
	}
}

func decryptHelp() commandDoc {
	return commandDoc{
		name: "decrypt",
		purpose: `Decrypt an env file and print the plaintext.
Printing secrets is easy to leak. Prefer "pogo run" so the values stay in memory.
Pogo always prints a warning. Without an output path, the plaintext goes to stdout and the warning goes to stderr.`,
		synopsis: "pogo decrypt [file] [outfile]",
		args: `file       Optional encrypted file. Default: .env.enc. Same as -f.
outfile    Optional plaintext destination. Default: print to the terminal.
           "-" also means the terminal.
           If -f is set and you pass one positional path, that path is the output file.`,
		flags: commonFlags() + `
--env NAME
    Print the merged view for that [section], not the raw file.
    Default: print the original file, including comments and blank lines.

` + forceFlag(),
		examples: `pogo decrypt
pogo decrypt -f .env.production.enc
pogo decrypt .env.enc ./check.env
pogo decrypt --env production`,
		workflows: `Inspect plaintext on the terminal only when necessary. Terminal output and logs can disclose secrets:
  pogo decrypt

Check a required variable without printing its value:
  pogo run -- sh -c 'test -n "$API_KEY" && printf "API_KEY is set\n"'

Write a file only when you must. Pogo refuses to overwrite an existing output unless you pass --force.`,
		frameworks: "decrypt is for inspection. Applications should use `pogo run` instead of reading a decrypted file.",
		security:   securityNotes() + "\nA decrypted file on disk is plaintext. Delete it when you are done. Pogo does not delete it for you.\n",
		exits:      exitNotes(),
	}
}

func convertHelp() commandDoc {
	return commandDoc{
		name: "convert",
		purpose: `Encrypt an existing plaintext .env into a Pogo file.
The original file is not deleted and not modified. After you have checked the encrypted file, delete the plaintext yourself.
You type the new passphrase twice, unless you pass --key-env or --key-stdin. Pogo will not read a passphrase from a file.`,
		synopsis: "pogo convert <plaintext.env> [file]",
		args: `plaintext.env    Required. The existing env file to read.
file             Optional encrypted output. Default: .env.enc
                 Same as -f. If you pass both and they differ, Pogo stops.`,
		flags: commonFlags() + forceFlag(),
		examples: `pogo convert .env
pogo convert .env -f .env.production.enc
printf '%s' "$POGO_PASS" | pogo convert .env --key-stdin`,
		workflows: `Convert, run the app, then delete the plaintext:
  pogo convert .env
  pogo run -- npm start
  rm .env

Custom output name:
  pogo convert .env -f .env.production.enc
  pogo run -f .env.production.enc -- ./server`,
		frameworks: "Works for any project's .env. The encrypted file is what `pogo run` reads. The app does not need a Pogo library.",
		security:   securityNotes() + "\nPogo will not remove the plaintext .env. Leaving it on disk keeps the secrets exposed. Delete it after you have verified the encrypted file.\n",
		exits:      exitNotes(),
	}
}

func rekeyHelp() commandDoc {
	return commandDoc{
		name: "rekey",
		purpose: `Change the passphrase on an encrypted file.
Pogo decrypts with the current passphrase and writes the file again with a new passphrase, a new salt, and a new nonce.
The old passphrase stops working.`,
		synopsis: "pogo rekey [file]",
		args: `file    Optional encrypted file. Default: .env.enc
        Same as -f. If you pass both and they differ, Pogo stops.`,
		flags: commonFlags() + `
--new-key-env NAME
    Read the new passphrase from environment variable NAME.
    Default: ask twice in the terminal.
    Pogo will not read the new passphrase from a file. --new-key-file was removed.
`,
		examples: `pogo rekey
pogo rekey -f .env.production.enc
pogo rekey --key-env POGO_OLD --new-key-env POGO_NEW`,
		workflows: `Interactive: Pogo asks for the current passphrase, then the new one, then asks you to type the new one again.

Non-interactive. Export the variables in the shell first. Do not put the values in the command, and do not write them to a file:
  pogo rekey --key-env POGO_OLD --new-key-env POGO_NEW

The new passphrase must be at least 15 characters unless you pass --allow-short-passphrase.
The current passphrase is accepted when it is not empty. --allow-short-passphrase applies only to the new passphrase. A blank new passphrase is still rejected.`,
		frameworks: "Rekey does not change how programs are started. Keep using `pogo run` with the new passphrase.",
		security:   securityNotes() + "\nDo not pass the new passphrase as a command-line argument, and do not put it in a file. Type it, or use --new-key-env.\n",
		exits:      exitNotes(),
	}
}

func skillHelp() commandDoc {
	return commandDoc{
		name: "skill",
		purpose: `Print the agent skill document: what Pogo is, how to supply a passphrase, and how to run each framework.
The document is plain text on stdout so an agent can read it. With no flags, the full document is printed.`,
		synopsis: "pogo skill [--list | --framework NAME]",
		args:     "No positional arguments.",
		flags: `
--list
    Print framework ids, one per line. Default: off.

--framework NAME
    Print only that framework's section. Run --list to see ids.
    Default: print the whole document.

-h, --help
    Show this help and exit 0.
`,
		examples: `pogo skill
pogo skill --list
pogo skill --framework sveltekit`,
		workflows: `An agent should read the skill before touching secrets:
  pogo skill
  pogo convert --help
  pogo run --help

Then show the user the exact command before running it.`,
		frameworks: "The framework sections are part of the skill. Add new ones in the skill source under a \"## framework <id>\" heading.",
		security:   securityNotes(),
		exits:      exitNotes(),
	}
}

func helpHelp() commandDoc {
	return commandDoc{
		name: "help",
		purpose: `Show help for Pogo or for one command.
"pogo help", "pogo --help", and "pogo -h" are the same page.
"pogo help run", "pogo run --help", and "pogo run -h" are the same page.`,
		synopsis: "pogo help [command]",
		args: `command    Optional command name: init, edit, run, encrypt, decrypt, convert, rekey, skill, help, version, docker, or doctor.
          With no command, Pogo prints the main help page.
          docker is a guide, not a container runtime. doctor does not ask for a passphrase.`,
		flags: `
-h, --help
    With no command name, show this page (help for the help command).
    "pogo help run --help" still shows help for run.
`,
		examples: `pogo help
pogo help run
pogo run --help
pogo help docker`,
		workflows: `Not sure which flags a command takes:
  pogo convert --help
  pogo run --help
  pogo skill`,
		frameworks: "Framework-specific run examples are in `pogo run --help` and in `pogo skill`.",
		security:   securityNotes(),
		exits:      exitNotes(),
	}
}

func versionHelp() commandDoc {
	return commandDoc{
		name:     "version",
		purpose:  `Print the Pogo version. The version line looks like "pogo 1.0.3".`,
		synopsis: "pogo version",
		args:     "No arguments.",
		flags: `
-h, --help
    Show this help instead of the version.

-V, --version
    Also prints the version when used as: pogo --version
`,
		examples: `pogo version
pogo --version
pogo version --help`,
		workflows:  "Check the version before filing a bug or reading an old script:\n  pogo version",
		frameworks: "The version does not change how env files work. v1 files store their own key-derivation settings in the file header.",
		security:   securityNotes(),
		exits:      exitNotes(),
	}
}

func doctorHelp() commandDoc {
	return commandDoc{
		name: "doctor",
		purpose: `Report which passphrase prompt Pogo would use, without asking for one.
Exit 0 when a terminal, POGO_ASKPASS, the editor prompt, or a desktop askpass is available.
Exit 1 when nothing can ask. The text names what was checked and tells an agent not to collect the passphrase in chat.
POGO_DEBUG=1 also prints the same decisions on stderr while a real command runs.`,
		synopsis: "pogo doctor",
		args:     "No arguments.",
		flags: `
-h, --help
    Show this help and exit 0.
`,
		examples: `pogo doctor
POGO_DEBUG=1 pogo doctor
pogo help doctor`,
		workflows: `An agent runs this before starting a program:

  pogo doctor

If it prints a backend and exits 0, run the program with no key flags and tell the person to watch for the prompt:

  pogo run -- npm start

If it exits 1, ask the person to run that command in their own terminal, or to pipe a secret manager into --key-stdin. Do not ask them to paste the passphrase into chat. --key-env is a last resort because the value stays visible in /proc and docker inspect.

A real terminal waits at "Enter passphrase:" until you type or press Ctrl-C. Set POGO_TTY_TIMEOUT to a number of seconds only when a terminal was allocated and nobody is there to type. Leave it unset for normal interactive use.`,
		frameworks: "doctor is the same for every language. It does not decrypt a file and it does not start a program.",
		security:   securityNotes() + "\ndoctor does not read, print, or store a passphrase. It does not print POGO_ASKPASS secrets or the editor auth token.\n",
		exits: `0    A prompt is available (this terminal, POGO_ASKPASS, the editor, or a desktop askpass).
1    No prompt is available, or the command was used wrong.
`,
	}
}

func dockerPassphraseGuide() string {
	return `How to pass the passphrase into Docker. Pogo will not read it from a file. Mount .env.enc read-only. Do not mount a passphrase file.

Which method should I pick?
    A person is at the keyboard            Interactive: docker run -it, or compose run, or compose up then attach
    A shell, once, at start                --key-stdin from read -rs, then unset the variable
    A password manager (op, pass)          --key-stdin from that program's stdout
    The passphrase is already in the env   --key-env (visible to docker inspect and /proc)
    Compose, a person                      docker compose run, or up with stdin_open and tty, then docker attach
    Compose, a pipe                        docker compose run -T with the --key-stdin entrypoint

Recipe 1. Interactive.
    When to use: a person is starting the container and can type.
    Security: the passphrase is typed, not stored, and docker inspect does not show it.
    ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--"]
    CMD ["node","server.js"]
    docker run -it --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
    You will see this prompt. Nothing is echoed as you type:
    Enter passphrase:
    Compose, attached terminal (same entrypoint, same prompt):
    docker compose run --rm app
    Compose up, then attach. The service needs stdin_open: true and tty: true.
    docker compose up -d
    docker attach CONTAINER
    Same prompt. Detach with Ctrl-P Ctrl-Q if the program should keep running. See examples/docker/compose.interactive.yaml.

Recipe 2. --key-stdin from a prompt.
    When to use: you want to type it once in the shell, and Docker must not store it.
    Security: the shell variable exists until you unset it. It is not in docker inspect. Stdin is consumed, so the program cannot also read stdin.
    ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--key-stdin","--"]
    CMD ["node","server.js"]
    read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE ; unset POGO_PASS
    If stdin is a terminal, this same --key-stdin entrypoint does not wait for Ctrl-D and does not echo the passphrase.
    docker run -it prints "Enter passphrase: " to stderr, turns echo off, and reads one line.
    That is the same as the interactive prompt. The program keeps the terminal.
    A pipe (docker run -i, or docker compose run -T) still reads all of stdin, trims the ends, and the program sees end-of-file.

Recipe 3. --key-stdin from a password manager.
    When to use: 1Password, pass, or another manager can print the passphrase.
    Security: the manager prints it into the pipe. Do not write that output to a file. Stdin is consumed. docker inspect does not show it.
    ENTRYPOINT is the same --key-stdin entrypoint as step 2.
    op read op://vault/pogo/passphrase | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
    pass show pogo | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE

Recipe 4. --key-env. Last resort. Prefer --key-stdin.
    When to use: POGO_KEY is already in the environment of the command that starts Docker, and you accept the tradeoff.
    Security: docker inspect shows POGO_KEY. It is also visible in /proc while the container runs. Pogo removes POGO_KEY from the program it execs, but Docker has already stored the value. Do not write the value in the command.
    ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--key-env","POGO_KEY","--"]
    CMD ["node","server.js"]
    docker run --rm -e POGO_KEY -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE

Recipe 5. Compose.
    When to use: you already have a compose file, including the shared-env demo.
    Security: same as the method you pick. Do not use a Compose secret file, and do not put the passphrase in the compose file.
    Interactive run uses the step 1 entrypoint. docker compose run allocates a terminal. Same prompt:
    docker compose run --rm app
    Interactive up uses stdin_open: true and tty: true, then:
    docker compose up -d
    docker attach CONTAINER
    A pipe uses the step 2 entrypoint. -T means no terminal, so the pipe is stdin:
    read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker compose run -T --rm app ; unset POGO_PASS
    op read op://vault/pogo/passphrase | docker compose run -T --rm app

--key-stdin on a terminal. docker run -it with a --key-stdin entrypoint prints the passphrase prompt, turns echo off, and reads one line. You do not press Ctrl-D. A pipe still uses docker run -i or docker compose run -T.

Troubleshooting. When stdin is not a terminal and no prompt on the machine can ask, Pogo exits 1. The first line is:

    pogo: no way to ask for the passphrase here

The next lines name what was checked (POGO_ASKPASS, the editor socket, display). Ways forward, in order: run it yourself in a terminal; docker run -it (or compose run, or compose up with stdin_open and tty, then docker attach); --key-stdin piped from a secret manager; POGO_ASKPASS; and --key-env as a last resort (visible in /proc and docker inspect). The last line tells an agent not to ask for the passphrase in chat. Run "pogo doctor" to see the same choice without starting the program.

A terminal waits at the passphrase prompt until you type or press Ctrl-C. POGO_TTY_TIMEOUT is an optional number of seconds. Leave it unset for normal interactive use.

systemd has no terminal. Pipe from a password manager, or use --key-env for that process only. --key-env is visible in /proc and in systemctl show. Do not point Pogo at a file.
    ExecStart=/bin/sh -c 'op read op://vault/pogo/passphrase | /usr/local/bin/pogo run -f /etc/myapp/.env.enc --key-stdin -- /usr/bin/myapp'

Pogo on the host, starting the docker client, is separate. The client does not copy every variable into the container. Forward each name with -e NAME and no value. sudo drops the environment unless you pass -E. Export POGO_KEY in the shell first; do not put the passphrase in the command.
    pogo run --key-env POGO_KEY -- docker run --rm -e API_KEY alpine printenv API_KEY
    pogo run --key-env POGO_KEY -- sudo -E docker run --rm -e API_KEY alpine printenv API_KEY
    pogo run --key-env POGO_KEY -- sudo -E docker compose -f examples/docker/compose.host.yaml up --abort-on-container-exit --exit-code-from passed
`
}

func dockerHelp() commandDoc {
	return commandDoc{
		name: "docker",
		purpose: `How to run Pogo in Docker and how to start Docker from Pogo.
This command only prints the guide. It does not talk to a Docker daemon.
Pogo can be the container entrypoint. exec replaces Pogo, so your program is PID 1. docker stop and the exit code belong to that program.
Pogo can also stay on the host and start the docker client. The client does not copy every variable into the container.
Pogo will not read the passphrase from a file.`,
		synopsis: "pogo help docker",
		args: `No arguments. "pogo docker", "pogo docker --help", and "pogo help docker" print this page.
To start a container from the host, the program after -- is docker, not this guide:
  pogo run -- docker run ...`,
		flags: `
-h, --help
    Show this page and exit 0.
`,
		examples: `docker run -it --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE ; unset POGO_PASS
op read op://vault/pogo/passphrase | docker run -i --rm -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
docker run --rm -e POGO_KEY -v "$PWD/.env.enc:/app/.env.enc:ro" IMAGE
read -rs POGO_PASS && printf '%s' "$POGO_PASS" | docker compose run -T --rm app ; unset POGO_PASS`,
		workflows: dockerPassphraseGuide() + `
Arguments you pass to docker run replace CMD. They are the program after --.
Images: examples/docker/alpine.Dockerfile (interactive entrypoint), scratch.Dockerfile, and distroless.Dockerfile. examples/docker/pogo.Dockerfile is the published-image shape (COPY --from). examples/docker/pogo-src.Dockerfile compiles Pogo, then copies the binary onto scratch.
examples/docker/compose.interactive.yaml is the tty attach layout. examples/docker/compose.stdin.yaml is the --key-stdin layout.
Do not use --init if you want your program to be PID 1.`,
		frameworks: `Any program that reads the environment works as CMD. A shell wrapper must exec the real program, or the shell is PID 1 and may swallow docker stop.
The shared demo (examples/shared-env-demo) starts Flask and SvelteKit on the host with pogo run, and Node in Docker from the same .env.enc. The container entrypoint is --key-stdin. The walkthrough pipes the passphrase; it does not use a key file.
Run "pogo skill --framework docker" for the same notes in the agent skill.`,
		security: `Do not store a passphrase in a file, a Compose secret, an image layer, or the compose file.
Interactive and --key-stdin keep it out of docker inspect.
--key-env with docker run -e POGO_KEY puts it in the container config. docker inspect and /proc show it. Pogo removes POGO_KEY from the program it starts, but Docker has already copied the value.
The encrypted file can be mounted read-only. That file is ciphertext, not the passphrase.
` + securityNotes(),
		exits: exitNotes() + "\nThis guide itself exits 0. Once a container's program has started, docker run exits with that program's code. Exit 42 from the program is 42 from docker run.\n",
	}
}

type commandDoc struct {
	name       string
	quick      string
	purpose    string
	synopsis   string
	args       string
	flags      string
	examples   string
	workflows  string
	frameworks string
	security   string
	exits      string
}

func render(d commandDoc) string {
	var b string
	if d.name == "pogo" {
		b += "pogo — encrypt env files and load them into a program\n\n"
	} else {
		b += "pogo " + d.name + "\n\n"
	}
	if d.quick != "" {
		b += d.quick
		if d.quick[len(d.quick)-1] != '\n' {
			b += "\n"
		}
		b += "\n"
	}
	b += "Purpose\n" + indentBlock(d.purpose) + "\n\n"
	b += "Synopsis\n    " + d.synopsis + "\n\n"
	b += "Arguments\n" + indentBlock(d.args) + "\n\n"
	b += "Flags\n" + stringsTrim(d.flags) + "\n\n"
	b += "Examples\n" + indentExamples(d.examples) + "\n\n"
	b += "Common workflows\n" + indentBlock(d.workflows) + "\n\n"
	b += "Framework notes\n" + indentBlock(d.frameworks) + "\n\n"
	b += "Security notes\n" + indentBlock(d.security) + "\n\n"
	b += "Exit codes\n" + indentBlock(d.exits)
	if d.exits == "" || d.exits[len(d.exits)-1] != '\n' {
		b += "\n"
	}
	return b
}

func indentBlock(s string) string {
	s = trimNewlines(s)
	if s == "" {
		return "    (none)\n"
	}
	lines := splitLines(s)
	out := ""
	for _, line := range lines {
		if line == "" {
			out += "\n"
			continue
		}
		out += "    " + line + "\n"
	}
	return out
}

func indentExamples(s string) string {
	s = trimNewlines(s)
	lines := splitLines(s)
	out := ""
	n := 0
	for _, line := range lines {
		if line == "" {
			out += "\n"
			continue
		}
		n++
		out += fmt.Sprintf("    %d. %s\n", n, line)
	}
	return out
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimNewlines(s string) string {
	for len(s) > 0 && (s[0] == '\n' || s[0] == ' ') && s[0] == '\n' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s
}

func stringsTrim(s string) string {
	return indentBlock(s)
}

func commonFlags() string {
	return `
-f, --file PATH
    Encrypted file. Default: .env.enc in the current directory.
    A positional path sets the same thing. If both are given and they differ, Pogo stops and tells you.
    On encrypt and convert, this flag is the file Pogo writes.

--key-env NAME
    Read the passphrase from the environment variable NAME. The value is used exactly, with no trimming.
    When you "pogo run", Pogo removes NAME from the program's environment so the passphrase is not passed on.
    The variable stays in this process's environment, so it is visible in /proc and in docker inspect.
    Prefer --key-stdin. For example: printf '%s' "$PASS" | pogo run --key-stdin -- cmd
    or: op read ... | pogo run --key-stdin -- cmd
    Use --key-env only as a last resort.
    Default: unset.

--key-stdin
    Read the passphrase from standard input.
    When stdin is a pipe, Pogo reads all of it, trims leading and trailing whitespace, and uses that as the passphrase.
    The program started by "pogo run" then sees end-of-file on stdin, so that program cannot also read stdin.
    When stdin is a terminal, Pogo prints the usual passphrase prompt to stderr, turns echo off, and reads one line.
    That is the same as the interactive prompt. docker run -it with a --key-stdin entrypoint does this.
    You do not press Ctrl-D. The program keeps the terminal.
    Default: off. If you also did not set --key-env, Pogo asks in the terminal when stdin is a terminal.
    When stdin is not a terminal, Pogo asks through a prompt already on the machine, in this order: POGO_ASKPASS, the editor Git prompt (VSCODE_GIT_IPC_HANDLE, used by code-server, VS Code, and Cursor), then a desktop askpass when WAYLAND_DISPLAY, DISPLAY, or MIR_SOCKET is set.
    An agent runs "pogo doctor", then "pogo run -- <command>" with no key flags, and does not collect the passphrase.
    If nothing can ask, Pogo exits 1. The first line is "pogo: no way to ask for the passphrase here".
    POGO_DEBUG=1 prints those detection steps on stderr. It never prints the passphrase.
    A real terminal waits until you type or press Ctrl-C. Set POGO_TTY_TIMEOUT to a number of seconds only when nobody is at that terminal.
    Pogo will not read a passphrase from a file. --key-file was removed.

--allow-short-passphrase
    When you create a passphrase (init, encrypt, convert, and the new passphrase for rekey), skip the 15-character minimum.
    A passphrase is still required. Empty or whitespace-only is rejected with exit code 4.
    One character that is not only whitespace is enough. The check runs before the slow key step.
    Pogo prints one warning line on standard error: short passphrases are much easier to guess.
    That warning is not written to standard output.
    Opening an existing file (run, decrypt, edit, and the current passphrase for rekey) does not use this minimum.
    Any passphrase that is not empty is tried. An empty passphrase is rejected with exit code 4.
    Passing this flag while opening is accepted and has no effect, and it does not print the warning.
    It works with the prompt, --key-stdin, and --key-env.
    Example: printf '%s' "$SHORT_PASS" | pogo convert .env --key-stdin --allow-short-passphrase
    Default: off.

-h, --help
    Show help for this command and exit 0.
    "pogo help <command>", "pogo <command> --help", and "pogo <command> -h" print the same text.
`
}

func forceFlag() string {
	return `
--force
    Replace an existing destination file. Default: off. Without --force, Pogo refuses to overwrite.
`
}

func securityNotes() string {
	return `A passphrase is a long password. Pogo never writes it to disk, and it will not read one from a file.
--key-file and --new-key-file were removed on purpose. A file that can decrypt the env file would be another secret stored on the system. Type the passphrase, or pipe it with --key-stdin. Prefer --key-stdin over --key-env. --key-env leaves the passphrase in the process environment, which is visible in /proc and docker inspect, so use it only as a last resort.
Pogo uses AES-256-GCM, which is the encryption, and Argon2id, which turns your passphrase into the key slowly so guessing is expensive. You do not configure either one. The slow step uses 128 MiB of memory, 4 passes, and 4 lanes. Each save gets a new random salt and a new random nonce (extra random bytes so two saves of the same text do not look alike).
The file header (including those parameters) is checked as part of decryption. A wrong passphrase or a flipped bit fails decryption (exit 2).
There is no network access and no telemetry.
Pogo tries to zero passphrase and key bytes after use. Go can still keep copies the program cannot reach; do not treat zeroing as a guarantee. See the README.
--allow-short-passphrase skips the 15-character minimum when you create a passphrase. Short passphrases are much easier to guess. Leave it off unless you mean to use a short passphrase. A blank passphrase is still rejected. Opening an existing file does not apply that minimum, and passing the flag then has no effect.
`
}

func exitNotes() string {
	return `0    Success. For "pogo run", once your program has started, this is your program's exit code.
1    Something else went wrong: usage, a missing file, a mismatched confirmation, or the program was not found.
2    Decryption failed. The passphrase was wrong, or the file is damaged.
3    The editor ran and exited non-zero. The encrypted file was not updated.
4    Creating a passphrase: shorter than 15 characters, or empty or only whitespace.
     --allow-short-passphrase skips the 15-character minimum and still rejects a blank passphrase.
     Opening an existing file: only an empty passphrase is rejected with this code. A wrong passphrase is exit 2.
`
}
