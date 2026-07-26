package scenario

import (
	"math"
	"testing"
)

// A three-match set where the trigger and the outcome are both known by hand,
// so the counting can be checked without trusting the machinery.
func handMadeMatches() []MatchTimeline {
	return []MatchTimeline{
		{
			// Leading 2-0 from the 60th; a goal follows at 80'.
			MatchID:   "a",
			EndSecond: 5400,
			Goals: []Goal{
				{Second: 600, Home: true},
				{Second: 3600, Home: true},
				{Second: 4800, Home: false},
			},
		},
		{
			// Leading 2-0 from the 60th; nothing else happens.
			MatchID:   "b",
			EndSecond: 5400,
			Goals: []Goal{
				{Second: 600, Home: true},
				{Second: 3600, Home: true},
			},
		},
		{
			// Never more than one goal apart.
			MatchID:   "c",
			EndSecond: 5400,
			Goals: []Goal{
				{Second: 1200, Home: true},
				{Second: 2400, Home: false},
			},
		},
	}
}

func TestEvaluateCountsOccurrencesAndWins(t *testing.T) {
	sc := Scenario{
		Name:    "test",
		Trigger: func(s State) bool { return s.Second >= 70*60 && s.AbsScoreDiff() >= 2 },
	}

	got := Evaluate(sc, handMadeMatches())

	// Match c never fires. Matches a and b each contribute exactly one
	// observation, taken at the first minute the condition holds.
	if got.Occurrences != 2 || got.MatchCount != 2 {
		t.Fatalf("got %d occurrences over %d matches, want 2 and 2", got.Occurrences, got.MatchCount)
	}
	// Match a scores again at 80', match b does not.
	if got.Wins != 1 {
		t.Fatalf("Wins = %d, want 1", got.Wins)
	}
}

// A condition that holds for twenty minutes is one betting opportunity, not
// twenty. Counting each minute would inflate the sample and collapse the
// confidence interval around what is really a single match outcome.
func TestEvaluateTakesOneObservationPerMatch(t *testing.T) {
	matches := []MatchTimeline{{MatchID: "a", EndSecond: 5400}}
	got := Evaluate(Scenario{Trigger: func(State) bool { return true }}, matches)

	if got.Occurrences != 1 {
		t.Fatalf("Occurrences = %d, want 1 — an always-true trigger is still one bet", got.Occurrences)
	}
}

func TestEvaluateHorizonLimitsTheWindow(t *testing.T) {
	matches := []MatchTimeline{{
		MatchID:   "a",
		EndSecond: 5400,
		Goals:     []Goal{{Second: 3000, Home: true}},
	}}

	// Fires once at 0'. With a 10-minute horizon the goal at 50' is outside.
	short := Evaluate(Scenario{
		Trigger:        func(s State) bool { return s.Second == 0 },
		HorizonSeconds: 600,
	}, matches)
	if short.Occurrences != 1 || short.Wins != 0 {
		t.Fatalf("short horizon: %d occurrences, %d wins; want 1 and 0", short.Occurrences, short.Wins)
	}

	// With no horizon it runs to the whistle and the goal counts.
	full := Evaluate(Scenario{
		Trigger: func(s State) bool { return s.Second == 0 },
	}, matches)
	if full.Occurrences != 1 || full.Wins != 1 {
		t.Fatalf("full horizon: %d occurrences, %d wins; want 1 and 1", full.Occurrences, full.Wins)
	}
}

func TestBreakEvenOddsAndItsBand(t *testing.T) {
	matches := []MatchTimeline{}
	// 100 matches, exactly half with a goal after the trigger instant.
	for i := 0; i < 100; i++ {
		m := MatchTimeline{MatchID: string(rune('a'+i%26)) + string(rune('a'+i/26)), EndSecond: 120}
		if i%2 == 0 {
			m.Goals = []Goal{{Second: 90, Home: true}}
		}
		matches = append(matches, m)
	}

	got := Evaluate(Scenario{Trigger: func(s State) bool { return s.Second == 0 }}, matches)

	if got.Occurrences != 100 {
		t.Fatalf("Occurrences = %d, want 100", got.Occurrences)
	}
	if math.Abs(got.HitRate-0.5) > 1e-9 {
		t.Fatalf("HitRate = %v, want 0.5", got.HitRate)
	}
	if math.Abs(got.BreakEvenOdds-2.0) > 1e-9 {
		t.Fatalf("BreakEvenOdds = %v, want 2.0", got.BreakEvenOdds)
	}
	// The pessimistic price must sit above the point estimate — that gap is
	// the entire reason this package reports an interval.
	if !(got.OddsLow < got.BreakEvenOdds && got.BreakEvenOdds < got.OddsHigh) {
		t.Fatalf("odds band %v < %v < %v is not ordered", got.OddsLow, got.BreakEvenOdds, got.OddsHigh)
	}
}

