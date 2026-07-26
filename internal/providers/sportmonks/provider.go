package sportmonks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

const startingAtLayout = "2006-01-02 15:04:05"

// SportMonks v3 event type_id values, confirmed against a real fixture
// (docs.sportmonks.com/v3 + a live API response on 2026-07-25 for fixture
// 19714015 "AGF vs Brøndby IF"): 14=goal, 16=penalty scored ("addition":
// "1st Penalty", confirmed by the score incrementing in "result"),
// 18=substitution, 19=yellow card. 20 (red) and 21 (yellow-red) are
// confirmed by SportMonks' own eventTypes filter docs but weren't present
// in this particular match. Anything else (e.g. type_id 10, "Penalty
// awarded" — not itself a goal) maps to EventUnknown; the raw event JSON
// is preserved in MatchEvent.Attributes so nothing is silently lost.
// Own goals and missed penalties don't have a confirmed separate type_id
// yet — refine this mapping further once a match containing one is seen.
const (
	typeIDGoal          = 14
	typeIDPenaltyScored = 16
	typeIDSubstitution  = 18
	typeIDYellowCard    = 19
	typeIDRedCard       = 20
	typeIDYellowRedCard = 21
)

type fixtureDTO struct {
	ID           int64            `json:"id"`
	LeagueID     int64            `json:"league_id"`
	SeasonID     int64            `json:"season_id"`
	StateID      int              `json:"state_id"`
	Name         string           `json:"name"`
	StartingAt   string           `json:"starting_at"`
	Participants []participantDTO `json:"participants"`
	Events       []eventDTO       `json:"events"`
}

type participantDTO struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Meta struct {
		Location string `json:"location"` // "home" or "away"
	} `json:"meta"`
}

type eventDTO struct {
	ID              int64  `json:"id"`
	TypeID          int    `json:"type_id"`
	ParticipantID   *int64 `json:"participant_id"`
	PlayerID        *int64 `json:"player_id"`
	PlayerName      string `json:"player_name"`
	RelatedPlayerID *int64 `json:"related_player_id"`
	Minute          int    `json:"minute"`
	ExtraMinute     *int   `json:"extra_minute"`
	Result          string `json:"result"`
	Info            string `json:"info"`
}

// Provider implements providers.Provider against the SportMonks Football API v3.
type Provider struct {
	client *client

	// priorityRank maps a competition ID (SportMonks league_id, as a string)
	// to its position in the configured priority list; lower is higher
	// priority. Competitions absent from the map sort after all ranked ones,
	// in whatever order the API returned them.
	priorityRank map[string]int

	mu       sync.Mutex
	fixtures map[string]int64 // internal matchID -> SportMonks fixture ID

	lastSuccessAt time.Time
	lastErrorAt   time.Time
	errorCount    int
	lastErrMsg    string
}

// New creates a SportMonks provider. apiKey must never be exposed to the frontend;
// it is only ever read from server-side config (internal/config).
//
// priorityCompetitions reorders (does not filter) ListLiveMatches results so
// higher-priority competitions surface first — SportMonks itself scopes
// which leagues are fetchable at the account/plan level, not via this list.
func New(apiKey, baseURL string, priorityCompetitions []string) *Provider {
	rank := make(map[string]int, len(priorityCompetitions))
	for i, id := range priorityCompetitions {
		rank[id] = i
	}

	return &Provider{
		client:       newClient(apiKey, baseURL),
		priorityRank: rank,
		fixtures:     make(map[string]int64),
	}
}

func (p *Provider) Name() string {
	return "sportmonks"
}

