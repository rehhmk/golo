package sportmonks

import (
	"context"
	"encoding/json"
	"errors"
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

// SportMonks fixture state_id values, from the /states endpoint (verified
// live 2026-07-25). The /livescores/inplay endpoint does NOT only return
// matches in progress — it also returns fixtures kicking off shortly
// (state 1, "Not Started"), so state_id must be interpreted rather than
// assumed, or pre-match fixtures get published as live at minute zero.
const (
	stateNotStarted     = 1
	stateInplay1stHalf  = 2
	stateHalfTime       = 3
	stateBreak          = 4
	stateFullTime       = 5
	stateInplayET       = 6
	stateAfterET        = 7
	stateAfterPens      = 8
	stateInplayPens     = 9
	statePostponed      = 10
	stateSuspended      = 11
	stateCancelled      = 12
	stateAbandoned      = 15
	stateInterrupted    = 18
	stateInplay2ndHalf  = 22
	stateInplayET2nd    = 23
	statePenaltiesBreak = 25
	stateExtraTimeBreak = 21
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
	Periods      []periodDTO      `json:"periods"`
	Scores       []scoreDTO       `json:"scores"`
	Statistics   []statisticDTO   `json:"statistics"`
	League       *leagueDTO       `json:"league"`
}

type leagueDTO struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// periodDTO carries the live match clock. The period with ticking=true is the
// one currently running; its minutes/seconds are elapsed time within the
// match (already inclusive of counts_from, i.e. a 2nd half period reports
// minute 77, not minute 32).
type periodDTO struct {
	ID          int64 `json:"id"`
	TypeID      int   `json:"type_id"`
	CountsFrom  int   `json:"counts_from"`
	SortOrder   int   `json:"sort_order"`
	Ticking     bool  `json:"ticking"`
	HasTimer    bool  `json:"has_timer"`
	Minutes     *int  `json:"minutes"`
	Seconds     *int  `json:"seconds"`
	TimeAdded   *int  `json:"time_added"`
	Started     *int64 `json:"started"`
	Ended       *int64 `json:"ended"`
	Description string `json:"description"`
}

type scoreDTO struct {
	ParticipantID int64  `json:"participant_id"`
	Description   string `json:"description"`
	Score         struct {
		Goals       int    `json:"goals"`
		Participant string `json:"participant"`
	} `json:"score"`
}

type statisticDTO struct {
	ParticipantID int64 `json:"participant_id"`
	TypeID        int   `json:"type_id"`
	Type          struct {
		DeveloperName string `json:"developer_name"`
	} `json:"type"`
	Data struct {
		Value float64 `json:"value"`
	} `json:"data"`
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

	// mu guards the cached fixture payloads and health counters.
	//
	// cache holds the full fixture as returned by the most recent
	// ListLiveMatches poll, keyed by internal match ID. A single
	// /livescores/inplay call carries participants, periods, scores,
	// statistics and events for every live match at once, so both
	// FetchEventsSince and Snapshot are served from here without further
	// I/O. That matters: the previous one-request-per-match approach cost
	// 1+N calls per tick, which at a 3s poll with 8 live matches is ~10,800
	// requests/hour against an API budget of roughly 3,000.
	mu    sync.Mutex
	cache map[string]fixtureDTO

	// cooldownUntil suppresses outbound requests after a 429. SportMonks
	// quotas reset on an hourly window, so polling through a rate limit
	// cannot succeed and only burns the quota the moment it refills.
	cooldownUntil time.Time
	cooldown      time.Duration

	lastSuccessAt time.Time
	lastErrorAt   time.Time
	errorCount    int
	lastErrMsg    string
}

// Backoff bounds for rate-limit cooldowns: long enough that a quota has a
// chance to refill, short enough that recovery isn't delayed by many minutes.
const (
	minRateLimitCooldown = 60 * time.Second
	maxRateLimitCooldown = 10 * time.Minute
)

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
		cache:        make(map[string]fixtureDTO),
	}
}

func (p *Provider) Name() string {
	return "sportmonks"
}

// inplayIncludes pulls everything Golo needs for one poll tick in a single
// request: team identities, the live clock, the authoritative score, the
// cumulative statistics that feed the rolling windows, and the event list.
const inplayIncludes = "participants;periods;scores;statistics.type;events;league"

