package main

// This file is `marshal keys`: the command-line way to put a model provider's key in the OS
// keychain, list which providers have one, and remove one (docs/backend-checklist.md section 3,
// "Keys go into the keychain with the `marshal keys` command"). It writes exactly where the daemon
// reads: the same keychain service name, platform.AppName(mode), so `marshal keys set` on this
// machine sets up the daemon that runs on it.
//
// The key travels on standard input, never as an argument, so it does not land in the shell history
// or in another process's view of this command's arguments. Nothing here is stored in a file.

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/security"
)

const keysUsage = "Usage: marshal keys <set|list|remove> [provider] [--dev]"

// quietLogger is handed to the provider service so a keychain hiccup it reports through a log line
// cannot print a timestamped line in the middle of this command's own answer.
func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// keychainFor opens the real OS keychain for a mode, filed under the same name the daemon files its
// secrets under (platform.AppName), so the dev daemon and a normal install never share keys.
func keychainFor(mode platform.Mode) security.Keychain {
	return security.NewOSKeychain(platform.AppName(mode))
}

// runKeys dispatches the three keys subcommands. newKeychain is injected so every path here is
// tested against an in-memory keychain, and no test ever reads or writes the machine's real one.
func runKeys(env platform.Env, args []string, term terminal, newKeychain func(platform.Mode) security.Keychain) int {
	if len(args) == 0 {
		say(term.stderr, "%s", keysUsage)
		return exitBadInput
	}
	switch args[0] {
	case "set":
		return runKeysSet(env, args[1:], term, newKeychain)
	case "list":
		return runKeysList(env, args[1:], term, newKeychain)
	case "remove":
		return runKeysRemove(env, args[1:], term, newKeychain)
	default:
		say(term.stderr, "Unknown keys command %q.\n\n%s", args[0], keysUsage)
		return exitBadInput
	}
}

// parseKeysFlags parses the one flag every keys subcommand takes, and returns the arguments left
// after it. A non-zero code means the flags were wrong and the command is already answered.
func parseKeysFlags(name string, args []string, term terminal) (rest []string, dev bool, code int) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(term.stderr)
	devFlag := fs.Bool("dev", false, "use the dev daemon's own keychain entries, which are separate from a normal install's")
	if err := fs.Parse(args); err != nil {
		return nil, false, exitBadInput
	}
	return fs.Args(), *devFlag, exitOK
}

// keysService loads the settings for the mode, opens the keychain for that mode, and returns a
// provider service over it. The service is only ever asked to save, list, or remove a key, so it is
// built without a recorder or a notifier. A nil service means the command is already answered and
// the returned code is what to exit with.
func keysService(env platform.Env, dev bool, term terminal, newKeychain func(platform.Mode) security.Keychain) (*providers.Service, int) {
	var settingsArgs []string
	if dev {
		settingsArgs = []string{devFlag}
	}
	settings, err := config.Load(settingsArgs, env, term.stderr)
	if err != nil {
		say(term.stderr, "%v", err)
		return nil, exitFailed
	}
	svc, err := providers.New(newKeychain(settings.Mode), providers.Options{Logger: quietLogger()})
	if err != nil {
		say(term.stderr, "%s", keysFailure(err))
		return nil, exitFailed
	}
	return svc, exitOK
}

// keysProvider reads the one provider id a set or remove names. When Marshal has no adapter for it,
// it says so and lists the ids it does know, rather than filing a key under a name nothing reads.
func keysProvider(rest []string, term terminal) (providers.Info, int) {
	if len(rest) != 1 {
		say(term.stderr, "%s", keysUsage)
		return providers.Info{}, exitBadInput
	}
	info, ok := providers.Lookup(rest[0])
	if !ok {
		say(term.stderr, "%s", unknownProviderLine(rest[0]))
		return providers.Info{}, exitBadInput
	}
	return info, exitOK
}

