package scenario

import (
	"fmt"
	"testing"
)

func qualifiedTimelines() []MatchTimeline {
	rows := make([]MatchTimeline, 0, 600)
	for i := 0; i < 600; i++ {
		season := "2025"
		if i >= 400 {
			season = "2026"
		}
		rows = append(rows, MatchTimeline{
			MatchID: fmt.Sprintf("m%d", i), CompetitionID: "648", Season: season, EndSecond: 5400,
			Goals: []Goal{{Second: 4500, Home: true}, {Second: 5100, Home: false}},
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
	if !report.Qualified {
		t.Fatalf("expected qualification, failures=%v", report.Failures)
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
