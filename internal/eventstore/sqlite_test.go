package eventstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

func TestSQLiteStore_Integration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "golo_test_db_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "golo.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	defer store.Close()

	match := domain.Match{
		ID:            "m1",
		Provider:      "test_provider",
		HomeTeamID:    "home",
		AwayTeamID:    "away",
		CompetitionID: "comp1",
		Status:        domain.MatchStatusLive,
	}

	if err := store.SaveMatch(match); err != nil {
		t.Fatalf("SaveMatch failed: %v", err)
	}

	state := domain.InitialState(match)
	state.Score.Home = 1
	state.ClockSeconds = 1200

	if err := store.SaveMatchState(state); err != nil {
		t.Fatalf("SaveMatchState failed: %v", err)
	}

	readState, err := store.GetLatestMatchState("m1")
	if err != nil {
		t.Fatalf("GetLatestMatchState failed: %v", err)
	}

	if readState.Score.Home != 1 || readState.ClockSeconds != 1200 {
		t.Errorf("read state mismatch: %+v", readState)
	}

	pred := domain.Prediction{
		MatchID:         "m1",
		AsOfMatchSecond: 1200,
		CalculatedAt:    time.Now(),
		Probabilities: domain.Probabilities{
			GoalNext5m:         0.25,
			GoalNext10m:        0.45,
			GoalBeforeFullTime: 0.82,
		},
		DataQuality:        0.95,
		ConfidenceBand:     domain.ConfidenceHigh,
		Status:             domain.PredictionStatusOK,
		ModelVersion:       "v1.0",
		PredictionSequence: 1,
	}

	if err := store.SavePrediction(pred); err != nil {
		t.Fatalf("SavePrediction failed: %v", err)
	}

	preds, err := store.GetMatchPredictions("m1")
	if err != nil {
		t.Fatalf("GetMatchPredictions failed: %v", err)
	}

	if len(preds) != 1 || preds[0].Probabilities.GoalNext10m != 0.45 {
		t.Errorf("prediction mismatch: %+v", preds)
	}
}

// Accuracy reported across mixed model versions describes no model that
// exists. The store keeps every generation ever run, so a since-replaced
// model would otherwise keep dragging the published figures around.
func TestGetPredictionsForModelScopesByVersion(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "scope.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	versions := []string{"baseline_v1.0.0", "baseline_v1.0.0", "hazard_v1.1.0"}
	for i, version := range versions {
		if err := store.SavePrediction(domain.Prediction{
			MatchID:         "m1",
			AsOfMatchSecond: i * 60,
			ModelVersion:    version,
			ConfidenceBand:  domain.ConfidenceHigh,
			Status:          domain.PredictionStatusOK,
			CalculatedAt:    time.Now(),
		}); err != nil {
			t.Fatalf("SavePrediction: %v", err)
		}
	}

	current, err := store.GetPredictionsForModel("hazard_v1.1.0")
	if err != nil {
		t.Fatalf("GetPredictionsForModel: %v", err)
	}
	if len(current) != 1 {
		t.Fatalf("got %d predictions for the current model, want 1", len(current))
	}
	if current[0].ModelVersion != "hazard_v1.1.0" {
		t.Fatalf("got version %s", current[0].ModelVersion)
	}

	all, err := store.GetAllPredictions()
	if err != nil {
		t.Fatalf("GetAllPredictions: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d predictions unscoped, want all 3", len(all))
	}
}
