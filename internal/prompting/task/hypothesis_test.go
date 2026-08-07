package task

import (
	"encoding/json"
	"testing"
)

func TestLikelihoodConstants(t *testing.T) {
	if LikelihoodHigh != "H" {
		t.Errorf("LikelihoodHigh = %q, want %q", LikelihoodHigh, "H")
	}
	if LikelihoodMedium != "M" {
		t.Errorf("LikelihoodMedium = %q, want %q", LikelihoodMedium, "M")
	}
	if LikelihoodLow != "L" {
		t.Errorf("LikelihoodLow = %q, want %q", LikelihoodLow, "L")
	}
	if LikelihoodImpossible != "I" {
		t.Errorf("LikelihoodImpossible = %q, want %q", LikelihoodImpossible, "I")
	}
	// Zero value is distinct from every valid likelihood — a typo or missing
	// assignment should never accidentally match a sentinel.
	if Likelihood("") == LikelihoodLow {
		t.Error("zero Likelihood must not equal any valid sentinel")
	}
}

func TestStanceConstants(t *testing.T) {
	if StanceSupports != "supports" {
		t.Errorf("StanceSupports = %q, want %q", StanceSupports, "supports")
	}
	if StanceRefutes != "refutes" {
		t.Errorf("StanceRefutes = %q, want %q", StanceRefutes, "refutes")
	}
}

func TestEvidenceZeroValue(t *testing.T) {
	var e Evidence
	if e.NodeID != "" {
		t.Errorf("zero Evidence.NodeID = %q, want empty string", e.NodeID)
	}
	if e.Stance != "" {
		t.Errorf("zero Evidence.Stance = %q, want empty string", e.Stance)
	}
}

func TestAssertionOutcomeConstants(t *testing.T) {
	if OutcomeSatisfied != "satisfied" {
		t.Errorf("OutcomeSatisfied = %q, want %q", OutcomeSatisfied, "satisfied")
	}
	if OutcomeRefuted != "refuted" {
		t.Errorf("OutcomeRefuted = %q, want %q", OutcomeRefuted, "refuted")
	}
	if OutcomeAbstained != "abstained" {
		t.Errorf("OutcomeAbstained = %q, want %q", OutcomeAbstained, "abstained")
	}
}

func TestResolutionAssertionRoundTrip(t *testing.T) {
	a := ResolutionAssertion{
		Text:     "find the root cause",
		Outcome:  OutcomeSatisfied,
		Evidence: []Evidence{{NodeID: "fact-1", Stance: StanceSupports}},
		Declared: "initial",
	}

	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got ResolutionAssertion
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Text != a.Text {
		t.Errorf("Text = %q, want %q", got.Text, a.Text)
	}
	if got.Outcome != a.Outcome {
		t.Errorf("Outcome = %q, want %q", got.Outcome, a.Outcome)
	}
	if len(got.Evidence) != 1 {
		t.Fatalf("Evidence len = %d, want 1", len(got.Evidence))
	}
	if got.Evidence[0].NodeID != "fact-1" {
		t.Errorf("Evidence[0].NodeID = %q, want %q", got.Evidence[0].NodeID, "fact-1")
	}
	if got.Declared != "initial" {
		t.Errorf("Declared = %q, want %q", got.Declared, "initial")
	}
}

func TestResolutionAssertionOmitEmpty(t *testing.T) {
	// Outcome and Evidence have omitempty; a zero-value ResolutionAssertion
	// must not emit them in JSON.
	a := ResolutionAssertion{
		Text:     "test",
		Declared: "initial",
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	body := string(data)
	for _, field := range []string{"outcome", "evidence"} {
		if containsString(body, `"`+field+`"`) {
			t.Errorf("expected %q to be omitted; got %s", field, body)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
