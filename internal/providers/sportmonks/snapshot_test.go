package sportmonks

import (
	"testing"

	"github.com/enzotriches/golo/internal/domain"
)

func intPtr(v int) *int { return &v }

func TestClockSecondsUsesTickingPeriod(t *testing.T) {
	// A 2nd-half period reports elapsed match time inclusive of counts_from,
	// so 77:52 in the second half is 77 minutes into the match, not 32.
	periods := []periodDTO{
		{TypeID: 1, CountsFrom: 0, SortOrder: 1, Ticking: false, Minutes: intPtr(50), Seconds: intPtr(16)},
		{TypeID: 2, CountsFrom: 45, SortOrder: 2, Ticking: true, Minutes: intPtr(77), Seconds: intPtr(52)},
	}

	got := clockSeconds(periods, stateInplay2ndHalf)
	if want := 77*60 + 52; got != want {
		t.Fatalf("clockSeconds = %d, want %d", got, want)
	}
}

func TestClockSecondsFallsBackToFurthestPeriodWhenNothingTicking(t *testing.T) {
	// Half time: no period is ticking, but the match is 50 minutes in and
	// must not report zero.
	periods := []periodDTO{
		{TypeID: 1, SortOrder: 1, Ticking: false, Minutes: intPtr(50), Seconds: intPtr(16)},
	}

	got := clockSeconds(periods, stateHalfTime)
	if want := 50*60 + 16; got != want {
		t.Fatalf("clockSeconds = %d, want %d", got, want)
	}
}

func TestClockSecondsHalfTimeWithoutPeriodsIsAtLeastFortyFive(t *testing.T) {
	got := clockSeconds(nil, stateHalfTime)
	if want := 45 * 60; got != want {
		t.Fatalf("clockSeconds = %d, want %d", got, want)
	}
}

func TestMapStatus(t *testing.T) {
	cases := []struct {
		name    string
		stateID int
		want    domain.MatchStatus
	}{
		{"1st half", stateInplay1stHalf, domain.MatchStatusLive},
		{"2nd half", stateInplay2ndHalf, domain.MatchStatusLive},
		{"extra time", stateInplayET, domain.MatchStatusLive},
		{"half time", stateHalfTime, domain.MatchStatusHalfTime},
		{"not started", stateNotStarted, domain.MatchStatusScheduled},
		{"full time", stateFullTime, domain.MatchStatusFinished},
		{"postponed", statePostponed, domain.MatchStatusCancelled},
		{"unknown code", 999, domain.MatchStatusStale},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapStatus(tc.stateID); got != tc.want {
				t.Fatalf("mapStatus(%d) = %s, want %s", tc.stateID, got, tc.want)
			}
		})
	}
}

// A fixture that hasn't kicked off must never be reported as live — the
// inplay endpoint returns these alongside genuinely running matches.
func TestNotStartedIsNotLive(t *testing.T) {
	if mapStatus(stateNotStarted) == domain.MatchStatusLive {
		t.Fatal("a Not Started fixture was mapped to LIVE")
	}
}

func TestCurrentScoreIgnoresPerHalfEntries(t *testing.T) {
	scores := []scoreDTO{
		{ParticipantID: 478, Description: "1ST_HALF"},
		{ParticipantID: 254172, Description: "1ST_HALF"},
		{ParticipantID: 478, Description: "CURRENT"},
		{ParticipantID: 254172, Description: "CURRENT"},
	}
	scores[0].Score.Goals = 2
	scores[1].Score.Goals = 1
	scores[2].Score.Goals = 3
	scores[3].Score.Goals = 1

	got := currentScore(scores, "478", "254172")
	if got.Home != 3 || got.Away != 1 {
		t.Fatalf("currentScore = %+v, want {Home:3 Away:1}", got)
	}
}

func TestStatTotals(t *testing.T) {
	stats := []statisticDTO{
		newStat(478, "SHOTS_ON_TARGET", 5),
		newStat(478, "SHOTS_OFF_TARGET", 4),
		newStat(478, "SHOTS_BLOCKED", 2),
		newStat(478, "CORNERS", 6),
		newStat(478, "DANGEROUS_ATTACKS", 53),
		newStat(254172, "SHOTS_ON_TARGET", 1),
		newStat(254172, "FOULS", 8),
		newStat(478, "LONG_PASSES", 41), // not consumed by the feature engine
	}

	home, away, hasStats := statTotals(stats, "478", "254172")
	if !hasStats {
		t.Fatal("hasStats = false, want true")
	}
	if home.ShotsOnTarget != 5 || home.ShotsOffTarget != 4 || home.ShotsBlocked != 2 {
		t.Fatalf("home shots = %+v", home)
	}
	if home.Corners != 6 || home.DangerousAttacks != 53 {
		t.Fatalf("home set pieces/attacks = %+v", home)
	}
	if away.ShotsOnTarget != 1 || away.Fouls != 8 {
		t.Fatalf("away = %+v", away)
	}
}

func TestStatTotalsReportsAbsenceRatherThanZeroes(t *testing.T) {
	_, _, hasStats := statTotals(nil, "478", "254172")
	if hasStats {
		t.Fatal("hasStats = true for an empty statistics list, want false")
	}
}

func newStat(participantID int64, developerName string, value float64) statisticDTO {
	s := statisticDTO{ParticipantID: participantID}
	s.Type.DeveloperName = developerName
	s.Data.Value = value
	return s
}

func TestRateLimitCooldownSuppressesRequestsAndBacksOff(t *testing.T) {
	p := New("test-key", "https://example.invalid", nil)

	if _, ok := p.inCooldown(); ok {
		t.Fatal("a fresh provider is in cooldown")
	}

	p.enterCooldown()
	wait, ok := p.inCooldown()
	if !ok {
		t.Fatal("expected cooldown after a rate limit")
	}
	if wait > minRateLimitCooldown {
		t.Fatalf("first cooldown = %s, want at most %s", wait, minRateLimitCooldown)
	}

	// Repeated limits escalate the backoff.
	p.enterCooldown()
	if p.cooldown != 2*minRateLimitCooldown {
		t.Fatalf("second cooldown = %s, want %s", p.cooldown, 2*minRateLimitCooldown)
	}

	// A success means the quota refilled — clear the backoff entirely.
	p.recordSuccess()
	if _, ok := p.inCooldown(); ok {
		t.Fatal("cooldown survived a successful call")
	}
	if p.cooldown != 0 {
		t.Fatalf("cooldown = %s after success, want 0", p.cooldown)
	}
}

func TestRateLimitCooldownIsBounded(t *testing.T) {
	p := New("test-key", "https://example.invalid", nil)
	for i := 0; i < 20; i++ {
		p.enterCooldown()
	}
	if p.cooldown != maxRateLimitCooldown {
		t.Fatalf("cooldown = %s, want it capped at %s", p.cooldown, maxRateLimitCooldown)
	}
}
