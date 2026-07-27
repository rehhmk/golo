package scenario

import (
	"fmt"
	"strings"
)

// Definition is a scenario expressed as data rather than as a Go closure, so
// that a user can build one in the interface and the server can run it.
//
// Every condition is optional and they combine with AND. That is deliberate:
// the whole premise of scenario betting is that a *narrow* situation departs
// from the base rate of football, and OR-ing conditions together widens a
// scenario until it measures nothing but the base rate again.
type Definition struct {
	Name string `json:"name"`

	// FromMinute and UntilMinute bound when the scenario may fire.
	FromMinute  *int `json:"fromMinute,omitempty"`
	UntilMinute *int `json:"untilMinute,omitempty"`

	// RemainingAtMost fires only inside the closing minutes of a match.
	RemainingAtMost *int `json:"remainingAtMost,omitempty"`

	// ScoreDiff conditions read the size of the gap, not who is ahead.
	ScoreDiffExactly *int `json:"scoreDiffExactly,omitempty"`
	ScoreDiffAtLeast *int `json:"scoreDiffAtLeast,omitempty"`

	GoalsExactly *int `json:"goalsExactly,omitempty"`
	GoalsAtLeast *int `json:"goalsAtLeast,omitempty"`

	// RedCardsAtLeast counts dismissals on either side.
	RedCardsAtLeast *int `json:"redCardsAtLeast,omitempty"`

	// CompetitionIDs restricts the scenario to particular leagues. Empty means
	// every competition in the dataset.
	CompetitionIDs []string `json:"competitionIds,omitempty"`

	// HorizonMinutes of 0 means "until the final whistle", which is what the
	// live over markets reduce to: at 1-1, "over 2.5" is exactly the question
	// "will there be one more goal before the end?".
	HorizonMinutes int `json:"horizonMinutes"`

	// TargetOdds is the price the user can actually get, and the number every
	// verdict is judged against. A scenario is never "good" in the abstract —
	// it is good or bad relative to a price.
	TargetOdds float64 `json:"targetOdds"`
}

// Validate rejects definitions that cannot produce a meaningful measurement.
func (d Definition) Validate() error {
	if d.TargetOdds <= 1.0 {
		return fmt.Errorf("a odd alvo precisa ser maior que 1,00")
	}
	if d.HorizonMinutes < 0 {
		return fmt.Errorf("o horizonte não pode ser negativo")
	}
	if d.FromMinute != nil && d.UntilMinute != nil && *d.FromMinute > *d.UntilMinute {
		return fmt.Errorf("o minuto inicial vem depois do final")
	}
	if !d.hasAnyCondition() {
		// An unconditioned scenario fires on every match at minute zero and
		// measures the base rate of football. That is a useful comparison, but
		// it is not a strategy, and returning it as one invites reading the
		// base rate as an edge.
		return fmt.Errorf("defina ao menos uma condição — sem condição o cenário mede apenas a taxa base")
	}
	return nil
}

func (d Definition) hasAnyCondition() bool {
	return d.FromMinute != nil || d.UntilMinute != nil || d.RemainingAtMost != nil ||
		d.ScoreDiffExactly != nil || d.ScoreDiffAtLeast != nil ||
		d.GoalsExactly != nil || d.GoalsAtLeast != nil ||
		d.RedCardsAtLeast != nil
}

// Compile turns the definition into a runnable Scenario.
func (d Definition) Compile() Scenario {
	def := d // captured by value so later edits cannot change a running scenario
	return Scenario{
		Name:           d.Name,
		Description:    d.Describe(),
		HorizonSeconds: d.HorizonMinutes * 60,
		Trigger: func(s State) bool {
			if def.FromMinute != nil && s.Second < *def.FromMinute*60 {
				return false
			}
			if def.UntilMinute != nil && s.Second > *def.UntilMinute*60 {
				return false
			}
			if def.RemainingAtMost != nil && s.Remaining > *def.RemainingAtMost*60 {
				return false
			}
			if def.ScoreDiffExactly != nil && s.AbsScoreDiff() != *def.ScoreDiffExactly {
				return false
			}
			if def.ScoreDiffAtLeast != nil && s.AbsScoreDiff() < *def.ScoreDiffAtLeast {
				return false
			}
			if def.GoalsExactly != nil && s.GoalsSoFar != *def.GoalsExactly {
				return false
			}
			if def.GoalsAtLeast != nil && s.GoalsSoFar < *def.GoalsAtLeast {
				return false
			}
			if def.RedCardsAtLeast != nil && s.RedHome+s.RedAway < *def.RedCardsAtLeast {
				return false
			}
			return true
		},
	}
}

