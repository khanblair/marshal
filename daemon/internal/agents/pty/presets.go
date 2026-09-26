package pty

import "github.com/khanblair/marshal/daemon/internal/agents"

// ClaudeConfig runs Claude Code's own interactive command, for the terminal view of a card whose
// agent is Claude Code. It is the same CLI that the chat view drives through its streaming JSON
// mode, and both name a session by the id Claude Code was given with --session-id, so a card can
// switch between the two and keep its conversation: the terminal resumes the id that the chat view
// started, and the chat view resumes the id that the terminal continued.
//
// path is the claude program, and env is added to its environment (the folder of the program first
// on PATH, so that the node it needs is found, as the chat view does). Only the model is passed on
// from the card: the permission mode and the thinking setting are Claude Code's own to ask about in
// its terminal, where a person can answer, and its own settings apply.
func ClaudeConfig(path string, env []string) Config {
	return Config{
		Path: path,
		Env:  env,
		StartArgs: func(sessionID string) []string {
			return []string{"--session-id=" + sessionID}
		},
		ResumeArgs: func(sessionID string) []string {
			return []string{"--resume=" + sessionID}
		},
		SpecArgs: func(spec agents.StartSpec) []string {
			if spec.Model == "" {
				return nil
			}
			return []string{"--model=" + spec.Model}
		},
		// Claude Code draws its own screen in raw mode, where Enter is a carriage return.
		LineEnd: "\r",
	}
}
