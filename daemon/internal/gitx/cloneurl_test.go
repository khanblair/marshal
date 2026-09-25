package gitx_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

func TestParseCloneURLAcceptsAddressesFromTheInternet(t *testing.T) {
	tests := []struct {
		raw  string
		want gitx.CloneURL
	}{
		{"https://github.com/acme/web.git", gitx.CloneURL{
			Scheme: "https", Host: "github.com", Name: "web", Redacted: "https://github.com/acme/web.git"}},
		{"https://github.com/acme/web", gitx.CloneURL{
			Scheme: "https", Host: "github.com", Name: "web", Redacted: "https://github.com/acme/web"}},
		{"HTTPS://GitHub.com/acme/web.git", gitx.CloneURL{
			Scheme: "https", Host: "GitHub.com", Name: "web", Redacted: "HTTPS://GitHub.com/acme/web.git"}},
		{"https://gitlab.example.com:8443/group/sub/proj.git/", gitx.CloneURL{
			Scheme: "https", Host: "gitlab.example.com", Name: "proj", Redacted: "https://gitlab.example.com:8443/group/sub/proj.git/"}},
		{"https://someone:s3cr3t@github.com/acme/web.git", gitx.CloneURL{
			Scheme: "https", Host: "github.com", Name: "web", HasCredentials: true, Redacted: "https://github.com/acme/web.git"}},
		{"https://ghp_token@github.com/acme/web.git", gitx.CloneURL{
			Scheme: "https", Host: "github.com", Name: "web", HasCredentials: true, Redacted: "https://github.com/acme/web.git"}},
		{"ssh://git@github.com/acme/web.git", gitx.CloneURL{
			Scheme: "ssh", Host: "github.com", Name: "web", Redacted: "ssh://git@github.com/acme/web.git"}},
		{"ssh://git:pw@example.com:2222/acme/web.git", gitx.CloneURL{
			Scheme: "ssh", Host: "example.com", Name: "web", HasCredentials: true, Redacted: "ssh://git@example.com:2222/acme/web.git"}},
		{"git@github.com:acme/web.git", gitx.CloneURL{
			Scheme: "scp", Host: "github.com", Name: "web", Redacted: "git@github.com:acme/web.git"}},
		{"deploy_user.1@git.example.com:team/repo", gitx.CloneURL{
			Scheme: "scp", Host: "git.example.com", Name: "repo", Redacted: "deploy_user.1@git.example.com:team/repo"}},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := gitx.ParseCloneURL(tc.raw, false)
			tc.want.Raw = tc.raw
			if err != nil || got != tc.want {
				t.Errorf("ParseCloneURL(%q) = %+v, %v\nwant %+v", tc.raw, got, err, tc.want)
			}
		})
	}
}

func TestParseCloneURLRefusesUnsafeAddresses(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", 2100)
	bad := []string{
		"", " ", "\n", long,
		// Transport helpers run programs.
		"ext::sh -c touch% /tmp/pwned", "ext::https://example.com/x", "fd::17", "EXT::x", "foo::bar",
		// Anything that starts with a dash is read as an option.
		"--upload-pack=touch /tmp/pwned", "--upload-pack=x", "-oProxyCommand=x", "-", "-c", "--",
		// Other schemes and local addresses.
		"file:///tmp/repo", "FILE:///tmp/repo", "git://github.com/acme/web.git", "http://github.com/acme/web.git",
		"ftp://example.com/x.git", "svn+ssh://example.com/x", "javascript://x", "/tmp/repo", "./repo", "../repo",
		"C:\\repo", "C:/repo", "\\\\server\\share\\repo", "repo", "~/repo",
		// Space, control characters, and backslashes.
		" https://github.com/acme/web.git", "https://github.com/acme/web.git ", "https://github.com/acme/web.git\n",
		"https://git hub.com/acme/web", "https://github.com/acme/web repo", "https://github.com/acme/\tweb",
		"https://github.com/acme/web\x00", "https://github.com/a\x1bcme/web", "https://exa\nmple.com/x",
		"https://user:pa ss@example.com/x", "https://example.com\\@evil.com/x", "https://example.com/a\\b",
		"git@github.com:acme/web repo", "git@github.com:acme/we\nb", "git @github.com:acme/web",
		// No host, or a host that is not a plain name.
		"https://", "https:///acme/web", "https://:8080/acme/web", "ssh://", "ssh:///path", "https://user@/acme/web",
		"https://-host.example.com/x", "ssh://-oProxyCommand=touch/x", "ssh://-oProxyCommand=x@host/y",
		"ssh://git@-oProxyCommand=touch/x", "git@-oProxyCommand=touch:x", "git@:path", "@host:path",
		"https://exämple.com/x", "https://[::1]/x", "https://ex ample.com/x", "https://ex%41mple.com/x",
		"https://host_name.example.com/x",
		// Nothing to clone.
		"https://github.com", "https://github.com/", "ssh://git@github.com", "git@github.com", "git@github.com:",
		"github.com:acme/web.git", "github.com/acme/web",
		// Odd forms.
		"://github.com/x", "https:github.com/x", "https//github.com/x",
	}
	for _, raw := range bad {
		name := raw
		if len(name) > 40 {
			name = name[:40]
		}
		t.Run(name, func(t *testing.T) {
			got, err := gitx.ParseCloneURL(raw, false)
			if !errors.Is(err, gitx.ErrBadURL) {
				t.Fatalf("ParseCloneURL(%q) = %+v, %v; want ErrBadURL", raw, got, err)
			}
			if got != (gitx.CloneURL{}) {
				t.Errorf("a refused address still returned %+v", got)
			}
			assertPlainMessage(t, err)
			if strings.Contains(err.Error(), "s3cr3t") || strings.Contains(err.Error(), "pw@") {
				t.Errorf("the error shows a credential: %v", err)
			}
		})
	}
}

