package proc

import (
	"slices"
	"testing"
)

func TestEnvironFiltersTheDaemonEnvironment(t *testing.T) {
	t.Setenv("MARSHAL_PROC_TREE_SECRET", "hunter2")

	env := Environ([]string{"MARSHAL_PROC_TREE_EXTRA=given", "TERM=xterm-256color"})

	for _, entry := range env {
		if entry == "MARSHAL_PROC_TREE_SECRET=hunter2" {
			t.Fatalf("a variable outside the allow-list reached the child: %q", entry)
		}
	}
	if !slices.Contains(env, "MARSHAL_PROC_TREE_EXTRA=given") || !slices.Contains(env, "TERM=xterm-256color") {
		t.Errorf("the extra entries are missing from %v", env)
	}
}
