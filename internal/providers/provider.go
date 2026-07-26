package providers

import (
	"context"

	"github.com/enzotriches/golo/internal/domain"
)

type Provider interface {
	Name() string
	ListLiveMatches(ctx context.Context) ([]domain.Match, error)
	FetchEventsSince(ctx context.Context, matchID string, lastEventID string) ([]domain.MatchEvent, error)
	Health(ctx context.Context) domain.ProviderHealth
}
