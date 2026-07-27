package scenario

import (
	"strings"
	"testing"
)

func ptr(v int) *int { return &v }

func TestDefinitionCompilesToTheDescribedTrigger(t *testing.T) {
	def := Definition{
		FromMinute:       ptr(70),
		ScoreDiffAtLeast: ptr(2),
		TargetOdds:       1.5,
	}
	sc := def.Compile()

	if !sc.Trigger(State{Second: 70 * 60, ScoreHome: 2}) {
		t.Error("should fire at 70' with a two-goal lead")
	}
	if sc.Trigger(State{Second: 69 * 60, ScoreHome: 2}) {
		t.Error("must not fire before the 70th minute")
	}
	if sc.Trigger(State{Second: 80 * 60, ScoreHome: 1}) {
		t.Error("must not fire on a one-goal lead")
	}
}

// The gap is what matters, not who holds it.
func TestScoreConditionsReadTheGapNotTheLeader(t *testing.T) {
	sc := Definition{ScoreDiffAtLeast: ptr(2), TargetOdds: 1.5}.Compile()
	if !sc.Trigger(State{ScoreHome: 0, ScoreAway: 2}) {
		t.Error("an away lead of two must fire just like a home lead")
	}
}

// An unconditioned scenario measures the base rate of football. Returning it
// as a strategy invites reading that base rate as an edge.
func TestUnconditionedDefinitionIsRejected(t *testing.T) {
	if err := (Definition{TargetOdds: 1.5}).Validate(); err == nil {
		t.Fatal("expected a definition with no conditions to be rejected")
	}
}

func TestTargetOddsMustBeAPrice(t *testing.T) {
	for _, odds := range []float64{0, 1.0, -2} {
		if err := (Definition{FromMinute: ptr(70), TargetOdds: odds}).Validate(); err == nil {
			t.Errorf("odds %v should be rejected", odds)
		}
	}
}

func TestRunRefusesAVerdictOnATinySample(t *testing.T) {
	matches := []MatchTimeline{{MatchID: "a", EndSecond: 5400, Goals: []Goal{{Second: 600}}}}
	report, err := Run(Definition{FromMinute: ptr(70), TargetOdds: 1.5}, matches, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	if report.SampleSufficient {
		t.Error("one match cannot be a sufficient sample")
	}
	if report.Verdict == "" {
		t.Error("a verdict must still explain why nothing can be said")
	}
}

// A scenario that clears the price but not the base rate is not informing
// anything, and the verdict has to say so rather than celebrate.
func TestVerdictFlagsAScenarioThatOnlyMatchesTheBaseRate(t *testing.T) {
	r := Report{
		Occurrences: 500, HitRate: 0.80, RateLow: 0.78, BaselineHitRate: 0.85,
		Worthwhile: true, BeatsBaseline: false, SampleSufficient: true,
	}
	got := verdictFor(r, 1.2, 1/1.2)
	if !strings.Contains(got, "taxa base") {
		t.Fatalf("verdict should call out the base rate, got %q", got)
	}
}
