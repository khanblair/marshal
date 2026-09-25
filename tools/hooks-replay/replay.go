package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// recording is one webhook saved from a real delivery: the headers it came with, and its body
// exactly as it was sent. The body is kept as raw JSON, so its bytes are not changed and a
// signature that was computed over them still matches.
type recording struct {
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

// safeName allows only plain names, so a provider or recording name can never point outside
// the recordings folder.
var safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var errBadName = errors.New("a provider or recording name may only use lower case letters, digits, and dashes")

// loadRecording reads <dir>/<provider>/<name>.json.
func loadRecording(dir, provider, name string) (recording, error) {
	if !safeName.MatchString(provider) || !safeName.MatchString(name) {
		return recording{}, errBadName
	}
	path := filepath.Join(dir, provider, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return recording{}, fmt.Errorf("read the recording %s: %w", path, err)
	}
	var rec recording
	if err := json.Unmarshal(data, &rec); err != nil {
		return recording{}, fmt.Errorf("read the recording %s: %w", path, err)
	}
	if len(rec.Body) == 0 {
		return recording{}, fmt.Errorf("the recording %s has no body", path)
	}
	return rec, nil
}

// target is where a webhook goes: a daemon's address and the provider whose route it uses.
type target struct {
	baseURL  string
	provider string
}

// send posts a recording to <baseURL>/hooks/<provider> and returns the daemon's status line.
func send(ctx context.Context, client *http.Client, to target, rec recording) (string, error) {
	url := strings.TrimRight(to.baseURL, "/") + "/hooks/" + to.provider
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rec.Body))
	if err != nil {
		return "", fmt.Errorf("build the request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range rec.Headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send the webhook to %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp.Status, fmt.Errorf("the daemon answered %s", resp.Status)
	}
	return resp.Status, nil
}
