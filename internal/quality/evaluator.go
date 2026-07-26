package quality

import (
	"math"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

type Evaluator struct{}

func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// EvaluateDataQuality computes a 0.0 - 1.0 quality score for a MatchState.
func (e *Evaluator) EvaluateDataQuality(state domain.MatchState) float64 {
	score := 1.0

	// 1. Penalize feed lag
	if state.FeedLagMs > 60000 { // > 60s
		score -= 0.4
	} else if state.FeedLagMs > 15000 { // > 15s
		score -= 0.2
	} else if state.FeedLagMs > 5000 { // > 5s
		score -= 0.1
	}

	// 2. Penalize stale provider updates
	if !state.ProviderUpdatedAt.IsZero() {
		timeSinceUpdate := time.Since(state.ProviderUpdatedAt).Seconds()
		if timeSinceUpdate > 120 {
			score -= 0.3
		} else if timeSinceUpdate > 60 {
			score -= 0.15
		}
	}

	// 3. Status penalty
	if state.Status == domain.MatchStatusStale {
		score -= 0.3
	} else if state.Status == domain.MatchStatusError {
		score -= 0.5
	}

	return math.Max(0.0, math.Min(1.0, score))
}