// The interval must narrow as evidence accumulates, or it is not measuring
// confidence.
func TestIntervalNarrowsWithSampleSize(t *testing.T) {
	prevWidth := math.Inf(1)
	for _, n := range []int{50, 100, 500, 5000} {
		low, high := jeffreysInterval(n/2, n)
		width := high - low
		if width >= prevWidth {
			t.Fatalf("n=%d width %.4f did not narrow versus previous %.4f", n, width, prevWidth)
		}
		prevWidth = width
	}
}

// Cross-checked against scipy: the Wilson interval for 25/50 is
// approximately 36.6% to 63.4%, so the break-even odds run 1.58 to 2.73.
func TestIntervalMatchesKnownValues(t *testing.T) {
	low, high := jeffreysInterval(25, 50)
	if math.Abs(low-0.366) > 0.005 || math.Abs(high-0.634) > 0.005 {
		t.Fatalf("interval for 25/50 = [%.3f, %.3f], want approximately [0.366, 0.634]", low, high)
	}

	if oddsHigh := oddsFor(low); math.Abs(oddsHigh-2.73) > 0.05 {
		t.Errorf("pessimistic odds = %.2f, want approximately 2.73", oddsHigh)
	}
}

// A rate of 0 or 1 must not produce a bound outside [0,1], which is where the
// naive normal-approximation interval breaks.
func TestIntervalStaysAProbabilityAtTheExtremes(t *testing.T) {
	for _, tc := range []struct{ wins, n int }{{0, 10}, {10, 10}, {0, 1}, {1, 1}} {
		low, high := jeffreysInterval(tc.wins, tc.n)
		if low < 0 || high > 1 || low > high {
			t.Errorf("%d/%d gave [%v, %v]", tc.wins, tc.n, low, high)
		}
	}
}

func TestLongestLossStreakIsObserved(t *testing.T) {
	occurrences := []Occurrence{
		{MatchID: "a", Second: 1, Won: false},
		{MatchID: "a", Second: 2, Won: false},
		{MatchID: "a", Second: 3, Won: true},
		{MatchID: "a", Second: 4, Won: false},
		{MatchID: "a", Second: 5, Won: false},
		{MatchID: "a", Second: 6, Won: false},
		{MatchID: "a", Second: 7, Won: true},
	}
	if got := longestLossStreak(occurrences); got != 3 {
		t.Fatalf("longestLossStreak = %d, want 3", got)
	}
}

func TestStakeFractionGuardsAgainstAnUnbeatenScenario(t *testing.T) {
	// 20% drawdown over a 10-loss run is 2% per bet.
	if got := StakeFraction(0.20, 10); math.Abs(got-0.02) > 1e-9 {
		t.Errorf("StakeFraction = %v, want 0.02", got)
	}
	// Never having lost is not proof of never losing.
	if got := StakeFraction(0.20, 0); got != 0.20 {
		t.Errorf("StakeFraction with no observed losses = %v, want it to assume one loss", got)
	}
}

func TestMinimumSampleForRisesAsEdgeShrinks(t *testing.T) {
	wide := MinimumSampleFor(0.60, 2.0)   // 10pp of edge
	narrow := MinimumSampleFor(0.53, 2.0) // 3pp of edge
	if wide <= 0 || narrow <= 0 {
		t.Fatalf("expected both to be establishable, got %d and %d", wide, narrow)
	}
	if narrow <= wide {
		t.Fatalf("a 3pp edge needs %d samples but a 10pp edge needs %d — should be the other way round", narrow, wide)
	}
	// No edge at all can never be established.
	if got := MinimumSampleFor(0.50, 2.0); got != -1 {
		t.Errorf("MinimumSampleFor with no edge = %d, want -1", got)
	}
}

func TestVerdictSaysSomethingUsableWhenNothingFired(t *testing.T) {
	got := Evaluate(Scenario{Trigger: func(State) bool { return false }}, handMadeMatches())
	if got.Occurrences != 0 {
		t.Fatalf("Occurrences = %d, want 0", got.Occurrences)
	}
	if got.Verdict() == "" {
		t.Error("Verdict should explain that nothing was observed")
	}
}
