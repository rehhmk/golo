package quality

import (
	"testing"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

func TestEvaluator_EvaluateDataQuality(t *testing.T) {
	eval := NewEvaluator()

	stateHigh := domain.MatchState{
		FeedLagMs:         2000,
		ProviderUpdatedAt: time.Now(),
		Status:            domain.MatchStatusLive,
	}

	qHigh := eval.EvaluateDataQuality(stateHigh)
	if qHigh != 1.0 {
		t.Errorf("expected 1.0 quality score for fresh state, got %f", qHigh)
	}

	stateLag := domain.MatchState{
		FeedLagMs:         20000, // 20s lag
		ProviderUpdatedAt: time.Now(),
		Status:            domain.MatchStatusLive,
	}

	qLag := eval.EvaluateDataQuality(stateLag)
	if qLag >= 1.0 {
		t.Errorf("expected degraded quality score for laggy state, got %f", qLag)
	}
}
