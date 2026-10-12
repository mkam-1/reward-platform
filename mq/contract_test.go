package mq_test

import (
	"encoding/json"
	"os"
	"testing"

	"reward-platform/mq"
)

// Contract tests: SessionCompleted and PointsAwarded are the wire
// contract between independently deployed services (game-service
// publishes SessionCompleted, player-service consumes it and publishes
// PointsAwarded, rewards-service consumes that) — they currently happen
// to share this Go source because everything lives in one repo, but
// nothing stops that from changing (ADR 0006 already treats each
// service as independently buildable). These tests pin the exact JSON
// shape against a checked-in golden file: renaming, retyping, or
// removing a field changes test output, which is the point — it forces
// a human to notice "this is a breaking change to a contract another
// service depends on," the same way a schema-registry compatibility
// check would in a system using one.

func goldenFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read golden file %s: %v", name, err)
	}
	return data
}

func TestSessionCompletedContract(t *testing.T) {
	event := mq.SessionCompleted{
		PlayerID:       "player_1",
		GameID:         "game_1",
		Points:         15,
		Reason:         "played dice-run",
		IdempotencyKey: "session_1",
	}

	got, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := goldenFile(t, "session_completed.golden.json")
	if string(got) != string(want) {
		t.Fatalf("SessionCompleted JSON shape changed — if intentional, update testdata/session_completed.golden.json\ngot:\n%s\nwant:\n%s", got, want)
	}

	// Decode direction too: a shape change that still happens to encode
	// the same way (unlikely, but e.g. a field becoming unexported)
	// could still break decoding without the encode-side check above
	// catching it.
	var decoded mq.SessionCompleted
	if err := json.Unmarshal(want, &decoded); err != nil {
		t.Fatalf("unmarshal golden file: %v", err)
	}
	if decoded != event {
		t.Fatalf("decoded golden file = %+v, want %+v", decoded, event)
	}
}

func TestPointsAwardedContract(t *testing.T) {
	event := mq.PointsAwarded{PlayerID: "player_1", Points: 15}

	got, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := goldenFile(t, "points_awarded.golden.json")
	if string(got) != string(want) {
		t.Fatalf("PointsAwarded JSON shape changed — if intentional, update testdata/points_awarded.golden.json\ngot:\n%s\nwant:\n%s", got, want)
	}

	var decoded mq.PointsAwarded
	if err := json.Unmarshal(want, &decoded); err != nil {
		t.Fatalf("unmarshal golden file: %v", err)
	}
	if decoded != event {
		t.Fatalf("decoded golden file = %+v, want %+v", decoded, event)
	}
}
