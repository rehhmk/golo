package scenario

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// fixtureRecord mirrors one line of ml/data/fixtures.jsonl, as written by
// ml/src/build_dataset.py.
type fixtureRecord struct {
	FixtureID  int64  `json:"fixture_id"`
	LeagueID   int64  `json:"league_id"`
	Season     string `json:"season"`
	Name       string `json:"name"`
	StartingAt string `json:"starting_at"`
	EndSecond  int    `json:"end_second"`
	Goals      []struct {
		Second int  `json:"second"`
		Home   bool `json:"home"`
	} `json:"goals"`
	RedCards []struct {
		Second int  `json:"second"`
		Home   bool `json:"home"`
	} `json:"red_cards"`
}

// LoadTimelines reads the historical dataset produced by the Python builder.
//
// Reading the same file the trainer uses keeps the backtest honest: a
// scenario measured against a different set of matches than the model was
// fitted on would compare two different worlds.
func LoadTimelines(path string) ([]MatchTimeline, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("scenario: opening dataset: %w", err)
	}
	defer file.Close()

	timelines := make([]MatchTimeline, 0)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}

		var rec fixtureRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, fmt.Errorf("scenario: dataset line %d: %w", line, err)
		}

		startingAt, err := parseFixtureTime(rec.StartingAt)
		if err != nil {
			return nil, fmt.Errorf("scenario: dataset line %d starting_at: %w", line, err)
		}
		timeline := MatchTimeline{
			MatchID:       fmt.Sprintf("%d", rec.FixtureID),
			CompetitionID: fmt.Sprintf("%d", rec.LeagueID),
			Season:        rec.Season,
			StartingAt:    startingAt,
			EndSecond:     rec.EndSecond,
		}
		for _, g := range rec.Goals {
			timeline.Goals = append(timeline.Goals, Goal{Second: g.Second, Home: g.Home})
		}
		for _, r := range rec.RedCards {
			timeline.RedCards = append(timeline.RedCards, RedCard{Second: r.Second, Home: r.Home})
		}

		timelines = append(timelines, timeline)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scenario: reading dataset: %w", err)
	}

	return timelines, nil
}

func parseFixtureTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}

type PartitionSummary struct {
	MatchCount       int      `json:"matchCount"`
	EarliestKickoff  string   `json:"earliestKickoff,omitempty"`
	LatestKickoff    string   `json:"latestKickoff,omitempty"`
	FixtureIDsSHA256 string   `json:"fixtureIdsSha256"`
	CompetitionIDs   []string `json:"competitionIds"`
}

type PartitionManifest struct {
	SHA256                 string           `json:"sha256"`
	DatasetSHA256          string           `json:"datasetSha256"`
	Policy                 string           `json:"policy"`
	Training               PartitionSummary `json:"training"`
	Validation             PartitionSummary `json:"validation"`
	ExcludedCompetitionIDs []string         `json:"excludedCompetitionIds"`
}

