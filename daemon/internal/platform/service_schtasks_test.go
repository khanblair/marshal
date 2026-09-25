package platform

import (
	"context"
	"encoding/csv"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestSchtasksTaskName(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeNormal, "Marshal Daemon"},
		{ModeDev, "Marshal Daemon Dev"},
	}
	for _, tc := range tests {
		if got := schtasksTaskName(tc.mode); got != tc.want {
			t.Errorf("schtasksTaskName(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestSchtasksRunValue(t *testing.T) {
	tests := []struct {
		name string
		spec ServiceSpec
		want string
	}{
		{
			name: "no arguments",
			spec: ServiceSpec{ExecutablePath: `C:\Program Files\Marshal\marshald.exe`},
			want: `"C:\Program Files\Marshal\marshald.exe"`,
		},
		{
			name: "plain arguments need no quoting",
			spec: ServiceSpec{ExecutablePath: `C:\Marshal\marshald.exe`, Args: []string{"--dev"}},
			want: `"C:\Marshal\marshald.exe" --dev`,
		},
		{
			name: "an argument with a space is quoted",
			spec: ServiceSpec{ExecutablePath: `C:\Marshal\marshald.exe`, Args: []string{"--data-dir", `C:\a b`}},
			want: `"C:\Marshal\marshald.exe" --data-dir "C:\a b"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := schtasksRunValue(tc.spec); got != tc.want {
				t.Errorf("schtasksRunValue = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSchtasksInstallerInstallBuildsTheExactArguments(t *testing.T) {
	runner := &fakeRunner{}
	inst := newSchtasksInstaller(runner.run)
	spec := ServiceSpec{ExecutablePath: `C:\Marshal\marshald.exe`, Args: []string{"--dev"}, Mode: ModeDev}

	if err := inst.Install(context.Background(), spec); err != nil {
		t.Fatalf("Install: %v", err)
	}
	wantCalls := [][]string{
		{
			"schtasks", "/Create", "/SC", "ONLOGON", "/TN", "Marshal Daemon Dev",
			"/TR", `"C:\Marshal\marshald.exe" --dev`, "/RL", "LIMITED", "/F",
		},
	}
	assertCalls(t, runner.calls, wantCalls)
}

func TestSchtasksInstallerInstallReturnsAPlainErrorOnFailure(t *testing.T) {
	key := cmdKey("schtasks", "/Create", "/SC", "ONLOGON", "/TN", "Marshal Daemon",
		"/TR", `"C:\Marshal\marshald.exe"`, "/RL", "LIMITED", "/F")
	runner := &fakeRunner{answers: map[string]fakeAnswer{key: {err: errors.New("exit status 1")}}}
	inst := newSchtasksInstaller(runner.run)

	err := inst.Install(context.Background(), ServiceSpec{ExecutablePath: `C:\Marshal\marshald.exe`, Mode: ModeNormal})
	if err == nil {
		t.Fatal("Install with a failing schtasks call = nil, want an error")
	}
}

func TestSchtasksInstallerUninstallToleratesAMissingTask(t *testing.T) {
	key := cmdKey("schtasks", "/Delete", "/TN", "Marshal Daemon", "/F")
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		key: {
			out: []byte(`ERROR: The specified task name "Marshal Daemon" does not exist in the system.`),
			err: errors.New("exit status 1"),
		},
	}}
	inst := newSchtasksInstaller(runner.run)

	if err := inst.Uninstall(context.Background(), ModeNormal); err != nil {
		t.Fatalf("Uninstall of a missing task = %v, want nil", err)
	}
}

func TestSchtasksInstallerUninstallBuildsTheExactArguments(t *testing.T) {
	runner := &fakeRunner{}
	inst := newSchtasksInstaller(runner.run)

	if err := inst.Uninstall(context.Background(), ModeDev); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	wantCalls := [][]string{{"schtasks", "/Delete", "/TN", "Marshal Daemon Dev", "/F"}}
	assertCalls(t, runner.calls, wantCalls)
}

// schtasksStatusQueryKey is the exact command Status runs, shared by every Status test below.
func schtasksStatusQueryKey() string {
	return cmdKey("schtasks", "/Query", "/TN", "Marshal Daemon", "/FO", "CSV", "/NH", "/V")
}

// schtasksVerboseRowFields are one realistic "/FO CSV /NH /V" row's fields (written from the
// tool's documented column order, not captured from a live command; see the report), with status
// and lastResult at their documented indices (3 and 6) and a "Task To Run" column that itself
// holds quotes, the case that broke a naive comma split.
func schtasksVerboseRowFields(status, lastResult string) []string {
	return []string{
		"WORKSTATION", `\Marshal Daemon`, "N/A", status, "Interactive/Background",
		"9/25/2026 8:00:00 AM", lastResult, "SYSTEM",
		`"C:\Marshal\marshald.exe" --dev`, `C:\Marshal\`, "N/A", "Enabled",
	}
}

// encodeCSVRow does the same doubled-quote escaping the real tool's CSV output does, so a test
// row is a faithful round trip through the same parser Status uses.
func encodeCSVRow(fields []string) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(fields); err != nil {
		panic(err) // a fixed, well-formed slice of strings never fails to encode
	}
	w.Flush()
	return b.String()
}

func schtasksVerboseRow(status string, lastResult int) string {
	return encodeCSVRow(schtasksVerboseRowFields(status, strconv.Itoa(lastResult)))
}

func TestSchtasksInstallerStatusNotFound(t *testing.T) {
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		schtasksStatusQueryKey(): {
			out: []byte(`ERROR: The system cannot find the file specified.`),
			err: errors.New("exit status 1"),
		},
	}}
	inst := newSchtasksInstaller(runner.run)

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.Installed {
		t.Errorf("Status = %+v, want Installed false", got)
	}
}

func TestSchtasksInstallerStatusRunning(t *testing.T) {
	// Last Result 0x41301 (267009) is SCHED_S_TASK_RUNNING, Task Scheduler's documented code for
	// a task currently running.
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		schtasksStatusQueryKey(): {out: []byte(schtasksVerboseRow("Running", 0x41301))},
	}}
	inst := newSchtasksInstaller(runner.run)

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !got.Installed || !got.Running {
		t.Errorf("Status = %+v, want Installed and Running true", got)
	}
}

// TestParseSchtasksQueryToleratesAnUnescapedQuote covers the one thing this package cannot
// confirm without a live Windows machine: whether real schtasks doubles a quote embedded in a
// field (RFC 4180 style, what encoding/csv.Writer does, and what every other test row here uses)
// or prints it bare. A hand-written row with a bare, un-doubled quote in "Task To Run" (this
// package's own /TR value, which always contains quotes) must still parse Status and Last Result
// correctly, since the reader is deliberately lenient about quoting.
func TestParseSchtasksQueryToleratesAnUnescapedQuote(t *testing.T) {
	row := `WORKSTATION,\Marshal Daemon,N/A,Running,Interactive/Background,9/25/2026 8:00:00 AM,267009,SYSTEM,"C:\Marshal\marshald.exe" --dev,C:\Marshal\,N/A,Enabled` + "\r\n"
	got := parseSchtasksQuery([]byte(row))
	if !got.Installed || !got.Running {
		t.Errorf("parseSchtasksQuery with a bare inner quote = %+v, want Installed and Running true", got)
	}
}

func TestSchtasksInstallerStatusReady(t *testing.T) {
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		schtasksStatusQueryKey(): {out: []byte(schtasksVerboseRow("Ready", 0))},
	}}
	inst := newSchtasksInstaller(runner.run)

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !got.Installed || got.Running {
		t.Errorf("Status = %+v, want Installed true and Running false", got)
	}
	if !strings.Contains(got.Detail, "Ready") {
		t.Errorf("Detail = %q, want it to mention the Status column", got.Detail)
	}
}

func TestSchtasksInstallerStatusUnparsableAnswerIsNotAnError(t *testing.T) {
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		schtasksStatusQueryKey(): {
			out: []byte("something unexpected"),
			err: errors.New("exit status 1"),
		},
	}}
	inst := newSchtasksInstaller(runner.run)

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status returned an error instead of an unknown-running status: %v", err)
	}
	if !got.Installed || got.Running || got.Detail == "" {
		t.Errorf("Status = %+v, want Installed true, Running false, and a Detail", got)
	}
}

func TestParseSchtasksQueryTooFewColumns(t *testing.T) {
	got := parseSchtasksQuery([]byte(`"\Marshal Daemon","N/A","Running"` + "\r\n"))
	if !got.Installed || got.Running || got.Detail == "" {
		t.Errorf("parseSchtasksQuery = %+v, want Installed true, Running false, and a Detail", got)
	}
}

func TestParseSchtasksQueryNonNumericLastResultIsNotRunning(t *testing.T) {
	row := encodeCSVRow(schtasksVerboseRowFields("Running", "N/A"))
	got := parseSchtasksQuery([]byte(row))
	if !got.Installed || got.Running {
		t.Errorf("parseSchtasksQuery with a non-numeric Last Result = %+v, want Running false", got)
	}
}