func (p *Provider) ListLiveMatches(ctx context.Context) ([]domain.Match, error) {
	if wait, ok := p.inCooldown(); ok {
		return nil, fmt.Errorf("sportmonks: rate limited, retrying in %s", wait.Round(time.Second))
	}

	query := url.Values{}
	query.Set("include", inplayIncludes)

	data, err := p.client.get(ctx, "/livescores/inplay", query)
	if err != nil {
		if errors.Is(err, errRateLimited) {
			p.enterCooldown()
		}
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
	// Replace rather than merge: a fixture that dropped out of the inplay
	// response has finished, so its stale payload must not keep being served
	// to Snapshot as though it were current.
	p.cache = make(map[string]fixtureDTO, len(fixtures))
	for _, f := range fixtures {
		matchID := internalMatchID(f.ID)
		p.cache[matchID] = f

		home, away := participantIDs(f.Participants)
		homeName, awayName := participantNames(f.Participants)
		scheduledAt, _ := time.Parse(startingAtLayout, f.StartingAt)

		competitionName := ""
		if f.League != nil {
			competitionName = f.League.Name
		}

		matches = append(matches, domain.Match{
			HomeTeamName:    homeName,
			AwayTeamName:    awayName,
			CompetitionName: competitionName,
			ID:              matchID,
			Provider:        p.Name(),
			ProviderMatchID: strconv.FormatInt(f.ID, 10),
			CompetitionID:   strconv.FormatInt(f.LeagueID, 10),
			SeasonID:        strconv.FormatInt(f.SeasonID, 10),
			HomeTeamID:      home,
			AwayTeamID:      away,
			ScheduledAt:     scheduledAt,
			Status:          mapStatus(f.StateID),
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}
	p.mu.Unlock()

	p.sortByPriority(matches)

	p.recordSuccess()
	return matches, nil
}

// Snapshot serves the cached view captured by the last ListLiveMatches call.
func (p *Provider) Snapshot(matchID string) (domain.LiveSnapshot, bool) {
	p.mu.Lock()
	f, ok := p.cache[matchID]
	p.mu.Unlock()
	if !ok {
		return domain.LiveSnapshot{}, false
	}

	home, away := participantIDs(f.Participants)
	homeStats, awayStats, hasStats := statTotals(f.Statistics, home, away)

	now := time.Now()
	return domain.LiveSnapshot{
		MatchID:      matchID,
		Status:       mapStatus(f.StateID),
		Period:       periodNumber(f.StateID),
		ClockSeconds: clockSeconds(f.Periods, f.StateID),
		HasScore:     true,
		Score:        currentScore(f.Scores, home, away),
		HasStats:     hasStats,
		Home:         homeStats,
		Away:         awayStats,
		ProviderTime: now,
		ReceivedAt:   now,
	}, true
}

// mapStatus translates a SportMonks state_id into Golo's match lifecycle.
// Unknown states map to STALE rather than LIVE so an unrecognized code can
// never cause a non-running match to be published as though it were live.
func mapStatus(stateID int) domain.MatchStatus {
	switch stateID {
	case stateInplay1stHalf, stateInplay2ndHalf, stateInplayET, stateInplayET2nd, stateInplayPens:
		return domain.MatchStatusLive
	case stateHalfTime:
		return domain.MatchStatusHalfTime
	case stateBreak, stateExtraTimeBreak, statePenaltiesBreak:
		return domain.MatchStatusPaused
	case stateNotStarted:
		return domain.MatchStatusScheduled
	case stateFullTime, stateAfterET, stateAfterPens:
		return domain.MatchStatusFinished
	case statePostponed, stateCancelled, stateAbandoned:
		return domain.MatchStatusCancelled
	case stateSuspended, stateInterrupted:
		return domain.MatchStatusPaused
	default:
		return domain.MatchStatusStale
	}
}

// periodNumber reports which half the match is in, matching the reducer's
// 1-based period convention. Extra time and penalties continue the count.
func periodNumber(stateID int) int {
	switch stateID {
	case stateInplay1stHalf:
		return 1
	case stateHalfTime, stateInplay2ndHalf:
		return 2
	case stateInplayET, stateExtraTimeBreak, stateInplayET2nd:
		return 3
	case stateInplayPens, statePenaltiesBreak:
		return 4
	default:
		return 0
	}
}

// clockSeconds extracts elapsed match time from the period list.
//
// SportMonks reports minutes/seconds on the ticking period already inclusive
// of counts_from, so a 2nd half at 77:52 reports minutes=77 — not 32. When no
// period is ticking (half time, or a feed without a timer) the clock is taken
// from the furthest-progressed period instead, so state still reflects where
// the match actually is rather than falling back to zero.
func clockSeconds(periods []periodDTO, stateID int) int {
	best := 0
	for _, period := range periods {
		secs := 0
		if period.Minutes != nil {
			secs = *period.Minutes * 60
		}
		if period.Seconds != nil {
			secs += *period.Seconds
		}
		if period.Ticking && period.Minutes != nil {
			return secs
		}
		if secs > best {
			best = secs
		}
	}

	// Half time has no ticking period; the match is at least 45 minutes in.
	if best == 0 && stateID == stateHalfTime {
		return 45 * 60
	}
	return best
}

// currentScore reads the CURRENT score entries, which SportMonks maintains
// authoritatively. Preferring these over the event-derived tally keeps the
// score correct for goal variants the event type mapping doesn't recognize
// (own goals in particular have no confirmed type_id).
func currentScore(scores []scoreDTO, homeID, awayID string) domain.ScoreState {
	var out domain.ScoreState
	for _, s := range scores {
		if s.Description != "CURRENT" {
			continue
		}
		switch strconv.FormatInt(s.ParticipantID, 10) {
		case homeID:
			out.Home = s.Score.Goals
		case awayID:
			out.Away = s.Score.Goals
		}
	}
	return out
}

// statTotals collects the cumulative statistics the feature engine consumes.
// Statistic types are matched by developer_name, which is stable across the
// API, rather than by numeric type_id.
func statTotals(stats []statisticDTO, homeID, awayID string) (home, away domain.TeamStatTotals, hasStats bool) {
	for _, s := range stats {
		var target *domain.TeamStatTotals
		switch strconv.FormatInt(s.ParticipantID, 10) {
		case homeID:
			target = &home
		case awayID:
			target = &away
		default:
			continue
		}

		value := int(s.Data.Value)
		switch s.Type.DeveloperName {
		case "SHOTS_ON_TARGET":
			target.ShotsOnTarget = value
		case "SHOTS_OFF_TARGET":
			target.ShotsOffTarget = value
		case "SHOTS_BLOCKED":
			target.ShotsBlocked = value
		case "CORNERS":
			target.Corners = value
		case "FOULS":
			target.Fouls = value
		case "DANGEROUS_ATTACKS":
			target.DangerousAttacks = value
		case "BALL_POSSESSION":
			target.Possession = s.Data.Value
		default:
			continue
		}
		hasStats = true
	}
	return home, away, hasStats
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

// FetchEventsSince returns the events for a fixture that are newer than
// lastEventID, read from the payload the last ListLiveMatches poll already
// fetched. SportMonks has no native "since" cursor for events, so the feed
// carries the full cumulative list every time and the id filter narrows it
// to what the reducer hasn't folded in yet.
//
// This performs no request of its own. The event list arrives with the
// inplay poll, so fetching it again per match would triple-to-tenfold the
// request count for data already in hand.
func (p *Provider) FetchEventsSince(ctx context.Context, matchID string, lastEventID string) ([]domain.MatchEvent, error) {
	p.mu.Lock()
	fixture, ok := p.cache[matchID]
	p.mu.Unlock()
	if !ok {
		// Not in the last poll: the match is over or was never live. No
		// events rather than an error — the ingestion loop drops it anyway.
		return nil, nil
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

// inCooldown reports whether requests are currently suppressed, and for how
// much longer.
func (p *Provider) inCooldown() (time.Duration, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if remaining := time.Until(p.cooldownUntil); remaining > 0 {
		return remaining, true
	}
	return 0, false
}

// enterCooldown starts (or doubles) the rate-limit backoff.
func (p *Provider) enterCooldown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cooldown == 0 {
		p.cooldown = minRateLimitCooldown
	} else if p.cooldown < maxRateLimitCooldown {
		p.cooldown *= 2
		if p.cooldown > maxRateLimitCooldown {
			p.cooldown = maxRateLimitCooldown
		}
	}
	p.cooldownUntil = time.Now().Add(p.cooldown)
}

func (p *Provider) recordSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSuccessAt = time.Now()
	// A successful call means the quota has refilled; drop back to the
	// shortest backoff so a later, unrelated limit isn't punished by the
	// previous incident's escalation.
	p.cooldown = 0
	p.cooldownUntil = time.Time{}
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

// participantNames returns the display names for each side. SportMonks
// identifies teams by numeric ID, which is meaningless to a reader — without
// these the UI shows "609 vs 15522" instead of the actual fixture.
func participantNames(participants []participantDTO) (home, away string) {
	for _, participant := range participants {
		switch participant.Meta.Location {
		case "home":
			home = participant.Name
		case "away":
			away = participant.Name
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
