// Package scenario measures how often a specific in-match situation is
// followed by another goal, across historical matches.
//
// It exists to answer one question honestly: at what odds would betting this
// situation actually be profitable, and is the sample large enough to trust
// the answer?
//
// The method is the standard one — define a specific trigger, observe it many
// times without acting, compute the hit rate, and derive the minimum odds as
// 1/rate. What that method usually omits is the uncertainty. Fifty
// observations at a 50% hit rate put the true rate somewhere between 36.5%
// and 63.5%, which puts the true break-even odds anywhere between 1.57 and
// 2.74. Betting at 2.10 on the belief that break-even is 2.00 can be a
// guaranteed loss. Every result this package produces therefore carries an
// interval, and the verdict is driven by the pessimistic end of it.
package scenario

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// MatchTimeline is one historical match reduced to what a scenario needs:
// when goals happened, when red cards happened, and when the match ended.
type MatchTimeline struct {
	MatchID       string
	CompetitionID string
	Season        string
	StartingAt    time.Time
	EndSecond     int
	Goals         []Goal
	RedCards      []RedCard
}

type Goal struct {
	Second int
	Home   bool
}

type RedCard struct {
	Second int
	Home   bool
}

// State is the match situation at one instant, which a trigger inspects.
type State struct {
	Second     int
	ScoreHome  int
	ScoreAway  int
	RedHome    int
	RedAway    int
	Remaining  int
	GoalsSoFar int
}

// ScoreDiff is home minus away.
func (s State) ScoreDiff() int { return s.ScoreHome - s.ScoreAway }

// AbsScoreDiff is the size of the gap regardless of who leads.
func (s State) AbsScoreDiff() int {
	if d := s.ScoreDiff(); d < 0 {
		return -d
	} else {
		return d
	}
}

// Trigger decides whether a scenario applies at a given instant.
type Trigger func(State) bool

// Scenario is a named trigger plus the horizon over which the bet settles.
//
// HorizonSeconds of 0 means "until the final whistle", which is what the
// live over markets reduce to: at 1-1, "over 2.5 goals" is exactly the
// question "will there be one more goal before the end?".
type Scenario struct {
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Trigger         Trigger `json:"-"`
	HorizonSeconds  int     `json:"horizonSeconds"`
	AdditionalGoals int     `json:"additionalGoals"`
}

// Occurrence is one time a scenario fired, and what followed.
type Occurrence struct {
	MatchID string
	Second  int
	Won     bool
}

// Result is everything measured for one scenario.
type Result struct {
	Scenario    Scenario `json:"scenario"`
	Occurrences int      `json:"occurrences"`
	Wins        int      `json:"wins"`

	HitRate float64 `json:"hitRate"`
	// RateLow and RateHigh bound the true hit rate at 95% confidence.
	RateLow  float64 `json:"rateLow"`
	RateHigh float64 `json:"rateHigh"`

	// BreakEvenOdds is 1/HitRate: below this price the bet loses money over
	// time. OddsHigh is the pessimistic bound — derived from RateLow, it is
	// the price you would need if the true rate sits at the unlucky end of
	// the interval, and it is the number a cautious reader should use.
	BreakEvenOdds float64 `json:"breakEvenOdds"`
	OddsLow       float64 `json:"oddsLow"`
	OddsHigh      float64 `json:"oddsHigh"`

	// LongestLossStreak is observed, not estimated from the hit rate.
	LongestLossStreak int `json:"longestLossStreak"`

	// MatchCount equals Occurrences by construction — one observation per
	// match — and is kept explicit so the reader can see that the sample size
	// is a count of independent matches rather than of trigger firings.
	MatchCount int `json:"matchCount"`
}

// Evaluate runs a scenario across a set of match timelines, recording one
// observation per match: the first minute at which the trigger fires.
//
// Taking only the first firing is both the statistically sound choice and the
// behaviourally accurate one. A condition like "leading by two after the 70th
// minute" holds for twenty consecutive minutes, and counting those as twenty
// observations would inflate the sample twenty-fold while adding almost no
// information — the same match, the same scoreline, overwhelmingly the same
// outcome. Doing that produced a 2-percentage-point confidence interval off
// 255 matches, which is false precision of exactly the kind this package
// exists to prevent. It also matches how the bet is actually placed: once,
// when the situation appears, not once a minute while it persists.
func Evaluate(sc Scenario, matches []MatchTimeline) Result {
	const step = 60

	result := Result{Scenario: sc}
	var occurrences []Occurrence

	for _, match := range matches {
		for second := 0; second < match.EndSecond; second += step {
			state := stateAt(match, second)
			if !sc.Trigger(state) {
				continue
			}

			until := match.EndSecond
			if sc.HorizonSeconds > 0 {
				until = second + sc.HorizonSeconds
				if until > match.EndSecond {
					until = match.EndSecond
				}
			}
			if until <= second {
				continue
			}

			occurrences = append(occurrences, Occurrence{
				MatchID: match.MatchID,
				Second:  second,
				Won:     goalsBetween(match.Goals, second, until) >= max(1, sc.AdditionalGoals),
			})
			break // one observation per match
		}
	}

	result.Occurrences = len(occurrences)
	result.MatchCount = len(occurrences)
	for _, o := range occurrences {
		if o.Won {
			result.Wins++
		}
	}

	if result.Occurrences == 0 {
		return result
	}

	result.HitRate = float64(result.Wins) / float64(result.Occurrences)
	result.RateLow, result.RateHigh = wilsonInterval(result.Wins, result.Occurrences)
	result.BreakEvenOdds = oddsFor(result.HitRate)
	// A lower rate demands a higher price, so the bounds cross over.
	result.OddsLow = oddsFor(result.RateHigh)
	result.OddsHigh = oddsFor(result.RateLow)
	result.LongestLossStreak = longestLossStreak(occurrences)

	return result
}