// Describe renders the definition as a sentence, so the interface and the
// stored record always agree on what was measured.
func (d Definition) Describe() string {
	var parts []string

	if d.FromMinute != nil {
		parts = append(parts, fmt.Sprintf("a partir do minuto %d", *d.FromMinute))
	}
	if d.UntilMinute != nil {
		parts = append(parts, fmt.Sprintf("até o minuto %d", *d.UntilMinute))
	}
	if d.RemainingAtMost != nil {
		parts = append(parts, fmt.Sprintf("nos últimos %d minutos", *d.RemainingAtMost))
	}
	switch {
	case d.ScoreDiffExactly != nil && *d.ScoreDiffExactly == 0:
		parts = append(parts, "jogo empatado")
	case d.ScoreDiffExactly != nil:
		parts = append(parts, fmt.Sprintf("diferença de exatamente %d gol(s)", *d.ScoreDiffExactly))
	}
	if d.ScoreDiffAtLeast != nil {
		parts = append(parts, fmt.Sprintf("alguém vencendo por %d ou mais", *d.ScoreDiffAtLeast))
	}
	if d.GoalsExactly != nil {
		parts = append(parts, fmt.Sprintf("exatamente %d gol(s) na partida", *d.GoalsExactly))
	}
	if d.GoalsAtLeast != nil {
		parts = append(parts, fmt.Sprintf("%d ou mais gols na partida", *d.GoalsAtLeast))
	}
	if d.RedCardsAtLeast != nil {
		parts = append(parts, fmt.Sprintf("%d ou mais expulsões", *d.RedCardsAtLeast))
	}

	when := strings.Join(parts, ", ")
	if when == "" {
		when = "qualquer instante"
	}

	if d.HorizonMinutes > 0 {
		return fmt.Sprintf("%s → sai gol nos %d minutos seguintes", when, d.HorizonMinutes)
	}
	return when + " → sai mais um gol até o fim"
}

// Report is the full answer returned for one definition: what was measured,
// and what it means at the price the user can actually get.
type Report struct {
	Definition  Definition `json:"definition"`
	Description string     `json:"description"`

	Occurrences int     `json:"occurrences"`
	Wins        int     `json:"wins"`
	HitRate     float64 `json:"hitRate"`
	RateLow     float64 `json:"rateLow"`
	RateHigh    float64 `json:"rateHigh"`

	BreakEvenOdds float64 `json:"breakEvenOdds"`
	// RequiredOdds is derived from the pessimistic bound and is the number a
	// cautious reader should use: the price needed if the true rate sits at
	// the unlucky end of the interval.
	RequiredOdds float64 `json:"requiredOdds"`

	LongestLossStreak int     `json:"longestLossStreak"`
	StakeFraction     float64 `json:"stakeFraction"`

	// BaselineHitRate is the same outcome measured with no trigger at all. A
	// scenario that does not beat it is not informing anything, however good
	// its own rate looks.
	BaselineHitRate float64 `json:"baselineHitRate"`
	BeatsBaseline   bool    `json:"beatsBaseline"`

	// Worthwhile is true only when the pessimistic bound clears the price.
	Worthwhile bool `json:"worthwhile"`
	// SampleSufficient distinguishes "this does not work" from "we cannot
	// tell yet" — opposite problems with opposite remedies.
	SampleSufficient bool   `json:"sampleSufficient"`
	NeededSample     int    `json:"neededSample"`
	Verdict          string `json:"verdict"`
}

// minimumMatchesForVerdict is the sample below which no verdict is issued.
const minimumMatchesForVerdict = 100

