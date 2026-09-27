package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// sampleCheckpoints is a card with three restore points newest first: the one a person asked for by
// hand (which carries a label), the one made before a turn, and the first one, made when the card
// started, whose label is empty so the app names the moment by its time instead.
func sampleCheckpoints() []protocol.Checkpoint {
	return []protocol.Checkpoint{
		{
			ID: "01JD7Q4M2X8K9V0P5T3RB6NHAE", CardID: "01JD7Q4M2X8K9V0P5T3RB6NHC3",
			SHA: "9f2c1b7a4e6d85031234567890abcdef01234567", Label: "before the retry work",
			CreatedAt: protocol.NewTimestamp(providersNow),
		},
		{
			ID: "01JD7Q4M2X8K9V0P5T3RB6NHB7", CardID: "01JD7Q4M2X8K9V0P5T3RB6NHC3",
			SHA: "3c8d0f5b2a194e6789012345678abcdef0123456", Label: "before turn 3",
			CreatedAt: protocol.NewTimestamp(providersNow),
		},
		{
			ID: "01JD7Q4M2X8K9V0P5T3RB6NHD1", CardID: "01JD7Q4M2X8K9V0P5T3RB6NHC3",
			SHA: "0a1b2c3d4e5f60718293a4b5c6d7e8f901234567", Label: "",
			CreatedAt: protocol.NewTimestamp(providersNow),
		},
	}
}

func TestCheckpointListGolden(t *testing.T) {
	testutil.Golden(t, "checkpoint-list",
		protocol.NewCheckpointList("01JD7Q4M2X8K9V0P5T3RB6NHC3", sampleCheckpoints(), providersNow))
}

func TestCheckpointListNeverEncodesNull(t *testing.T) {
	list, err := json.Marshal(protocol.NewCheckpointList("card-1", nil, providersNow))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(list); !strings.Contains(got, `"checkpoints":[]`) {
		t.Errorf("an empty checkpoint list encoded as %s, want []", got)
	}
}

func TestRestoreCheckpointRequestGolden(t *testing.T) {
	testutil.Golden(t, "restore-checkpoint-request", protocol.RestoreCheckpointRequest{Conversation: true})
}

func TestRestoreCheckpointRequestDefaultIsNothing(t *testing.T) {
	body, err := json.Marshal(protocol.RestoreCheckpointRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "{}" {
		t.Errorf("an empty restore request encoded as %s, want {}", got)
	}
}
