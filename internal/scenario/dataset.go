package scenario

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// fixtureRecord mirrors one line of ml/data/fixtures.jsonl, as written by
// ml/src/build_dataset.py.
type fixtureRecord struct {
	FixtureID int64  `json:"fixture_id"`
	LeagueID  int64  `json:"league_id"`
	Season    string `json:"season"`
	Name      string `json:"name"`
	EndSecond int    `json:"end_second"`
	Goals     []struct {
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

	var timelines []MatchTimeline

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

		timeline := MatchTimeline{
			MatchID:       fmt.Sprintf("%d", rec.FixtureID),
			CompetitionID: fmt.Sprintf("%d", rec.LeagueID),
			Season:        rec.Season,
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
