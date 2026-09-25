package platform

import (
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

const schtasksCommand = "schtasks"

// The columns "/FO CSV /NH /V" prints for one task, in the documented order: HostName, TaskName,
// Next Run Time, Status, Logon Mode, Last Run Time, Last Result, then further columns this
// package does not read. statusColumnIndex and lastResultColumnIndex are where the two fields
// this package needs sit among them; schtasksQueryMinColumns is the least this package can work
// with.
const (
	statusColumnIndex       = 3
	lastResultColumnIndex   = 6
	schtasksQueryMinColumns = lastResultColumnIndex + 1
	// schedRunningResult is SCHED_S_TASK_RUNNING (0x41301), the "Last Result" code Task Scheduler
	// documents for a task that is currently running. Status's own word ("Running") is locale
	// dependent, so Running is decided from this number instead.
	schedRunningResult = 0x41301
)

// schtasksInstaller is the Windows ServiceInstaller: a per-user scheduled task that starts at
// logon, the architecture's chosen approach for Windows since a login item needs no admin
// elevation there. Its command runner is injected, so tests can check exactly what it would run
// without ever calling the real schtasks.
type schtasksInstaller struct {
	run commandRunner
}

func newSchtasksInstaller(run commandRunner) *schtasksInstaller {
	return &schtasksInstaller{run: run}
}

// schtasksTaskName is the task's display name, distinct for the dev daemon so the two services
// never collide.
func schtasksTaskName(mode Mode) string {
	if mode == ModeDev {
		return "Marshal Daemon Dev"
	}
	return "Marshal Daemon"
}

// schtasksRunValue builds the /TR value: one command line string with the executable path always
// quoted (a Windows install path routinely has spaces, for example under "Program Files") and
// each argument quoted only when it holds a space, matching the brief's "quote/escape nothing you
// do not have to". This value becomes a single argv entry to the schtasks process; Go's exec
// package quotes it for that outer call on its own, so nothing here needs to escape it a second
// time.
func schtasksRunValue(spec ServiceSpec) string {
	parts := make([]string, 0, len(spec.Args)+1)
	parts = append(parts, `"`+spec.ExecutablePath+`"`)
	for _, arg := range spec.Args {
		parts = append(parts, schtasksQuoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func schtasksQuoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t") {
		return s
	}
	return `"` + s + `"`
}

// Install creates the scheduled task with a per-user logon trigger and no admin elevation
// (/RL LIMITED), replacing any task already installed under the same name (/F).
func (i *schtasksInstaller) Install(ctx context.Context, spec ServiceSpec) error {
	name := schtasksTaskName(spec.Mode)
	args := []string{
		"/Create", "/SC", "ONLOGON", "/TN", name, "/TR", schtasksRunValue(spec), "/RL", "LIMITED", "/F",
	}
	if out, err := i.run(ctx, schtasksCommand, args...); err != nil {
		return fmt.Errorf("create the scheduled task: %s", commandFailure(out, err))
	}
	return nil
}

// Uninstall removes the scheduled task. It does not fail if the task was already gone.
func (i *schtasksInstaller) Uninstall(ctx context.Context, mode Mode) error {
	name := schtasksTaskName(mode)
	out, err := i.run(ctx, schtasksCommand, "/Delete", "/TN", name, "/F")
	if err != nil && !schtasksTaskMissingOK(out) {
		return fmt.Errorf("remove the scheduled task: %s", commandFailure(out, err))
	}
	return nil
}

// schtasksTaskMissingOK reports whether a failed delete or query only means the task was not
// there. schtasks's own wording differs by command: "/Delete" on a missing task names it directly
// ("does not exist in the system"), while "/Query" answers with its more generic file-not-found
// text ("cannot find the file specified"), both observed, undocumented exact strings from the
// real tool.
func schtasksTaskMissingOK(out []byte) bool {
	lower := strings.ToLower(string(out))
	return strings.Contains(lower, "does not exist") || strings.Contains(lower, "cannot find the file specified")
}

// Status asks schtasks for the task in a machine-readable, verbose form (/FO CSV /NH /V: no
// header, one row, every documented column) and reads whether it exists and its last run result.
func (i *schtasksInstaller) Status(ctx context.Context, mode Mode) (ServiceStatus, error) {
	name := schtasksTaskName(mode)
	out, err := i.run(ctx, schtasksCommand, "/Query", "/TN", name, "/FO", "CSV", "/NH", "/V")
	if err != nil {
		if schtasksTaskMissingOK(out) {
			return ServiceStatus{}, nil
		}
		return ServiceStatus{Installed: true, Detail: "the running status could not be checked"}, nil
	}
	return parseSchtasksQuery(out), nil
}

// parseSchtasksQuery reads one CSV row from "schtasks /Query /FO CSV /NH /V" with a real CSV
// reader, since the row's own "Task To Run" column holds this package's /TR value, itself
// containing quotes; a naive comma split breaks on that. Whether real schtasks doubles an
// embedded quote the way encoding/csv's writer does (RFC 4180 style) is not confirmed against a
// live command (see the report), so the reader is deliberately lenient: LazyQuotes tolerates a
// bare quote inside a field instead of failing the whole row over it, and FieldsPerRecord is left
// unbound in case the real column count differs from what this package expects. Running comes
// from the numeric Last Result column, not the (locale-dependent) Status word, which still goes
// into Detail. An unparsable row is reported as installed with the running state unknown, rather
// than crashing.
func parseSchtasksQuery(out []byte) ServiceStatus {
	reader := csv.NewReader(strings.NewReader(firstNonEmptyLine(out)))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1
	fields, err := reader.Read()
	if err != nil || len(fields) < schtasksQueryMinColumns {
		return ServiceStatus{Installed: true, Detail: "the running status could not be read"}
	}
	status := fields[statusColumnIndex]
	lastResult, err := strconv.ParseInt(strings.TrimSpace(fields[lastResultColumnIndex]), 10, 64)
	running := err == nil && lastResult == schedRunningResult
	return ServiceStatus{Installed: true, Running: running, Detail: "status: " + status}
}
