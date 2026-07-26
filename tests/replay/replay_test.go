package replay_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/enzotriches/golo/internal/domain"
	"github.com/enzotriches/golo/internal/features"
	"github.com/enzotriches/golo/internal/predictor"
	"github.com/enzotriches/golo/internal/providers/replay"
	"github.com/enzotriches/golo/internal/quality"
	"github.com/enzotriches/golo/internal/reducer"
)

type Fixture struct {
	Match  domain.Match        `json:"match"`
	Events []domain.MatchEvent `json:"events"`
}

func TestReplayPipeline_EndToEnd(t *testing.T) {
	fixturePath := filepath.Join("..", "fixtures", "sample_match.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var fix Fixture
	if err := json.Unmarshal(data, &fix); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}

	prov := replay.NewReplayProvider(fix.Match, fix.Events)
	prov.SetSpeed(replay.SpeedMax)

	red := reducer.NewReducer()
	fe := features.NewFeatureEngine()
	eval := quality.NewEvaluator()

	modelPath := filepath.Join("..", "..", "models", "baseline_v1.json")
	predEngine, err := predictor.NewPredictor(modelPath)
	if err != nil {
		t.Fatalf("failed to load predictor: %v", err)
	}

	ctx := context.Background()
	events, err := prov.FetchEventsSince(ctx, fix.Match.ID, "")
	if err != nil {
		t.Fatalf("failed to fetch events: %v", err)
	}

	if len(events) != len(fix.Events) {
		t.Errorf("expected %d events in max speed, got %d", len(fix.Events), len(events))
	}

	state := domain.InitialState(fix.Match)
	for _, ev := range events {
		state = red.Reduce(state, ev)
	}

	if state.Score.Home != 1 || state.Score.Away != 0 {
		t.Errorf("expected score 1-0 after replay, got %d-%d", state.Score.Home, state.Score.Away)
	}
	if state.RedCards.Away != 1 {
		t.Errorf("expected 1 away red card, got %d", state.RedCards.Away)
	}

	feats, _, err := fe.ExtractFeatures(state)
	if err != nil {
		t.Fatalf("failed to extract features: %v", err)
	}

	qScore := eval.EvaluateDataQuality(state)
	pred, err := predEngine.Predict(state, feats, qScore)
	if err != nil {
		t.Fatalf("failed to predict: %v", err)
	}

	if pred.Probabilities.GoalNext10m <= 0 || pred.Probabilities.GoalBeforeFullTime <= 0 {
		t.Errorf("expected non-zero probabilities, got %+v", pred.Probabilities)
	}
}
