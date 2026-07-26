package replay

import (
	"context"

	"github.com/enzotriches/golo/internal/domain"
)

type ReplaySpeed string

const (
	Speed1x  ReplaySpeed = "1x"
	Speed10x ReplaySpeed = "10x"
	SpeedMax ReplaySpeed = "max"
)

type ReplayProvider struct {
	match    domain.Match
	events   []domain.MatchEvent
	index    int
	speed    ReplaySpeed
	isPaused bool
}

func NewReplayProvider(match domain.Match, events []domain.MatchEvent) *ReplayProvider {
	return &ReplayProvider{
		match:    match,
		events:   events,
		index:    0,
		speed:    Speed10x,
		isPaused: false,
	}
}

func (r *ReplayProvider) Name() string {
	return "replay_engine"
}

func (r *ReplayProvider) ListLiveMatches(ctx context.Context) ([]domain.Match, error) {
	return []domain.Match{r.match}, nil
}

func (r *ReplayProvider) FetchEventsSince(ctx context.Context, matchID string, lastEventID string) ([]domain.MatchEvent, error) {
	if r.isPaused || r.index >= len(r.events) {
		return []domain.MatchEvent{}, nil
	}

	// In Max speed, yield all remaining events at once or batch of 5
	batchSize := 5
	if r.speed == SpeedMax {
		batchSize = len(r.events) - r.index
	}

	endIdx := r.index + batchSize
	if endIdx > len(r.events) {
		endIdx = len(r.events)
	}

	batch := r.events[r.index:endIdx]
	r.index = endIdx

	return batch, nil
}

func (r *ReplayProvider) SetSpeed(speed ReplaySpeed) {
	r.speed = speed
}

func (r *ReplayProvider) SetPaused(paused bool) {
	r.isPaused = paused
}

func (r *ReplayProvider) Reset() {
	r.index = 0
	r.isPaused = false
}

func (r *ReplayProvider) Health(ctx context.Context) domain.ProviderHealth {
	return domain.ProviderHealth{
		Provider:  r.Name(),
		IsHealthy: true,
	}
}
