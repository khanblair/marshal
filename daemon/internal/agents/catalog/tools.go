package catalog

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A tool is another coding agent program that Marshal looks for and tests but cannot start sessions
// with yet. Each says how it takes its work, which is what decides what an adapter for it would be:
// an ACP server rides the same adapter Gemini CLI uses, a print-mode CLI needs one written for its
// own streaming format.
type tool struct {
	id      string
	name    string
	program string
	// iface is how the program takes its work.
	iface protocol.AgentToolInterface
	// acpArgs are the arguments that start it as an ACP server. Empty for other interfaces.
	acpArgs []string
	note    string
}

const (
	noteACP   = "It speaks the Agent Client Protocol, the one Gemini CLI uses, so Marshal could start it with the same adapter. That adapter is not switched on for it yet."
	notePrint = "It runs one prompt at a time in print mode with its own streaming format. Marshal has no adapter for that format yet."
	noteRPC   = "It has its own RPC mode. Marshal has no adapter for it yet."
)

// tools lists the other agent programs Marshal looks for, in the order they are shown.
func tools() []tool {
	return []tool{
		{id: "qwen", name: "Qwen Code", program: "qwen", iface: protocol.AgentToolInterfaceACP, acpArgs: []string{"--acp"}, note: noteACP},
		{id: "kimi", name: "Kimi CLI", program: "kimi", iface: protocol.AgentToolInterfaceACP, acpArgs: []string{"acp"}, note: noteACP},
		{id: "minimax", name: "MiniMax Code", program: "mcode", iface: protocol.AgentToolInterfaceACP, acpArgs: []string{"acp"}, note: noteACP},
		{id: "cursor", name: "Cursor Agent", program: "cursor-agent", iface: protocol.AgentToolInterfacePrint, note: notePrint},
		{id: "antigravity", name: "Antigravity CLI", program: "agy", iface: protocol.AgentToolInterfacePrint, note: notePrint},
		{id: "openclaude", name: "OpenClaude", program: "openclaude", iface: protocol.AgentToolInterfacePrint, note: notePrint},
		{id: "pi", name: "Pi", program: "pi", iface: protocol.AgentToolInterfaceRPC, note: noteRPC},
	}
}

// toolByID finds one tool.
func toolByID(id string) (tool, bool) {
	for _, t := range tools() {
		if t.id == id {
			return t, true
		}
	}
	return tool{}, false
}

// ProgramProbe finds a program by name and reads its version. It is the part of a probe that the
// tools need: they are not agent kinds, so they are found by program name.
type ProgramProber interface {
	ProbeProgram(ctx context.Context, program string) (Found, error)
}

// ToolDetected is what detection found for one tool. Only installed ones are kept.
type ToolDetected struct {
	Tool    string
	Path    string
	Version string
}

// ProbeProgram finds a program on the PATH and in the well-known folders and reads its version.
func (p *ProgramProbe) ProbeProgram(ctx context.Context, program string) (Found, error) {
	path, ok := p.locate(program)
	if !ok {
		return Found{}, ErrNotFound
	}
	version, err := p.readVersion(ctx, path)
	if err != nil {
		return Found{Path: path}, errors.Join(ErrUnreadable, err)
	}
	return Found{Path: path, Version: version}, nil
}

// detectTools asks the probe about every tool at the same time, when the probe can look for a
// program by name. A probe that cannot (a test's fake) finds no tools.
func (c *Catalog) detectTools(ctx context.Context) []ToolDetected {
	prober, ok := c.probe.(ProgramProber)
	if !ok {
		return nil
	}
	list := tools()
	found := make([]*ToolDetected, len(list))
	var wg sync.WaitGroup
	for i, t := range list {
		wg.Go(func() {
			f, err := prober.ProbeProgram(ctx, t.program)
			if err != nil && f.Path == "" {
				return
			}
			d := &ToolDetected{Tool: t.id, Path: f.Path}
			if err == nil {
				d.Version = f.Version.String()
			}
			found[i] = d
		})
	}
	wg.Wait()
	out := make([]ToolDetected, 0, len(list))
	for _, d := range found {
		if d != nil {
			out = append(out, *d)
		}
	}
	return out
}

// toolsOf makes the wire list of the tools that were found.
func toolsOf(found []ToolDetected) []protocol.AgentTool {
	out := make([]protocol.AgentTool, 0, len(found))
	for _, d := range found {
		t, ok := toolByID(d.Tool)
		if !ok {
			continue
		}
		note := t.note
		version := d.Version
		if version == "" {
			note = "Marshal found it but could not read its version. " + note
		}
		out = append(out, protocol.AgentTool{ID: t.id, Name: t.name, Version: version, Interface: t.iface, Note: note})
	}
	return slices.Clone(out)
}
