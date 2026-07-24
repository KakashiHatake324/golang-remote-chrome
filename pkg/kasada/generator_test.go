package kasada

import (
	"encoding/json"
	"testing"
)

func TestGenerateCD(t *testing.T) {
	cdJSON := GenerateCD(1710000000000)
	if cdJSON == "" {
		t.Fatal("expected non-empty CD JSON")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(cdJSON), &payload); err != nil {
		t.Fatalf("invalid CD JSON: %v", err)
	}

	for _, key := range []string{"workTime", "id", "answers", "d", "rst", "st", "duration"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing key %q in CD payload", key)
		}
	}

	answers, ok := payload["answers"].([]any)
	if !ok || len(answers) != 2 {
		t.Fatalf("expected 2 answers, got %#v", payload["answers"])
	}

	if payload["duration"] != "10.0" {
		t.Fatalf("expected duration 10.0, got %#v", payload["duration"])
	}

	if payload["st"].(float64) != 1710000000000 {
		t.Fatalf("unexpected st: %#v", payload["st"])
	}
}

func TestRefreshCDPath(t *testing.T) {
	solver := &SolveKasada{KpsdkST: 1710000000000}
	if err := solver.HandleKasada(); err != nil {
		t.Fatalf("HandleKasada refresh failed: %v", err)
	}
	if solver.XKpsdkCd == "" {
		t.Fatal("expected XKpsdkCd to be set")
	}
}
