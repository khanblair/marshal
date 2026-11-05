package session

import (
	"strings"
	"testing"
)

func TestFailedOneShotDoesNotReturnAPartialAnswer(t *testing.T) {
	var said strings.Builder
	said.WriteString("I will review this change.")
	answer, err := answerOf(&said, "usage_limit_exceeded")
	if err == nil || err.Error() != "usage_limit_exceeded" || answer != "" {
		t.Fatalf("partial failed review returned %q, %v", answer, err)
	}
}
