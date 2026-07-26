package mock

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

type MockProvider struct {
	mu           sync.Mutex
	matches      []domain.Match
	matchClocks  map[string]int
	lastEventIDs map[string]int
	rand         *rand.Rand
}

func NewMockProvider() *MockProvider {
	now := time.Now()
	m1 := domain.Match{
		ID:              "live_match_101",
		Provider:        "mock_feed",
		ProviderMatchID: "mock_101",
		CompetitionID:   "Premier League",
		HomeTeamID:      "Arsenal",
		AwayTeamID:      "Chelsea",
		ScheduledAt:     now.Add(-60 * time.Minute),
		Status:          domain.MatchStatusLive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	m2 := domain.Match{
		ID:              "live_match_102",
		Provider:        "mock_feed",
		ProviderMatchID: "mock_102",
		CompetitionID:   "Brasileirão Série A",
		HomeTeamID:      "Flamengo",
		AwayTeamID:      "Palmeiras",
		ScheduledAt:     now.Add(-35 * time.Minute),
		Status:          domain.MatchStatusLive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	m3 := domain.Match{
		ID:              "live_match_103",
		Provider:        "mock_feed",
		ProviderMatchID: "mock_103",
		CompetitionID:   "UEFA Champions League",
		HomeTeamID:      "Real Madrid",
		AwayTeamID:      "Manchester City",
		ScheduledAt:     now.Add(-75 * time.Minute),
		Status:          domain.MatchStatusLive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	return &MockProvider{
		matches: []domain.Match{m1, m2, m3},
		matchClocks: map[string]int{
			"live_match_101": 3600, // 60th min
			"live_match_102": 2100, // 35th min
			"live_match_103": 4350, // 72.5 min
		},
		lastEventIDs: make(map[string]int),
		rand:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (m *MockProvider) Name() string {
	return "mock_feed"
}

func (m *MockProvider) ListLiveMatches(ctx context.Context) ([]domain.Match, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.matches, nil
}

func (m *MockProvider) FetchEventsSince(ctx context.Context, matchID string, lastEventID string) ([]domain.MatchEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clock, ok := m.matchClocks[matchID]
	if !ok {
		return nil, fmt.Errorf("match %s not found", matchID)
	}

	// Advance match clock by 5-15 seconds per fetch cycle
	clock += 5 + m.rand.Intn(10)
	m.matchClocks[matchID] = clock

	// Find team IDs
	var homeTeam, awayTeam string
	for _, match := range m.matches {
		if match.ID == matchID {
			homeTeam = match.HomeTeamID
			awayTeam = match.AwayTeamID
			break
		}
	}

	m.lastEventIDs[matchID]++
	eventSeq := m.lastEventIDs[matchID]

	now := time.Now()

	// Random event selector (shot, corner, foul, dangerous attack, or goal)
	roll := m.rand.Float64()
	var evType domain.EventType
	var teamID string
	var xg *float64

	if roll < 0.35 {
		evType = domain.EventDangerousAttack
		if m.rand.Float64() < 0.5 {
			teamID = homeTeam
		} else {
			teamID = awayTeam
		}
	} else if roll < 0.60 {
		evType = domain.EventShot
		val := 0.05 + m.rand.Float64()*0.15
		xg = &val
		if m.rand.Float64() < 0.5 {
			teamID = homeTeam
		} else {
			teamID = awayTeam
		}
	} else if roll < 0.75 {
		evType = domain.EventShotOnTarget
		val := 0.15 + m.rand.Float64()*0.35
		xg = &val
		if m.rand.Float64() < 0.55 {
			teamID = homeTeam
		} else {
			teamID = awayTeam
		}
	} else if roll < 0.90 {
		evType = domain.EventCorner
		if m.rand.Float64() < 0.5 {
			teamID = homeTeam
		} else {
			teamID = awayTeam
		}
	} else if roll < 0.97 {
		evType = domain.EventFoul
		if m.rand.Float64() < 0.5 {
			teamID = homeTeam
		} else {
			teamID = awayTeam
		}
	} else {
		evType = domain.EventGoal
		val := 0.65 + m.rand.Float64()*0.30
		xg = &val
		if m.rand.Float64() < 0.5 {
			teamID = homeTeam
		} else {
			teamID = awayTeam
		}
	}

	ev := domain.MatchEvent{
		EventID:         fmt.Sprintf("mock_ev_%s_%d", matchID, eventSeq),
		MatchID:         matchID,
		Provider:        m.Name(),
		ProviderEventID: fmt.Sprintf("pev_%d", eventSeq),
		EventType:       evType,
		TeamID:          &teamID,
		MatchSecond:     clock,
		Period:          1,
		ProviderTime:    now,
		ReceivedAt:      now,
		ProcessedAt:     now,
		XG:              xg,
		SchemaVersion:   1,
	}

	if clock >= 2700 && clock < 2730 {
		ev.Period = 1
	} else if clock >= 2730 {
		ev.Period = 2
	}

	return []domain.MatchEvent{ev}, nil
}

func (m *MockProvider) Health(ctx context.Context) domain.ProviderHealth {
	return domain.ProviderHealth{
		Provider:  m.Name(),
		IsHealthy: true,
	}
}
