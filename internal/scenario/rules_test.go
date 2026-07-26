package scenario

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func qualifiedTimelines() []MatchTimeline {
	rows := make([]MatchTimeline, 0, 600)
	for i := 0; i < 600; i++ {
		season := "2025"
		if i >= 400 {
			season = "2026"
		}
		year := 2025
		if season == "2026" {
			year = 2026
		}
		rows = append(rows, MatchTimeline{
			MatchID: fmt.Sprintf("m%d", i), CompetitionID: "648", Season: season, EndSecond: 5400,
			StartingAt: time.Date(year, time.January, 1, 0, 0, i, 0, time.UTC),
			Goals:      []Goal{{Second: 4500, Home: true}, {Second: 5100, Home: false}},
		})
	}
	return rows
}

func TestBacktestDefinitionUsesIndependentMatchesAndNewestSeason(t *testing.T) {
	def := DefaultDefinition()
	def.Name, def.Enabled = "late goal", true
	def.Conditions = []Condition{{Field: FieldMinute, Operator: OpGreaterOrEqual, Value: 70}}
	report, err := BacktestDefinition(def, qualifiedTimelines())
	if err != nil {
		t.Fatal(err)
	}
	if report.AllMatches.MatchCount != 600 || report.Holdout.MatchCount != 200 {
		t.Fatalf("wrong independent samples: total=%d holdout=%d", report.AllMatches.MatchCount, report.Holdout.MatchCount)
	}
	if !report.ValidationQualified || report.Qualified {
		t.Fatalf("expected validation pass and locked-test block, report=%+v", report)
	}
}

func TestTwoGoalTargetIsEvaluatedSeparately(t *testing.T) {
	rows := qualifiedTimelines()
	for i := range rows {
		rows[i].Goals = rows[i].Goals[:1]
	}
	def := DefaultDefinition()
	def.Name, def.AdditionalGoals = "two goals", 2
	def.Conditions = []Condition{{Field: FieldMinute, Operator: OpGreaterOrEqual, Value: 70}}
	report, err := BacktestDefinition(def, rows)
	if err != nil {
		t.Fatal(err)
	}
	if report.Holdout.Wins != 0 || report.Qualified {
		t.Fatalf("two-goal target inherited one-goal success: %+v", report)
	}
}

func TestPartitionTimelinesIsDeterministicDisjointAndUsesActualKickoff(t *testing.T) {
	rows := []MatchTimeline{
		{MatchID: "a-old", CompetitionID: "a", Season: "z-name", StartingAt: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)},
		{MatchID: "a-new", CompetitionID: "a", Season: "a-name", StartingAt: time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)},
		{MatchID: "b-only", CompetitionID: "b", Season: "only", StartingAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)},
		{MatchID: "c-old", CompetitionID: "c", Season: "1", StartingAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		{MatchID: "c-new", CompetitionID: "c", Season: "2", StartingAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	training, validation, first := PartitionTimelines(rows)
	_, _, second := PartitionTimelines([]MatchTimeline{rows[4], rows[2], rows[0], rows[3], rows[1]})

	if first.SHA256 != second.SHA256 || first.DatasetSHA256 != second.DatasetSHA256 {
		t.Fatalf("manifest changed with input order: %#v %#v", first, second)
	}
	changed := append([]MatchTimeline(nil), rows...)
	changed[0].Goals = []Goal{{Second: 100, Home: true}}
	_, _, outcomeChanged := PartitionTimelines(changed)
	if outcomeChanged.DatasetSHA256 == first.DatasetSHA256 || outcomeChanged.SHA256 == first.SHA256 {
		t.Fatal("outcome change did not invalidate dataset and manifest hashes")
	}
	if len(first.ExcludedCompetitionIDs) != 1 || first.ExcludedCompetitionIDs[0] != "b" {
		t.Fatalf("single-season competition was not reported: %#v", first.ExcludedCompetitionIDs)
	}
	if len(training) != 2 || len(validation) != 2 {
		t.Fatalf("unexpected split sizes: training=%d validation=%d", len(training), len(validation))
	}
	seen := map[string]string{}
	for _, row := range training {
		seen[row.MatchID] = "training"
	}
	for _, row := range validation {
		if partition := seen[row.MatchID]; partition != "" {
			t.Fatalf("fixture %s overlaps %s and validation", row.MatchID, partition)
		}
		seen[row.MatchID] = "validation"
	}
	if seen["a-new"] != "validation" || seen["a-old"] != "training" {
		t.Fatalf("season-name ordering leaked into split: %#v", seen)
	}
}

func TestMinimumHoldoutJSONRemainsReadCompatible(t *testing.T) {
	var definition StrategyDefinition
	if err := json.Unmarshal([]byte(`{
		"name":"legacy","additionalGoals":1,"minimumOdds":1.5,
		"minimumSamples":500,"minimumHoldout":150,"minimumModelEdge":0.08,
		"conditions":[{"field":"minute","operator":"gte","value":70}]
	}`), &definition); err != nil {
		t.Fatal(err)
	}
	if definition.validationMinimum() != 150 {
		t.Fatalf("legacy minimumHoldout was not used: %+v", definition)
	}
}
