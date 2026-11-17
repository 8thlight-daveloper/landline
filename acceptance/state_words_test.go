package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLandlineAgentsTextAndJSONKeepTheWordsConnectedAndDisconnectedAndShowNoShapeOrOtherWord(t *testing.T) {
	w := start(t)
	w.register("bravo")
	live := w.selfRegisteredWithLabel("a-x", "")
	text := runBin(t, w.bin, w.env, "agents")
	if text.code != 0 || text.stderr != "" {
		t.Fatalf("%+v", text)
	}
	states := map[string]string{}
	for _, row := range strings.Split(strings.TrimSuffix(text.stdout, "\n"), "\n") {
		fields := strings.Split(row, "\t")
		if len(fields) < 4 {
			t.Fatalf("the row %q has fewer than 4 tab separated fields", row)
		}
		states[fields[0]] = fields[3]
	}
	if states["bravo"] != "disconnected" || states[live] != "connected" || len(states) != 2 {
		t.Errorf("the text states are %v", states)
	}
	for _, shape := range []string{"●", "○", "active", "idle"} {
		if strings.Contains(text.stdout, shape) {
			t.Errorf("the text of agents shows %q: %s", shape, text.stdout)
		}
	}
	asJSON := runBin(t, w.bin, w.env, "agents", "--json")
	var snapshot struct {
		Agents []map[string]any `json:"agents"`
	}
	if err := json.Unmarshal([]byte(asJSON.stdout), &snapshot); err != nil || asJSON.code != 0 {
		t.Fatalf("%v %+v", err, asJSON)
	}
	got := map[string]any{}
	for _, entry := range snapshot.Agents {
		got[entry["handle"].(string)] = entry["state"]
	}
	if got["bravo"] != "disconnected" || got[live] != "connected" || len(got) != 2 {
		t.Errorf("the JSON states are %v", got)
	}
	for _, shape := range []string{"●", "○"} {
		if strings.Contains(asJSON.stdout, shape) {
			t.Errorf("the JSON shows %q", shape)
		}
	}
}