func TestParseCloneURLLocalAddressesNeedAllowLocal(t *testing.T) {
	good := map[string]string{
		"/tmp/repo":                    "repo",
		"/tmp/my repo/project.git":     "project",
		"./repo":                       "repo",
		"../work/repo/":                "repo",
		"file:///tmp/repo":             "repo",
		"file:///tmp/my repo":          "my repo",
		"C:\\Users\\me\\my repo":       "my repo",
		"C:/Users/me/repo.git":         "repo",
		"\\\\server\\share\\repo":      "repo",
		"/tmp/repo\u00a0name":          "repo\u00a0name",
		"/tmp/git@host:not-an-address": "git@host:not-an-address",
	}
	for raw, name := range good {
		got, err := gitx.ParseCloneURL(raw, true)
		if err != nil || got.Scheme != "local" || got.Name != name || got.Host != "" || got.Raw != raw {
			t.Errorf("ParseCloneURL(%q, true) = %+v, %v; want a local address named %q", raw, got, err, name)
		}
		if _, err := gitx.ParseCloneURL(raw, false); !errors.Is(err, gitx.ErrBadURL) {
			t.Errorf("ParseCloneURL(%q, false) = %v, want ErrBadURL", raw, err)
		}
	}
	// AllowLocal opens local folders and nothing else.
	still := []string{
		"", "-x", "--upload-pack=x", "ext::sh -c x", "git://x/y", "http://x/y", "ftp://x/y", "repo", "~/repo",
		"/tmp/re\npo", "/tmp/re\x00po", "git@-oProxyCommand=x:path", "https://exa mple.com/x", "https://host/x\n",
	}
	for _, raw := range still {
		if _, err := gitx.ParseCloneURL(raw, true); !errors.Is(err, gitx.ErrBadURL) {
			t.Errorf("ParseCloneURL(%q, true) = %v, want ErrBadURL", raw, err)
		}
	}
}

func TestStripCredentials(t *testing.T) {
	tests := map[string]string{
		"https://user:pass@example.com/acme/web.git": "https://example.com/acme/web.git",
		"https://token@example.com/acme/web.git":     "https://example.com/acme/web.git",
		"https://user:p@ss@example.com/x":            "https://example.com/x",
		"https://user:pass@example.com:8443/x?a=b@c": "https://example.com:8443/x?a=b@c",
		"https://example.com/acme@web.git":           "https://example.com/acme@web.git",
		"https://example.com/x":                      "https://example.com/x",
		"ssh://git@example.com/x":                    "ssh://git@example.com/x",
		"ssh://git:pw@example.com/x":                 "ssh://git@example.com/x",
		"ssh://example.com/x":                        "ssh://example.com/x",
		"git@github.com:acme/web.git":                "git@github.com:acme/web.git",
		"/tmp/repo":                                  "/tmp/repo",
		"":                                           "",
		"https://user:pass@":                         "https://",
		"git://user:pass@example.com/x":              "git://example.com/x",
	}
	for raw, want := range tests {
		if got := gitx.StripCredentials(raw); got != want {
			t.Errorf("StripCredentials(%q) = %q, want %q", raw, got, want)
		}
	}
}
