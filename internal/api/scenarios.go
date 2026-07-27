package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/enzotriches/golo/internal/scenario"
)

// scenarioCache holds the historical dataset in memory.
//
// The dataset is tens of megabytes and immutable for the life of the process,
// while a backtest is interactive — a user adjusts a condition and expects an
// answer immediately. Re-reading the file per request would make the product
// feel broken for no reason.
type scenarioCache struct {
	once     sync.Once
	matches  []scenario.MatchTimeline
	loadErr  error
	loadPath string
}

var scenarios scenarioCache

func (c *scenarioCache) load(path string) ([]scenario.MatchTimeline, error) {
	c.once.Do(func() {
		c.loadPath = path
		c.matches, c.loadErr = scenario.LoadTimelines(path)
	})
	return c.matches, c.loadErr
}

// defaultDrawdown is the share of a bankroll the stake sizing assumes a user
// is willing to lose through the worst observed losing run.
const defaultDrawdown = 0.20

type backtestRequest struct {
	scenario.Definition
	// Drawdown is optional; it only scales the suggested stake and never
	// affects whether the scenario is worthwhile.
	Drawdown float64 `json:"drawdown,omitempty"`
}

// handleScenarioBacktest measures a user-built scenario against the historical
// dataset. It is deliberately unauthenticated: this is the product, and the
// answer it gives — including "this does not work" — is the thing worth
// showing before anyone signs up.
func (s *Server) handleScenarioBacktest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req backtestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "não consegui ler o cenário enviado")
		return
	}

	matches, err := scenarios.load(s.datasetPath)
	if err != nil {
		// The dataset is built by ml/src/build_dataset.py and is not committed,
		// so a fresh checkout genuinely lacks it. Say which file is missing
		// rather than returning a bare 500.
		writeJSONError(w, http.StatusServiceUnavailable,
			"o histórico não está disponível no servidor ("+s.datasetPath+")")
		return
	}

	drawdown := req.Drawdown
	if drawdown <= 0 || drawdown > 1 {
		drawdown = defaultDrawdown
	}

	report, err := scenario.Run(req.Definition, matches, drawdown)
	if err != nil {
		// A rejected definition is user error with a readable reason, not a
		// server fault.
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// handleScenarioDataset reports what the backtest is measuring against, so the
// interface can state the sample size instead of implying an unbounded one.
func (s *Server) handleScenarioDataset(w http.ResponseWriter, r *http.Request) {
	matches, err := scenarios.load(s.datasetPath)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "histórico indisponível")
		return
	}

	competitions := make(map[string]int)
	goals := 0
	for _, m := range matches {
		competitions[m.CompetitionID]++
		goals += len(m.Goals)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"matches":      len(matches),
		"goals":        goals,
		"competitions": competitions,
	})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