// ResultFromOutcomes constructs the same auditable summary used by historical
// backtests from already-settled prospective outcomes.
func ResultFromOutcomes(sc Scenario, outcomes []Occurrence) Result {
	result := Result{
		Scenario: sc, Occurrences: len(outcomes), MatchCount: len(outcomes),
	}
	for _, outcome := range outcomes {
		if outcome.Won {
			result.Wins++
		}
	}
	if result.Occurrences == 0 {
		return result
	}
	result.HitRate = float64(result.Wins) / float64(result.Occurrences)
	result.RateLow, result.RateHigh = wilsonInterval(result.Wins, result.Occurrences)
	result.BreakEvenOdds = oddsFor(result.HitRate)
	result.OddsLow = oddsFor(result.RateHigh)
	result.OddsHigh = oddsFor(result.RateLow)
	result.LongestLossStreak = longestLossStreak(outcomes)
	return result
}

// stateAt reconstructs the match situation at a given second.
func stateAt(match MatchTimeline, second int) State {
	state := State{Second: second, Remaining: match.EndSecond - second}
	for _, g := range match.Goals {
		if g.Second > second {
			continue
		}
		if g.Home {
			state.ScoreHome++
		} else {
			state.ScoreAway++
		}
	}
	for _, r := range match.RedCards {
		if r.Second > second {
			continue
		}
		if r.Home {
			state.RedHome++
		} else {
			state.RedAway++
		}
	}
	state.GoalsSoFar = state.ScoreHome + state.ScoreAway
	return state
}

func goalsBetween(goals []Goal, from, until int) int {
	count := 0
	for _, g := range goals {
		if g.Second > from && g.Second <= until {
			count++
		}
	}
	return count
}

// longestLossStreak reports the longest observed run of losses, in the order
// the scenario actually fired. Bankroll sizing built on a guessed worst case
// is guesswork; this is what the data did.
func longestLossStreak(occurrences []Occurrence) int {
	sorted := make([]Occurrence, len(occurrences))
	copy(sorted, occurrences)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].MatchID != sorted[j].MatchID {
			return sorted[i].MatchID < sorted[j].MatchID
		}
		return sorted[i].Second < sorted[j].Second
	})

	longest, current := 0, 0
	for _, o := range sorted {
		if o.Won {
			current = 0
			continue
		}
		current++
		if current > longest {
			longest = current
		}
	}
	return longest
}

// oddsFor converts a probability into the decimal price at which a bet on it
// breaks even.
func oddsFor(rate float64) float64 {
	if rate <= 0 {
		return math.Inf(1)
	}
	return 1 / rate
}

// StakeFraction is the share of a bankroll to risk per bet, from the
// acceptable drawdown and the worst losing run seen.
//
// The usual formulation divides the drawdown you can stomach by the worst
// streak you expect. Using the observed streak rather than a round guess
// matters, and so does the guard here: a scenario that never lost yet has not
// proven it cannot, so it is treated as though it had lost once.
func StakeFraction(acceptableDrawdown float64, longestLossStreak int) float64 {
	streak := longestLossStreak
	if streak < 1 {
		streak = 1
	}
	return acceptableDrawdown / float64(streak)
}

// MinimumSampleFor reports how many observations at the given true rate are
// needed before the pessimistic bound clears a target price — that is, before
// the edge can be called real rather than lucky.
func MinimumSampleFor(trueRate, targetOdds float64) int {
	breakEven := 1 / targetOdds
	if trueRate <= breakEven {
		return -1 // no edge to establish at this price
	}
	for n := 20; n <= 200000; n += 10 {
		wins := int(math.Round(trueRate * float64(n)))
		low, _ := wilsonInterval(wins, n)
		if low > breakEven {
			return n
		}
	}
	return -1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Verdict renders the result as a sentence that leads with the uncertainty
// rather than burying it.
func (r Result) Verdict() string {
	if r.Occurrences == 0 {
		return "o cenário nunca ocorreu no histórico"
	}
	if math.IsInf(r.OddsHigh, 1) {
		return fmt.Sprintf("%d ocorrências, nenhuma vitória — sem preço que compense", r.Occurrences)
	}
	return fmt.Sprintf(
		"acerto %.1f%% em %d ocorrências (%d partidas); precisa de odd acima de %.2f, "+
			"mas a incerteza ainda admite %.2f — use %.2f até ter mais amostra",
		r.HitRate*100, r.Occurrences, r.MatchCount,
		r.BreakEvenOdds, r.OddsHigh, r.OddsHigh,
	)
}