func (p *Provider) ListLiveMatches(ctx context.Context) ([]domain.Match, error) {
	query := url.Values{}
	query.Set("include", "participants")

	data, err := p.client.get(ctx, "/livescores/inplay", query)
	if err != nil {
		p.recordError(err)
		return nil, err
	}

	var fixtures []fixtureDTO
	if err := unmarshalArray(data, &fixtures); err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("sportmonks: decoding livescores: %w", err)
	}

	now := time.Now()
	matches := make([]domain.Match, 0, len(fixtures))

	p.mu.Lock()
	for _, f := range fixtures {
		matchID := internalMatchID(f.ID)
		p.fixtures[matchID] = f.ID

		home, away := participantIDs(f.Participants)
		scheduledAt, _ := time.Parse(startingAtLayout, f.StartingAt)

		matches = append(matches, domain.Match{
			ID:              matchID,
			Provider:        p.Name(),
			ProviderMatchID: strconv.FormatInt(f.ID, 10),
			CompetitionID:   strconv.FormatInt(f.LeagueID, 10),
			SeasonID:        strconv.FormatInt(f.SeasonID, 10),
			HomeTeamID:      home,
			AwayTeamID:      away,
			ScheduledAt:     scheduledAt,
			// The inplay endpoint only ever returns matches currently in
			// progress, so every fixture it returns is LIVE by construction.
			Status:    domain.MatchStatusLive,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	p.mu.Unlock()

	p.sortByPriority(matches)

	p.recordSuccess()
	return matches, nil
}

// sortByPriority stable-sorts matches so configured priority competitions
// come first, in configured order; everything else keeps its relative order.
func (p *Provider) sortByPriority(matches []domain.Match) {
	if len(p.priorityRank) == 0 {
		return
	}
	rankOf := func(competitionID string) int {
		if r, ok := p.priorityRank[competitionID]; ok {
			return r
		}
		return len(p.priorityRank) // unranked: after all ranked competitions
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return rankOf(matches[i].CompetitionID) < rankOf(matches[j].CompetitionID)
	})
}

// FetchEventsSince fetches the full event list for a fixture and returns only
// events with a SportMonks event ID greater than lastEventID. SportMonks has
// no native "since" cursor for events, so this fetches the whole fixture each
// time; the eventstore's SaveEvents already dedupes by event_id (INSERT OR
// IGNORE), so returning already-seen events again is harmless — the id filter
// here is purely an optimization to reduce the outbound payload size.
func (p *Provider) FetchEventsSince(ctx context.Context, matchID string, lastEventID string) ([]domain.MatchEvent, error) {
	p.mu.Lock()
	fixtureID, ok := p.fixtures[matchID]
	p.mu.Unlock()
	if !ok {
		var err error
		fixtureID, err = fixtureIDFromMatchID(matchID)
		if err != nil {
			return nil, fmt.Errorf("sportmonks: unknown match %s: %w", matchID, err)
		}
	}

	query := url.Values{}
	query.Set("include", "events;participants")

	data, err := p.client.get(ctx, fmt.Sprintf("/fixtures/%d", fixtureID), query)
	if err != nil {
		p.recordError(err)
		return nil, err
	}

	var fixture fixtureDTO
	if err := unmarshalObject(data, &fixture); err != nil {
		p.recordError(err)
		return nil, fmt.Errorf("sportmonks: decoding fixture: %w", err)
	}

	minEventID, _ := strconv.ParseInt(lastEventID, 10, 64)

	now := time.Now()
	events := make([]domain.MatchEvent, 0, len(fixture.Events))
	for _, ev := range fixture.Events {
		if ev.ID <= minEventID {
			continue
		}
		events = append(events, mapEvent(ev, matchID, now))
	}

	p.recordSuccess()
	return events, nil
}

func (p *Provider) Health(ctx context.Context) domain.ProviderHealth {
	p.mu.Lock()
	defer p.mu.Unlock()

	return domain.ProviderHealth{
		Provider:      p.Name(),
		IsHealthy:     p.errorCount == 0 || p.lastSuccessAt.After(p.lastErrorAt),
		LastSuccessAt: p.lastSuccessAt,
		LastErrorAt:   p.lastErrorAt,
		ErrorCount:    p.errorCount,
		Message:       p.lastErrMsg,
	}
}

func (p *Provider) recordSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSuccessAt = time.Now()
}

func (p *Provider) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastErrorAt = time.Now()
	p.errorCount++
	p.lastErrMsg = err.Error()
}

func internalMatchID(fixtureID int64) string {
	return fmt.Sprintf("sportmonks_%d", fixtureID)
}

func fixtureIDFromMatchID(matchID string) (int64, error) {
	var fixtureID int64
	_, err := fmt.Sscanf(matchID, "sportmonks_%d", &fixtureID)
	return fixtureID, err
}

func participantIDs(participants []participantDTO) (home, away string) {
	for _, participant := range participants {
		id := strconv.FormatInt(participant.ID, 10)
		switch participant.Meta.Location {
		case "home":
			home = id
		case "away":
			away = id
		}
	}
	return home, away
}

func mapEvent(ev eventDTO, matchID string, receivedAt time.Time) domain.MatchEvent {
	var teamID *string
	if ev.ParticipantID != nil {
		id := strconv.FormatInt(*ev.ParticipantID, 10)
		teamID = &id
	}

	var playerID *string
	if ev.PlayerID != nil {
		id := strconv.FormatInt(*ev.PlayerID, 10)
		playerID = &id
	}

	period := 1
	if ev.Minute > 45 {
		period = 2
	}
	matchSecond := ev.Minute * 60
	if ev.ExtraMinute != nil {
		matchSecond += *ev.ExtraMinute * 60
	}

	// Preserve the raw decoded event (player name, result, info, etc.) so
	// nothing SportMonks sends is silently discarded, per the domain's
	// "never drop unrecognized data" principle.
	attributes, _ := json.Marshal(ev)

	// providerTime is approximated with the poll's receive time: the base
	// fixture/event payload only carries match-minute granularity, not a
	// wall-clock timestamp per event.
	return domain.MatchEvent{
		EventID:         fmt.Sprintf("sportmonks_ev_%d", ev.ID),
		MatchID:         matchID,
		Provider:        "sportmonks",
		ProviderEventID: strconv.FormatInt(ev.ID, 10),
		EventType:       mapEventType(ev.TypeID),
		TeamID:          teamID,
		PlayerID:        playerID,
		MatchSecond:     matchSecond,
		Period:          period,
		ProviderTime:    receivedAt,
		ReceivedAt:      receivedAt,
		ProcessedAt:     receivedAt,
		Attributes:      attributes,
		SchemaVersion:   1,
	}
}

func mapEventType(typeID int) domain.EventType {
	switch typeID {
	case typeIDGoal:
		return domain.EventGoal
	case typeIDPenaltyScored:
		return domain.EventPenaltyScored
	case typeIDSubstitution:
		return domain.EventSubstitution
	case typeIDYellowCard:
		return domain.EventYellowCard
	case typeIDRedCard:
		return domain.EventRedCard
	case typeIDYellowRedCard:
		return domain.EventSecondYellow
	default:
		return domain.EventUnknown
	}
}
