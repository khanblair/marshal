package events

import "testing"

func TestFilterMatches(t *testing.T) {
	tests := []struct {
		name   string
		filter Filter
		topic  string
		want   bool
	}{
		{"the zero filter follows nothing", Filter{}, "a", false},
		{"no topics follow nothing", Topics(), "a", false},
		{"an exact topic", Topics("project:api"), "project:api", true},
		{"topics match exactly, not by prefix", Topics("project:api"), "project:api-2", false},
		{"one of several", Topics("a", "b", "c"), "b", true},
		{"all topics", AllTopics(), "anything", true},
		{"all topics, empty topic", AllTopics(), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Matches(tt.topic); got != tt.want {
				t.Errorf("Matches(%q) = %v, want %v", tt.topic, got, tt.want)
			}
		})
	}
}

func TestFilterAddAndRemove(t *testing.T) {
	var f Filter
	f.add("a", "b")
	f.remove("a", "missing")
	if f.Matches("a") || !f.Matches("b") {
		t.Errorf("after add and remove: a=%v b=%v, want false and true", f.Matches("a"), f.Matches("b"))
	}
	clone := f.clone()
	clone.add("c")
	if f.Matches("c") || !clone.Matches("b") || !clone.Matches("c") {
		t.Error("a clone should start with the same topics and change on its own")
	}
	if !AllTopics().clone().Matches("x") {
		t.Error("cloning lost the wildcard")
	}
}
