package odds

import (
	"testing"
	"time"
)

func TestValidateSnapshotRejectsDirtyRows(t *testing.T) {
	now := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	good := Quote{Market: "totals", Line: 2.5, Over: 1.9, Under: 1.9, UpdatedAt: now.Add(-time.Second)}
	events := []Event{
		{ID: "one", Home: "São Paulo", Away: "Santos", Quotes: []Quote{
			good,
			{Market: "totals", Over: 0, Under: 1.9, UpdatedAt: now},
			{Market: "totals", Over: 1.9, Under: 1.9},
			{Market: "totals", Over: 1.9, Under: 1.9, UpdatedAt: now.Add(-3 * time.Minute)},
		}},
		{ID: "one", Home: "São Paulo", Away: "Santos", Quotes: []Quote{good}},
		{ID: "empty", Home: "A", Away: "B"},
	}
	clean, report := ValidateSnapshot("shadow", events, now, 2*time.Minute)
	if len(clean) != 1 || len(clean[0].Quotes) != 1 {
		t.Fatalf("dirty rows survived: %+v", clean)
	}
	if report.DuplicateEvents != 1 || report.EmptyMarkets != 1 || report.InvalidPrices != 1 ||
		report.MissingTimestamps != 1 || report.StaleQuotes != 1 || report.AcceptedQuotes != 1 {
		t.Fatalf("wrong report: %+v", report)
	}
}

func TestCompareSnapshotsReportsCoverageScoreAndPriceGap(t *testing.T) {
	at := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	primary := []Event{{
		Home: "São Paulo", Away: "Santos", ScheduledAt: at, HomeScore: 1, AwayScore: 0,
		Quotes: []Quote{{Market: "totals", Line: 2.5, Over: 1.80}},
	}}
	shadow := []Event{{
		Home: "Sao Paulo", Away: "Santos", ScheduledAt: at.Add(time.Minute), HomeScore: 1, AwayScore: 0,
		Quotes: []Quote{{Market: "totals", Line: 2.5, Over: 1.90}},
	}}
	report := CompareSnapshots("primary", primary, "shadow", shadow, at)
	if report.MatchedEvents != 1 || report.ComparableQuotes != 1 ||
		report.MeanAbsoluteOverGap < .099 || report.MeanAbsoluteOverGap > .101 {
		t.Fatalf("wrong comparison: %+v", report)
	}
}
