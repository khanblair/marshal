package gitx

import (
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// maxURLLength keeps a pasted address from being a whole document.
const maxURLLength = 2048

// CloneURL is an address that passed ParseCloneURL.
type CloneURL struct {
	// Raw is the address as it was given. It can hold a token, so it is never logged or shown.
	Raw string
	// Redacted is the address with any user name and password removed. Only this is shown.
	Redacted string
	// Scheme is "https", "ssh", or "scp" for git@host:path, and "local" for a folder or a
	// file:// address, which need AllowLocal.
	Scheme string
	// Host is the server name, and is empty for a local address.
	Host string
	// Name is the repository's own name, from the end of the address, without ".git". It is empty
	// when the address has none.
	Name string
	// HasCredentials is true when the address holds a password or a token.
	HasCredentials bool
}

var (
	transportHelper = regexp.MustCompile(`^[A-Za-z0-9+._-]+::`)
	scpAddress      = regexp.MustCompile(`^([A-Za-z0-9._-]+)@([A-Za-z0-9][A-Za-z0-9.-]*):(.+)$`)
	driveAddress    = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
	hostName        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)
	userInfoInText  = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s@]+@`)
)

// ParseCloneURL checks an address that came from a user or a chat before it goes to Git. Only
// https://, ssh://, and git@host:path addresses pass. A folder or a file:// address passes only
// when allowLocal is true, which tests and dev mode use. Everything else is ErrBadURL: transport
// helpers such as ext::, git:// and http://, anything that starts with a dash, and anything with
// a space or control character in it.
func ParseCloneURL(raw string, allowLocal bool) (CloneURL, error) {
	if err := checkURLText(raw); err != nil {
		return CloneURL{}, err
	}
	var (
		u   CloneURL
		err error
	)
	switch {
	case strings.Contains(raw, "://"):
		u, err = parseSchemeURL(raw, allowLocal)
	case scpAddress.MatchString(raw):
		u, err = parseSCP(raw)
	case allowLocal && looksLikePath(raw):
		u = CloneURL{Scheme: "local", Name: repoName(raw)}
	default:
		err = badURL("use an https:// or ssh:// address, or one like git@host:owner/name.git")
	}
	if err != nil {
		return CloneURL{}, err
	}
	u.Raw = raw
	if u.Redacted == "" {
		u.Redacted = StripCredentials(raw)
	}
	return u, nil
}

func badURL(detail string) error { return newOpError(ErrBadURL, detail, nil) }

// checkURLText rejects what no address may hold, whatever its form.
func checkURLText(raw string) error {
	switch {
	case raw == "":
		return badURL("the address is empty")
	case len(raw) > maxURLLength:
		return badURL("the address is too long")
	case strings.HasPrefix(raw, "-"):
		return badURL("the address must not start with a dash")
	case transportHelper.MatchString(raw):
		return badURL("transport helpers are not allowed")
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return badURL("the address has a control character in it")
		}
	}
	return nil
}

// parseSchemeURL reads an address that has a scheme, like https://host/path.
func parseSchemeURL(raw string, allowLocal bool) (CloneURL, error) {
	scheme, _, _ := strings.Cut(raw, "://")
	scheme = strings.ToLower(scheme)
	if scheme == "file" {
		if !allowLocal {
			return CloneURL{}, badURL("local addresses are not allowed")
		}
		return CloneURL{Scheme: "local", Name: repoName(raw)}, nil
	}
	if scheme != "https" && scheme != "ssh" {
		return CloneURL{}, badURL("only https:// and ssh:// addresses can be cloned")
	}
	if err := checkRemoteText(raw); err != nil {
		return CloneURL{}, err
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return CloneURL{}, badURL("the address cannot be read")
	}
	host := parsed.Hostname()
	if err := checkHost(host); err != nil {
		return CloneURL{}, err
	}
	// Git hands ssh the user and the host as one word, so a user that starts with a dash has the
	// same effect as a host that does.
	if parsed.User != nil && strings.HasPrefix(parsed.User.Username(), "-") {
		return CloneURL{}, badURL("the user name must not start with a dash")
	}
	if strings.Trim(parsed.Path, "/") == "" {
		return CloneURL{}, badURL("the address has no repository in it")
	}
	return CloneURL{
		Scheme:         scheme,
		Host:           host,
		Name:           repoName(parsed.Path),
		HasCredentials: hasSecret(parsed, scheme),
	}, nil
}

// hasSecret reports whether a parsed address carries something worth hiding. A user name in an
// ssh:// address, like git@, is not a secret. In an https:// address it can be a token.
func hasSecret(parsed *url.URL, scheme string) bool {
	if parsed.User == nil {
		return false
	}
	if scheme == "ssh" {
		_, has := parsed.User.Password()
		return has
	}
	return true
}

// parseSCP reads user@host:path.
func parseSCP(raw string) (CloneURL, error) {
	m := scpAddress.FindStringSubmatch(raw)
	if err := checkRemoteText(raw); err != nil {
		return CloneURL{}, err
	}
	if err := checkHost(m[2]); err != nil {
		return CloneURL{}, err
	}
	return CloneURL{Scheme: "scp", Host: m[2], Name: repoName(m[3])}, nil
}

// checkRemoteText rejects what a server address may not hold. A folder may have a space in its
// name, but a server address never does, and a backslash can make two programs read the same
// address as two different servers.
func checkRemoteText(raw string) error {
	if strings.ContainsFunc(raw, unicode.IsSpace) || strings.Contains(raw, "\\") {
		return badURL("the address must not have spaces or backslashes in it")
	}
	return nil
}

// looksLikePath reports whether text names a folder on this machine: a full path, or one that
// starts with ./ or ../. Anything else, such as a host name that is not a valid server address, is
// left for Git to guess about, and Marshal would rather refuse.
func looksLikePath(text string) bool {
	return filepath.IsAbs(text) || driveAddress.MatchString(text) || strings.HasPrefix(text, `\\`) ||
		strings.HasPrefix(text, "./") || strings.HasPrefix(text, "../") ||
		strings.HasPrefix(text, `.\`) || strings.HasPrefix(text, `..\`)
}

// checkHost rejects a server name that could be read as an option, or is not a plain name.
func checkHost(host string) error {
	switch {
	case host == "":
		return badURL("the address has no server name")
	case strings.HasPrefix(host, "-"):
		return badURL("the server name must not start with a dash")
	case !hostName.MatchString(host):
		return badURL("the server name has characters that are not allowed")
	}
	return nil
}

// repoName is the last part of an address without ".git", or "" when there is none.
func repoName(address string) string {
	name := path.Base(strings.ReplaceAll(strings.TrimRight(address, "/\\"), "\\", "/"))
	name = strings.TrimSuffix(name, ".git")
	if name == "." || name == "/" || name == ".." {
		return ""
	}
	return name
}

// StripCredentials removes the user name and password from an address, so it is safe to show and
// to store. An https:// address loses everything before the @, because a token is often sent as
// the user name. An ssh:// address keeps its user name, like git@, and loses only a password.
// Text that is not a scheme address, such as git@host:path or a folder, is returned as it is.
func StripCredentials(raw string) string {
	scheme, rest, found := strings.Cut(raw, "://")
	if !found {
		return raw
	}
	end := strings.IndexAny(rest, "/?#")
	if end < 0 {
		end = len(rest)
	}
	authority, tail := rest[:end], rest[end:]
	at := strings.LastIndex(authority, "@")
	if at < 0 {
		return raw
	}
	userInfo, host := authority[:at], authority[at+1:]
	if strings.EqualFold(scheme, "ssh") {
		name, _, hasPassword := strings.Cut(userInfo, ":")
		if !hasPassword {
			return raw
		}
		return scheme + "://" + name + "@" + host + tail
	}
	return scheme + "://" + host + tail
}

// redactText removes credentials from text that may quote an address, such as a Git error. It
// first replaces each known address with its redacted form, and then strips the user part of
// any other address it finds.
func redactText(text string, known ...CloneURL) string {
	for _, u := range known {
		if u.Raw != "" && u.Raw != u.Redacted {
			text = strings.ReplaceAll(text, u.Raw, u.Redacted)
		}
	}
	return userInfoInText.ReplaceAllString(text, "$1")
}
