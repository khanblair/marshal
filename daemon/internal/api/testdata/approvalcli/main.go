// Command approvalcli is a small subprocess fixture that speaks the Claude CLI wire protocol.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	emit := func(value any) { _ = json.NewEncoder(os.Stdout).Encode(value) }
	id := ""
	for _, arg := range os.Args {
		if value, ok := strings.CutPrefix(arg, "--session-id="); ok {
			id = value
		}
	}
	emit(map[string]any{"type": "system", "subtype": "init", "session_id": id})
	cwd, _ := os.Getwd()
	path := filepath.Join(cwd, "approved.txt")
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		var message struct {
			Type    string `json:"type"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Response struct {
				RequestID string `json:"request_id"`
				Response  struct {
					Behavior string `json:"behavior"`
					Input    struct {
						Path    string `json:"file_path"`
						Content string `json:"content"`
					} `json:"updatedInput"`
				} `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(scan.Bytes(), &message) != nil {
			continue
		}
		if message.Type == "user" {
			emit(map[string]any{"type": "control_request", "request_id": "write-request", "request": map[string]any{"subtype": "can_use_tool", "tool_name": "Write", "tool_use_id": "write-call", "input": map[string]any{"file_path": path, "content": "approved content\n"}}})
			if message.Message.Content == "cancel" {
				emit(map[string]any{"type": "control_cancel_request", "request_id": "write-request"})
				emit(map[string]any{"type": "result", "subtype": "success", "is_error": false})
			}
		}
		if message.Type == "control_response" && message.Response.RequestID == "write-request" {
			if message.Response.Response.Behavior == "allow" {
				if err := os.WriteFile(message.Response.Response.Input.Path, []byte(message.Response.Response.Input.Content), 0o600); err != nil {
					fmt.Fprintln(os.Stderr, err)
					return
				}
			}
			emit(map[string]any{"type": "result", "subtype": "success", "result": "answered", "is_error": false})
		}
	}
}
