package transcript

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

const notifBody = `<task-notification>
<task-id>acb6584f99f2f81fd</task-id>
<tool-use-id>toolu_0112a19E</tool-use-id>
<output-file>/tmp/tasks/acb6584f99f2f81fd.output</output-file>
<status>completed</status>
<summary>Agent "Implement Task 12: Profiles view" finished</summary>
<note>may notify more than once</note>
<result>Both items fixed.

## Status: Complete</result>
</task-notification>`

func notifLine(t *testing.T, body string) string {
	t.Helper()
	b, err := jsonMarshalString(body)
	if err != nil {
		t.Fatal(err)
	}
	return `{"type":"user","timestamp":"2026-07-05T18:26:08.000Z","message":{"role":"user","content":` + b + `}}`
}

func TestParseTaskNotificationTurn(t *testing.T) {
	s, err := Parse(strings.NewReader(notifLine(t, notifBody)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(s.Turns))
	}
	turn := s.Turns[0]
	if turn.Kind != model.TurnAgentResult {
		t.Fatalf("kind = %q, want %q", turn.Kind, model.TurnAgentResult)
	}
	if turn.AgentID != "acb6584f99f2f81fd" {
		t.Errorf("AgentID = %q", turn.AgentID)
	}
	if turn.AgentStatus != "completed" {
		t.Errorf("AgentStatus = %q", turn.AgentStatus)
	}
	if want := `Agent "Implement Task 12: Profiles view" finished`; turn.AgentSummary != want {
		t.Errorf("AgentSummary = %q", turn.AgentSummary)
	}
	if len(turn.Blocks) != 1 || !strings.Contains(turn.Blocks[0].Text, "## Status: Complete") {
		t.Errorf("body block wrong: %+v", turn.Blocks)
	}
	if strings.Contains(turn.Blocks[0].Text, "<task-id>") {
		t.Error("body must be the <result> payload only, not the raw envelope")
	}
}

func TestParseTaskNotificationEmptyResultFallsBackToSummary(t *testing.T) {
	body := "<task-notification>\n<task-id>a1</task-id>\n<status>completed</status>\n<summary>Agent \"x\" finished</summary>\n<result></result>\n</task-notification>"
	s, err := Parse(strings.NewReader(notifLine(t, body)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 || s.Turns[0].Blocks[0].Text != `Agent "x" finished` {
		t.Fatalf("want summary as body, got %+v", s.Turns)
	}
}

func TestParseMalformedNotificationStaysUserTurn(t *testing.T) {
	// No <task-id> -> must stay a plain user turn, content untouched.
	body := "<task-notification>\nbroken payload\n</task-notification>"
	s, err := Parse(strings.NewReader(notifLine(t, body)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 || s.Turns[0].Kind != model.TurnUser {
		t.Fatalf("want plain user turn, got %+v", s.Turns)
	}
}

func TestUserTextMentioningNotificationMidStringStaysUserTurn(t *testing.T) {
	body := "please explain what a <task-notification> record is"
	s, err := Parse(strings.NewReader(notifLine(t, body)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 || s.Turns[0].Kind != model.TurnUser {
		t.Fatalf("want plain user turn, got %+v", s.Turns)
	}
}

func TestRepeatNotificationsEachProduceATurn(t *testing.T) {
	line := notifLine(t, notifBody)
	s, err := Parse(strings.NewReader(line+"\n"+line), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 2 {
		t.Fatalf("got %d turns, want 2 (no dedup)", len(s.Turns))
	}
}

func TestParseNotificationResultQuotingOtherTags(t *testing.T) {
	// A <result> body that quotes another notification's envelope must not
	// corrupt the simple fields: task-id/summary match their FIRST closing tag.
	body := "<task-notification>\n<task-id>real-id</task-id>\n<status>completed</status>\n" +
		"<summary>Agent \"x\" finished</summary>\n" +
		"<result>the agent saw `</task-id>` and `</summary>` in a quoted envelope</result>\n" +
		"</task-notification>"
	s, err := Parse(strings.NewReader(notifLine(t, body)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 || s.Turns[0].Kind != model.TurnAgentResult {
		t.Fatalf("want one agent-result turn, got %+v", s.Turns)
	}
	if s.Turns[0].AgentID != "real-id" {
		t.Errorf("AgentID = %q, want real-id (must not span into the result body)", s.Turns[0].AgentID)
	}
	if want := `Agent "x" finished`; s.Turns[0].AgentSummary != want {
		t.Errorf("AgentSummary = %q, want %q", s.Turns[0].AgentSummary, want)
	}
}

// jsonMarshalString wraps a string as a JSON string literal.
func jsonMarshalString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}
