package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeBeads writes the given JSONL content to .beads/beads.jsonl under dir.
func writeBeads(t *testing.T, dir, content string) {
	t.Helper()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("write beads: %v", err)
	}
}

func runRobotJSON(t *testing.T, bv, dir string, flag string, v any) {
	t.Helper()
	out, err := runCommand(bv, dir, flag)
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", flag, err, out)
	}
	if err := json.Unmarshal(out, v); err != nil {
		t.Fatalf("%s json decode: %v\nout=%s", flag, err, out)
	}
}

// runCommand is a tiny helper to exec the bv binary with a single flag.
func runCommand(bv, dir, flag string) ([]byte, error) {
	cmd := execCommand(bv, flag)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// execCommand is defined in other e2e tests; redeclare wrapper to avoid imports.
func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func TestRobotInsightsContract(t *testing.T) {
	bv := buildBvBinary(t)
	env := t.TempDir()
	// Simple chain A->B->C to populate metrics.
	writeBeads(t, env, `{"id":"A","title":"Root","status":"open","priority":1,"issue_type":"task"}
{"id":"B","title":"Mid","status":"open","priority":2,"issue_type":"task","dependencies":[{"issue_id":"B","depends_on_id":"A","type":"blocks"}]}
{"id":"C","title":"Leaf","status":"open","priority":3,"issue_type":"task","dependencies":[{"issue_id":"C","depends_on_id":"B","type":"blocks"}]}`)

	var first map[string]any
	runRobotJSON(t, bv, env, "--robot-insights", &first)

	// Basic contract checks
	if first["data_hash"] == "" {
		t.Fatalf("insights missing data_hash")
	}
	if first["analysis_config"] == nil {
		t.Fatalf("insights missing analysis_config")
	}
	status, ok := first["status"].(map[string]any)
	if !ok || len(status) == 0 {
		t.Fatalf("insights missing status map: %v", first["status"])
	}

	// Determinism: second call should share the same data_hash
	var second map[string]any
	runRobotJSON(t, bv, env, "--robot-insights", &second)
	if first["data_hash"] != second["data_hash"] {
		t.Fatalf("data_hash changed between calls: %v vs %v", first["data_hash"], second["data_hash"])
	}
}

func TestRobotPlanContract(t *testing.T) {
	bv := buildBvBinary(t)
	env := t.TempDir()
	// A unblocks B; expect A actionable, unblocks contains B.
	writeBeads(t, env, `{"id":"A","title":"Unblocker","status":"open","priority":1,"issue_type":"task"}
{"id":"B","title":"Blocked","status":"open","priority":2,"issue_type":"task","dependencies":[{"issue_id":"B","depends_on_id":"A","type":"blocks"}]}`)

	var payload struct {
		DataHash string `json:"data_hash"`
		Plan     struct {
			Tracks []struct {
				Items []struct {
					ID       string   `json:"id"`
					Unblocks []string `json:"unblocks"`
				} `json:"items"`
			} `json:"tracks"`
		} `json:"plan"`
	}
	runRobotJSON(t, bv, env, "--robot-plan", &payload)

	if payload.DataHash == "" {
		t.Fatalf("plan missing data_hash")
	}
	if len(payload.Plan.Tracks) == 0 || len(payload.Plan.Tracks[0].Items) == 0 {
		t.Fatalf("plan missing tracks/items: %#v", payload.Plan)
	}
	item := payload.Plan.Tracks[0].Items[0]
	if item.ID != "A" {
		t.Fatalf("expected actionable A first, got %s", item.ID)
	}
	if len(item.Unblocks) == 0 || item.Unblocks[0] != "B" {
		t.Fatalf("expected A to unblock B, got %v", item.Unblocks)
	}
}

func TestRobotPriorityContract(t *testing.T) {
	bv := buildBvBinary(t)
	env := t.TempDir()
	// Mis-prioritized root with two dependents to ensure a recommendation.
	writeBeads(t, env, `{"id":"P0","title":"Low but critical","status":"open","priority":5,"issue_type":"task"}
{"id":"D1","title":"Dep1","status":"open","priority":1,"issue_type":"task","dependencies":[{"issue_id":"D1","depends_on_id":"P0","type":"blocks"}]}
{"id":"D2","title":"Dep2","status":"open","priority":1,"issue_type":"task","dependencies":[{"issue_id":"D2","depends_on_id":"P0","type":"blocks"}]}`)

	var payload struct {
		DataHash        string `json:"data_hash"`
		Recommendations []struct {
			IssueID     string   `json:"issue_id"`
			Confidence  float64  `json:"confidence"`
			Reasoning   []string `json:"reasoning"`
			SuggestedPr int      `json:"suggested_priority"`
			CurrentPr   int      `json:"current_priority"`
			Direction   string   `json:"direction"`
		} `json:"recommendations"`
	}
	runRobotJSON(t, bv, env, "--robot-priority", &payload)

	if payload.DataHash == "" {
		t.Fatalf("priority missing data_hash")
	}
	if len(payload.Recommendations) == 0 {
		t.Fatalf("expected at least one recommendation")
	}
	// Expect the root P0 to be suggested
	found := false
	for _, r := range payload.Recommendations {
		if r.IssueID == "P0" && r.Confidence > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected recommendation for P0, got %+v", payload.Recommendations)
	}
}

func TestRobotTriageContract(t *testing.T) {
	bv := buildBvBinary(t)
	env := t.TempDir()
	// Simple issues for triage
	writeBeads(t, env, `{"id":"A","title":"Blocker","status":"open","priority":1,"issue_type":"task"}
{"id":"B","title":"Blocked","status":"open","priority":2,"issue_type":"task","dependencies":[{"issue_id":"B","depends_on_id":"A","type":"blocks"}]}`)

	var payload struct {
		GeneratedAt string `json:"generated_at"`
		DataHash    string `json:"data_hash"`
		Triage      struct {
			QuickRef struct {
				TopPicks []struct {
					ID    string  `json:"id"`
					Score float64 `json:"score"`
				} `json:"top_picks"`
			} `json:"quick_ref"`
		} `json:"triage"`
		UsageHints []string `json:"usage_hints"`
	}
	runRobotJSON(t, bv, env, "--robot-triage", &payload)

	if payload.DataHash == "" {
		t.Fatalf("triage missing data_hash")
	}
	if payload.GeneratedAt == "" {
		t.Fatalf("triage missing generated_at")
	}
	if len(payload.UsageHints) == 0 {
		t.Fatalf("triage missing usage_hints")
	}
	// Should have quick_ref.top_picks
	if len(payload.Triage.QuickRef.TopPicks) == 0 {
		t.Fatalf("triage missing quick_ref.top_picks")
	}
}

func TestRobotUsageHintsPresent(t *testing.T) {
	bv := buildBvBinary(t)
	env := t.TempDir()
	writeBeads(t, env, `{"id":"A","title":"Test","status":"open","priority":1,"issue_type":"task"}`)

	tests := []struct {
		flag string
		name string
	}{
		{"--robot-insights", "insights"},
		{"--robot-plan", "plan"},
		{"--robot-priority", "priority"},
		{"--robot-suggest", "suggest"},
		{"--robot-triage", "triage"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var payload map[string]any
			runRobotJSON(t, bv, env, tc.flag, &payload)

			hints, ok := payload["usage_hints"].([]any)
			if !ok || len(hints) == 0 {
				t.Fatalf("%s missing usage_hints array", tc.flag)
			}
			// Verify hints are non-empty strings
			for i, hint := range hints {
				s, ok := hint.(string)
				if !ok || s == "" {
					t.Fatalf("%s usage_hints[%d] is not a non-empty string: %v", tc.flag, i, hint)
				}
			}
		})
	}
}