// unknownProviderLine names the unknown id and every id Marshal does know.
func unknownProviderLine(id string) string {
	known := providers.Known()
	ids := make([]string, 0, len(known))
	for _, info := range known {
		ids = append(ids, info.ID)
	}
	return fmt.Sprintf("Marshal does not know a provider called %q. It knows: %s.", id, strings.Join(ids, ", "))
}

// readKey reads one line from standard input: the key, or the server address for a provider that
// runs on this machine. The trailing newline is dropped, so both a pasted line and a piped one work.
func readKey(term terminal, info providers.Info) (string, error) {
	if info.Local {
		say(term.stderr, "Paste the address of the %s server, such as %s, then press Enter:", info.Name, info.BaseURL)
	} else {
		say(term.stderr, "Paste the %s API key, then press Enter:", info.Name)
	}
	line, err := bufio.NewReader(term.stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read the key: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// keysFailure turns an error from the provider service into the line a person reads. A protocol
// error already carries the sentence the app shows - the same words the settings form shows - so it
// is printed as it is; anything else is a failure on this machine and is reported plainly.
func keysFailure(err error) string {
	var perr *protocol.Error
	if errors.As(err, &perr) {
		return perr.Message
	}
	return "Marshal could not reach the keychain: " + trimTrailingPeriod(err) + "."
}

// runKeysSet stores (or replaces) the key for one provider.
func runKeysSet(env platform.Env, args []string, term terminal, newKeychain func(platform.Mode) security.Keychain) int {
	rest, dev, code := parseKeysFlags("keys set", args, term)
	if code != exitOK {
		return code
	}
	info, code := keysProvider(rest, term)
	if code != exitOK {
		return code
	}
	svc, code := keysService(env, dev, term, newKeychain)
	if svc == nil {
		return code
	}
	secret, err := readKey(term, info)
	if err != nil {
		say(term.stderr, "%s", keysFailure(err))
		return exitFailed
	}
	if err := svc.Save(info.ID, secret); err != nil {
		say(term.stderr, "%s", keysFailure(err))
		return exitFailed
	}
	say(term.stdout, "Saved the %s key.", info.Name)
	return exitOK
}

// runKeysList prints one line per provider Marshal knows: its id and either the masked key that is
// stored or "not set". The key itself is never printed: the masked form is all that leaves the
// daemon (docs/backend-inventory.md N18), and this command keeps to that rule.
func runKeysList(env platform.Env, args []string, term terminal, newKeychain func(platform.Mode) security.Keychain) int {
	rest, dev, code := parseKeysFlags("keys list", args, term)
	if code != exitOK {
		return code
	}
	if len(rest) != 0 {
		say(term.stderr, "%s", keysUsage)
		return exitBadInput
	}
	svc, code := keysService(env, dev, term, newKeychain)
	if svc == nil {
		return code
	}
	rows, err := svc.List()
	if err != nil {
		say(term.stderr, "%s", keysFailure(err))
		return exitFailed
	}
	for _, row := range rows {
		value := "not set"
		if row.Status == protocol.ProviderStatusSaved {
			value = row.Masked
		}
		say(term.stdout, "%-11s %s", row.ID, value)
	}
	return exitOK
}

// runKeysRemove deletes the value stored for one provider. Removing a provider that has none is not
// an error: it says there was nothing to remove and exits 0, the same way the route treats it.
func runKeysRemove(env platform.Env, args []string, term terminal, newKeychain func(platform.Mode) security.Keychain) int {
	rest, dev, code := parseKeysFlags("keys remove", args, term)
	if code != exitOK {
		return code
	}
	info, code := keysProvider(rest, term)
	if code != exitOK {
		return code
	}
	svc, code := keysService(env, dev, term, newKeychain)
	if svc == nil {
		return code
	}
	switch err := svc.Remove(info.ID); {
	case errors.Is(err, security.ErrNoKey):
		say(term.stdout, "There is no %s key stored.", info.Name)
		return exitOK
	case err != nil:
		say(term.stderr, "%s", keysFailure(err))
		return exitFailed
	}
	say(term.stdout, "Removed the %s key.", info.Name)
	return exitOK
}