// PartitionTimelines produces a deterministic, competition-aware historical
// split. The season with the latest actual kickoff in each competition is
// Validation; every earlier season is Training. Competitions with fewer than
// two seasons cannot support this comparison and are excluded.
func PartitionTimelines(matches []MatchTimeline) (training, validation []MatchTimeline, manifest PartitionManifest) {
	type seasonInfo struct {
		latest time.Time
	}
	byCompetition := make(map[string]map[string]seasonInfo)
	for _, match := range matches {
		if match.StartingAt.IsZero() {
			continue
		}
		seasons := byCompetition[match.CompetitionID]
		if seasons == nil {
			seasons = make(map[string]seasonInfo)
			byCompetition[match.CompetitionID] = seasons
		}
		info := seasons[match.Season]
		if match.StartingAt.After(info.latest) {
			info.latest = match.StartingAt
		}
		seasons[match.Season] = info
	}

	validationSeason := make(map[string]string)
	excluded := make([]string, 0)
	for competition, seasons := range byCompetition {
		if len(seasons) < 2 {
			excluded = append(excluded, competition)
			continue
		}
		var newest string
		var newestAt time.Time
		for season, info := range seasons {
			if info.latest.After(newestAt) || (info.latest.Equal(newestAt) && season > newest) {
				newest, newestAt = season, info.latest
			}
		}
		validationSeason[competition] = newest
	}
	sort.Strings(excluded)

	training = make([]MatchTimeline, 0, len(matches))
	validation = make([]MatchTimeline, 0, len(matches))
	eligible := make([]MatchTimeline, 0, len(matches))
	for _, match := range matches {
		newest, ok := validationSeason[match.CompetitionID]
		if !ok || match.StartingAt.IsZero() {
			continue
		}
		eligible = append(eligible, match)
		if match.Season == newest {
			validation = append(validation, match)
		} else {
			training = append(training, match)
		}
	}
	sortTimelines(training)
	sortTimelines(validation)
	sortTimelines(eligible)

	manifest = PartitionManifest{
		Policy:                 "latest-season-by-actual-kickoff-per-competition-v1",
		DatasetSHA256:          timelineHash(eligible),
		Training:               summarizePartition(training),
		Validation:             summarizePartition(validation),
		ExcludedCompetitionIDs: excluded,
	}
	canonical, _ := json.Marshal(struct {
		DatasetSHA256          string
		Policy                 string
		Training               PartitionSummary
		Validation             PartitionSummary
		ExcludedCompetitionIDs []string
	}{
		manifest.DatasetSHA256, manifest.Policy, manifest.Training,
		manifest.Validation, manifest.ExcludedCompetitionIDs,
	})
	manifest.SHA256 = fmt.Sprintf("%x", sha256.Sum256(canonical))
	return training, validation, manifest
}

func sortTimelines(matches []MatchTimeline) {
	sort.Slice(matches, func(i, j int) bool {
		if !matches[i].StartingAt.Equal(matches[j].StartingAt) {
			return matches[i].StartingAt.Before(matches[j].StartingAt)
		}
		return matches[i].MatchID < matches[j].MatchID
	})
}

func summarizePartition(matches []MatchTimeline) PartitionSummary {
	summary := PartitionSummary{MatchCount: len(matches), FixtureIDsSHA256: timelineIDHash(matches)}
	competitions := make(map[string]struct{})
	for _, match := range matches {
		competitions[match.CompetitionID] = struct{}{}
		if summary.EarliestKickoff == "" || match.StartingAt.Format(time.RFC3339) < summary.EarliestKickoff {
			summary.EarliestKickoff = match.StartingAt.Format(time.RFC3339)
		}
		if match.StartingAt.Format(time.RFC3339) > summary.LatestKickoff {
			summary.LatestKickoff = match.StartingAt.Format(time.RFC3339)
		}
	}
	for competition := range competitions {
		summary.CompetitionIDs = append(summary.CompetitionIDs, competition)
	}
	sort.Strings(summary.CompetitionIDs)
	return summary
}

func timelineIDHash(matches []MatchTimeline) string {
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.MatchID)
	}
	sort.Strings(ids)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(ids, "\n"))))
}

func timelineHash(matches []MatchTimeline) string {
	type row struct {
		ID          string
		Competition string
		Season      string
		StartingAt  string
		EndSecond   int
		Goals       []Goal
		RedCards    []RedCard
	}
	rows := make([]row, 0, len(matches))
	for _, match := range matches {
		goals := append([]Goal(nil), match.Goals...)
		redCards := append([]RedCard(nil), match.RedCards...)
		sort.Slice(goals, func(i, j int) bool {
			if goals[i].Second != goals[j].Second {
				return goals[i].Second < goals[j].Second
			}
			return !goals[i].Home && goals[j].Home
		})
		sort.Slice(redCards, func(i, j int) bool {
			if redCards[i].Second != redCards[j].Second {
				return redCards[i].Second < redCards[j].Second
			}
			return !redCards[i].Home && redCards[j].Home
		})
		rows = append(rows, row{
			match.MatchID, match.CompetitionID, match.Season,
			match.StartingAt.Format(time.RFC3339Nano), match.EndSecond, goals, redCards,
		})
	}
	raw, _ := json.Marshal(rows)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
