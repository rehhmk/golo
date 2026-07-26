package providers

import (
	"context"

	"github.com/enzotriches/golo/internal/domain"
)

type Provider interface {
	Name() string
	ListLiveMatches(ctx context.Context) ([]domain.Match, error)
	FetchEventsSince(ctx context.Context, matchID string, lastEventID string) ([]domain.MatchEvent, error)

	// Snapshot returns the provider's authoritative point-in-time view of a
	// match — clock, status, score and cumulative statistics — as observed by
	// the most recent ListLiveMatches call. It performs no I/O.
	//
	// The bool is false when the provider publishes no such view (a purely
	// event-driven feed, or a match not seen in the last poll), in which case
	// state advances from events alone.
	Snapshot(matchID string) (domain.LiveSnapshot, bool)

	Health(ctx context.Context) domain.ProviderHealth
}
