package secrets_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/secrets"
)

// The scanner (docs/backend-checklist.md B3.5). Every key here is a synthetic one: the AWS key is
// Amazon's own public documentation example, and the rest are invented for this test. No real
// credential is used (prompt rule 3).

const (
	awsExampleKey  = "AKIAQRSTUVWXYZ234567"
	fakeGitHubPAT  = "ghp_012345678901234567890123456789012345"
	fakePrivateKey = "-----BEGIN RSA PRIVATE KEY-----\n" +
		"MIIEowIBAAKCAQEA0123456789abcdefghijklmnopqrstuvwxyz\n" +
		"-----END RSA PRIVATE KEY-----\n"
)

func TestTheScannerFindsAKnownKeyAndNamesTheRule(t *testing.T) {
	scanner := secrets.New()
	findings, err := scanner.Scan("deploy.sh", "export AWS_ACCESS_KEY_ID="+awsExampleKey+"\n")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("the scanner found nothing in an AWS example key")
	}
	if findings[0].Rule != "aws-access-token" {
		t.Errorf("the rule that fired = %q, want aws-access-token", findings[0].Rule)
	}
	if findings[0].File != "deploy.sh" {
		t.Errorf("the file = %q, want deploy.sh", findings[0].File)
	}
}

func TestTheScannerFindsAPrivateKeyAndAGitHubToken(t *testing.T) {
	scanner := secrets.New()
	for _, tt := range []struct {
		name    string
		path    string
		content string
		rule    string
	}{
		{"a private key", "id_rsa", fakePrivateKey, "private-key"},
		{"a GitHub token", "config.go", `token := "` + fakeGitHubPAT + `"`, "github-pat"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			findings, err := scanner.Scan(tt.path, tt.content)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(findings) == 0 {
				t.Fatalf("the scanner found nothing")
			}
			if findings[0].Rule != tt.rule {
				t.Errorf("the rule that fired = %q, want %q", findings[0].Rule, tt.rule)
			}
		})
	}
}

// TestTheScannerNeverKeepsTheSecret is the property that matters most: whatever it finds, the
// secret itself is not in what it returns, so it cannot reach a log, an audit row, or a client.
func TestTheScannerNeverKeepsTheSecret(t *testing.T) {
	scanner := secrets.New()
	findings, err := scanner.Scan("deploy.sh", "AWS_ACCESS_KEY_ID="+awsExampleKey+"\n")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, f := range findings {
		held := f.Rule + " " + f.Description + " " + f.File
		if strings.Contains(held, awsExampleKey) {
			t.Fatalf("a finding holds the secret itself: %+v", f)
		}
	}
}

func TestTheScannerLeavesOrdinaryWorkAlone(t *testing.T) {
	scanner := secrets.New()
	ordinary := []struct {
		path    string
		content string
	}{
		{"main.go", "package main\n\nfunc main() { println(\"hello\") }\n"},
		{"config.yaml", "apiUrl: https://example.com\napiKey: \"\"\n"},
		{"README.md", "Set your API key in the environment before running.\n"},
	}
	for _, tt := range ordinary {
		findings, err := scanner.Scan(tt.path, tt.content)
		if err != nil {
			t.Fatalf("scan %s: %v", tt.path, err)
		}
		if len(findings) != 0 {
			t.Errorf("the scanner flagged ordinary text %q: %+v", tt.path, findings)
		}
	}
}

// TestTheScannerIsSafeForManyGoroutines covers the shared detector: the daemon scans a whole turn's
// commits from one goroutine today, but the scanner is built to be shared.
func TestTheScannerIsSafeForManyGoroutines(t *testing.T) {
	scanner := secrets.New()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			findings, err := scanner.Scan("deploy.sh", "AWS_ACCESS_KEY_ID="+awsExampleKey+"\n")
			if err != nil {
				t.Errorf("scan: %v", err)
				return
			}
			if len(findings) == 0 {
				t.Errorf("the scanner found nothing")
			}
		}()
	}
	wg.Wait()
}