// Run measures a definition and renders the verdict.
func Run(def Definition, matches []MatchTimeline, drawdown float64) (Report, error) {
	if err := def.Validate(); err != nil {
		return Report{}, err
	}

	scoped := matches
	if len(def.CompetitionIDs) > 0 {
		allowed := make(map[string]struct{}, len(def.CompetitionIDs))
		for _, id := range def.CompetitionIDs {
			allowed[id] = struct{}{}
		}
		scoped = make([]MatchTimeline, 0, len(matches))
		for _, m := range matches {
			if _, ok := allowed[m.CompetitionID]; ok {
				scoped = append(scoped, m)
			}
		}
	}

	result := Evaluate(def.Compile(), scoped)

	// The same outcome with no trigger, over the same matches, so the two are
	// directly comparable.
	baseline := Evaluate(Scenario{
		Name:           "baseline",
		Trigger:        func(State) bool { return true },
		HorizonSeconds: def.HorizonMinutes * 60,
	}, scoped)

	breakEven := 1 / def.TargetOdds
	report := Report{
		Definition:        def,
		Description:       def.Describe(),
		Occurrences:       result.Occurrences,
		Wins:              result.Wins,
		HitRate:           result.HitRate,
		RateLow:           result.RateLow,
		RateHigh:          result.RateHigh,
		BreakEvenOdds:     result.BreakEvenOdds,
		RequiredOdds:      result.OddsHigh,
		LongestLossStreak: result.LongestLossStreak,
		StakeFraction:     StakeFraction(drawdown, result.LongestLossStreak),
		BaselineHitRate:   baseline.HitRate,
		BeatsBaseline:     result.HitRate > baseline.HitRate,
		SampleSufficient:  result.Occurrences >= minimumMatchesForVerdict,
		Worthwhile:        result.RateLow > breakEven,
	}
	if result.Occurrences > 0 {
		report.NeededSample = MinimumSampleFor(result.HitRate, def.TargetOdds)
	}
	report.Verdict = verdictFor(report, def.TargetOdds, breakEven)
	return report, nil
}

func verdictFor(r Report, targetOdds, breakEven float64) string {
	switch {
	case r.Occurrences == 0:
		return "Este cenário nunca ocorreu no histórico. Afrouxe alguma condição."

	case !r.SampleSufficient:
		return fmt.Sprintf(
			"Amostra insuficiente: %d partidas. Abaixo de %d nenhum veredito é confiável — "+
				"o intervalo é largo demais para separar vantagem de sorte.",
			r.Occurrences, minimumMatchesForVerdict)

	case r.Worthwhile && !r.BeatsBaseline:
		return fmt.Sprintf(
			"Compensa a %.2f, mas não supera a taxa base (%.1f%% contra %.1f%%). "+
				"O cenário não está informando nada — a mesma odd valeria em qualquer partida.",
			targetOdds, r.HitRate*100, r.BaselineHitRate*100)

	case r.Worthwhile:
		return fmt.Sprintf(
			"Compensa a %.2f. Acerto de %.1f%% (mínimo provável %.1f%%) contra o equilíbrio de %.1f%%. "+
				"Arrisque %.2f%% da banca por entrada — a pior sequência observada foi de %d perdas.",
			targetOdds, r.HitRate*100, r.RateLow*100, breakEven*100,
			r.StakeFraction*100, r.LongestLossStreak)

	case r.NeededSample > 0 && r.HitRate > breakEven:
		return fmt.Sprintf(
			"Ainda não dá para afirmar. Acerto de %.1f%% supera o equilíbrio de %.1f%%, mas com %d "+
				"partidas o intervalo ainda admite %.1f%%. Seriam necessárias ~%d ocorrências.",
			r.HitRate*100, breakEven*100, r.Occurrences, r.RateLow*100, r.NeededSample)

	default:
		return fmt.Sprintf(
			"Não compensa a %.2f. Precisa de odd acima de %.2f. "+
				"A esse preço é prejuízo no longo prazo — por aritmética, não por azar.",
			targetOdds, r.RequiredOdds)
	}
}
