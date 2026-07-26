package eventstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/enzotriches/golo/internal/scenario"
	"github.com/enzotriches/golo/internal/signals"
)

func (s *SQLiteStore) GetStrategy(id string, version int) (signals.StoredStrategy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT s.definition_json, s.report_json, s.armed, s.created_at, s.updated_at,
		COALESCE(t.id, ''), COALESCE(t.state, '')
		FROM strategies s LEFT JOIN strategy_locked_tests t
		ON t.strategy_id=s.id AND t.strategy_version=s.version
		WHERE s.id=? AND s.version=?`, id, version)
	if err != nil {
		return signals.StoredStrategy{}, err
	}
	defer rows.Close()
	list, err := scanStrategies(rows)
	if err != nil {
		return signals.StoredStrategy{}, err
	}
	if len(list) == 0 {
		return signals.StoredStrategy{}, sql.ErrNoRows
	}
	return list[0], nil
}

func scanStrategies(rows *sql.Rows) ([]signals.StoredStrategy, error) {
	out := make([]signals.StoredStrategy, 0)
	for rows.Next() {
		var strategy signals.StoredStrategy
		var definition, report string
		if err := rows.Scan(&definition, &report, &strategy.Armed, &strategy.CreatedAt, &strategy.UpdatedAt,
			&strategy.LockedTestID, &strategy.LockedTestState); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(definition), &strategy.Definition); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(report), &strategy.Report); err != nil {
			return nil, err
		}
		if strategy.LockedTestState == "" {
			strategy.LockedTestState = signals.LockedStateDraft
			if strategy.Report.ValidationQualified && strategy.Report.ModelValidationQualified {
				strategy.LockedTestState = signals.LockedStateValidated
			}
		}
		out = append(out, strategy)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) CreateLockedTest(test signals.LockedTest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if test.ID == "" || test.StrategyID == "" || test.StrategyVersion <= 0 {
		return fmt.Errorf("invalid locked test identity")
	}
	if test.State != signals.LockedStateCollecting {
		return fmt.Errorf("new locked test must start collecting")
	}
	if test.Contract.TargetOccurrences != signals.LockedCohortTarget {
		return fmt.Errorf("locked cohort target must be %d", signals.LockedCohortTarget)
	}
	if !test.ValidationReport.ValidationQualified || !test.ValidationReport.ModelValidationQualified {
		return fmt.Errorf("strategy and model must pass validation before sealing")
	}
	contract, err := json.Marshal(test.Contract)
	if err != nil {
		return err
	}
	validation, err := json.Marshal(test.ValidationReport)
	if err != nil {
		return err
	}
	now := time.Now()
	if test.StartedAt.IsZero() {
		test.StartedAt = now
	}
	if test.CreatedAt.IsZero() {
		test.CreatedAt = now
	}
	_, err = s.db.Exec(`INSERT INTO strategy_locked_tests
		(id, strategy_id, strategy_version, state, contract_json, contract_sha256,
		 validation_report_json, report_json, started_at, ready_at, revealed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, NULL, NULL, ?, ?)`,
		test.ID, test.StrategyID, test.StrategyVersion, test.State, string(contract),
		test.ContractSHA256, string(validation), test.StartedAt, test.CreatedAt, now)
	return err
}

func (s *SQLiteStore) ListCollectingLockedTests() ([]signals.LockedTest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, strategy_id, strategy_version, state, contract_json,
		contract_sha256, validation_report_json, report_json, started_at, ready_at,
		revealed_at, created_at, updated_at
		FROM strategy_locked_tests WHERE state=? ORDER BY started_at`,
		signals.LockedStateCollecting)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLockedTests(rows)
}

func (s *SQLiteStore) GetLockedTestByStrategy(strategyID string, version int) (signals.LockedTest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return getLockedTest(s.db, `WHERE strategy_id=? AND strategy_version=?`, strategyID, version)
}

func (s *SQLiteStore) GetLockedTest(id string) (signals.LockedTest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return getLockedTest(s.db, `WHERE id=?`, id)
}

func getLockedTest(db queryer, where string, args ...any) (signals.LockedTest, error) {
	rows, err := db.Query(`SELECT id, strategy_id, strategy_version, state, contract_json,
		contract_sha256, validation_report_json, report_json, started_at, ready_at,
		revealed_at, created_at, updated_at
		FROM strategy_locked_tests `+where, args...)
	if err != nil {
		return signals.LockedTest{}, err
	}
	defer rows.Close()
	tests, err := scanLockedTests(rows)
	if err != nil {
		return signals.LockedTest{}, err
	}
	if len(tests) == 0 {
		return signals.LockedTest{}, sql.ErrNoRows
	}
	return tests[0], nil
}

func scanLockedTests(rows *sql.Rows) ([]signals.LockedTest, error) {
	out := make([]signals.LockedTest, 0)
	for rows.Next() {
		var test signals.LockedTest
		var state, contract, validation string
		var report sql.NullString
		var ready, revealed sql.NullTime
		if err := rows.Scan(&test.ID, &test.StrategyID, &test.StrategyVersion, &state,
			&contract, &test.ContractSHA256, &validation, &report, &test.StartedAt,
			&ready, &revealed, &test.CreatedAt, &test.UpdatedAt); err != nil {
			return nil, err
		}
		test.State = signals.LockedTestState(state)
		if err := json.Unmarshal([]byte(contract), &test.Contract); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(validation), &test.ValidationReport); err != nil {
			return nil, err
		}
		if report.Valid {
			var parsed scenario.LockedTestReport
			if err := json.Unmarshal([]byte(report.String), &parsed); err != nil {
				return nil, err
			}
			test.Report = &parsed
		}
		if ready.Valid {
			test.ReadyAt = &ready.Time
		}
		if revealed.Valid {
			test.RevealedAt = &revealed.Time
		}
		out = append(out, test)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetLockedTestView(strategyID string, version int) (signals.LockedTestView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	test, err := getLockedTest(s.db, `WHERE strategy_id=? AND strategy_version=?`, strategyID, version)
	if err != nil {
		return signals.LockedTestView{}, err
	}
	progress, err := lockedProgress(s.db, test.ID, test.Contract.TargetOccurrences)
	if err != nil {
		return signals.LockedTestView{}, err
	}
	view := signals.LockedTestView{
		ID: test.ID, StrategyID: test.StrategyID, StrategyVersion: test.StrategyVersion,
		State: test.State, Contract: test.Contract, ContractSHA256: test.ContractSHA256,
		Progress: progress, StartedAt: test.StartedAt, ReadyAt: test.ReadyAt, RevealedAt: test.RevealedAt,
	}
	if test.State == signals.LockedStateRevealedPass || test.State == signals.LockedStateRevealedFail {
		view.Report = test.Report
	}
	return view, nil
}

func lockedProgress(db queryer, testID string, target int) (signals.LockedTestProgress, error) {
	progress := signals.LockedTestProgress{TargetOccurrences: target}
	rows, err := db.Query(`SELECT rule_cohort, signal_cohort, status, payload_json
		FROM strategy_locked_occurrences WHERE test_id=?`, testID)
	if err != nil {
		return progress, err
	}
	defer rows.Close()
	for rows.Next() {
		var rule, signal bool
		var status, payload string
		if err := rows.Scan(&rule, &signal, &status, &payload); err != nil {
			return progress, err
		}
		if signals.LockedOccurrenceStatus(status) == signals.LockedOccurrenceVoid {
			progress.Voids++
			continue
		}
		var occurrence signals.LockedOccurrence
		if err := json.Unmarshal([]byte(payload), &occurrence); err != nil {
			return progress, err
		}
		if rule {
			progress.RuleAccepted++
			if occurrence.Gates["feed_fresh"] {
				progress.RuleFeedsFresh++
			}
			if occurrence.Quote.Over > 1 {
				progress.RuleQuotesAvailable++
			}
			if occurrence.Status == signals.LockedOccurrenceWon || occurrence.Status == signals.LockedOccurrenceLost {
				progress.RuleResolved++
			}
		}
		if signal {
			progress.SignalAccepted++
			if occurrence.Status == signals.LockedOccurrenceWon || occurrence.Status == signals.LockedOccurrenceLost {
				progress.SignalResolved++
			}
		}
	}
	return progress, rows.Err()
}

// AdmitLockedOccurrence atomically assigns the match to whichever cohorts
// still have capacity. It returns false when the match was already seen or
// both cohorts are full.
func (s *SQLiteStore) AdmitLockedOccurrence(occurrence signals.LockedOccurrence) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var state string
	var contractJSON string
	if err := tx.QueryRow(`SELECT state, contract_json FROM strategy_locked_tests WHERE id=?`, occurrence.TestID).
		Scan(&state, &contractJSON); err != nil {
		return false, err
	}
	if signals.LockedTestState(state) != signals.LockedStateCollecting {
		return false, nil
	}
	var contract signals.LockedTestContract
	if err := json.Unmarshal([]byte(contractJSON), &contract); err != nil {
		return false, err
	}
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM strategy_locked_occurrences WHERE test_id=? AND match_id=?`,
		occurrence.TestID, occurrence.MatchID).Scan(&exists); err != nil {
		return false, err
	}
	if exists > 0 {
		return false, nil
	}
	var ruleCount, signalCount int
	if err := tx.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN rule_cohort=1 AND status<>? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN signal_cohort=1 AND status<>? THEN 1 ELSE 0 END),0)
		FROM strategy_locked_occurrences WHERE test_id=?`,
		signals.LockedOccurrenceVoid, signals.LockedOccurrenceVoid, occurrence.TestID).
		Scan(&ruleCount, &signalCount); err != nil {
		return false, err
	}
	occurrence.RuleCohort = ruleCount < contract.TargetOccurrences
	occurrence.SignalCohort = occurrence.SignalEligible && signalCount < contract.TargetOccurrences
	if !occurrence.RuleCohort && !occurrence.SignalCohort {
		return false, nil
	}
	if occurrence.ID == "" {
		return false, fmt.Errorf("locked occurrence id is required")
	}
	if occurrence.Status == "" {
		occurrence.Status = signals.LockedOccurrenceOpen
	}
	if occurrence.CreatedAt.IsZero() {
		occurrence.CreatedAt = time.Now()
	}
	payload, err := json.Marshal(occurrence)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(`INSERT INTO strategy_locked_occurrences
		(id, test_id, match_id, rule_cohort, signal_cohort, signal_eligible,
		 status, payload_json, created_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		occurrence.ID, occurrence.TestID, occurrence.MatchID, occurrence.RuleCohort,
		occurrence.SignalCohort, occurrence.SignalEligible, occurrence.Status,
		string(payload), occurrence.CreatedAt, occurrence.ResolvedAt); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *SQLiteStore) ListOpenLockedOccurrencesForMatch(matchID string) ([]signals.LockedOccurrence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT payload_json FROM strategy_locked_occurrences
		WHERE match_id=? AND status=?`, matchID, signals.LockedOccurrenceOpen)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLockedOccurrences(rows)
}

func scanLockedOccurrences(rows *sql.Rows) ([]signals.LockedOccurrence, error) {
	out := make([]signals.LockedOccurrence, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var occurrence signals.LockedOccurrence
		if err := json.Unmarshal([]byte(payload), &occurrence); err != nil {
			return nil, err
		}
		out = append(out, occurrence)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) UpdateLockedOccurrenceStatus(id string, status signals.LockedOccurrenceStatus, resolvedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var testID, payload string
	if err := tx.QueryRow(`SELECT test_id, payload_json FROM strategy_locked_occurrences WHERE id=?`, id).
		Scan(&testID, &payload); err != nil {
		return err
	}
	var occurrence signals.LockedOccurrence
	if err := json.Unmarshal([]byte(payload), &occurrence); err != nil {
		return err
	}
	if occurrence.Status != signals.LockedOccurrenceOpen {
		return nil
	}
	occurrence.Status, occurrence.ResolvedAt = status, &resolvedAt
	updated, err := json.Marshal(occurrence)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE strategy_locked_occurrences
		SET status=?, payload_json=?, resolved_at=? WHERE id=?`,
		status, string(updated), resolvedAt, id); err != nil {
		return err
	}

	var target, ruleResolved, signalResolved int
	if err := tx.QueryRow(`SELECT
		json_extract(contract_json, '$.targetOccurrences'),
		(SELECT COUNT(*) FROM strategy_locked_occurrences
		 WHERE test_id=? AND rule_cohort=1 AND status IN (?, ?)),
		(SELECT COUNT(*) FROM strategy_locked_occurrences
		 WHERE test_id=? AND signal_cohort=1 AND status IN (?, ?))
		FROM strategy_locked_tests WHERE id=?`,
		testID, signals.LockedOccurrenceWon, signals.LockedOccurrenceLost,
		testID, signals.LockedOccurrenceWon, signals.LockedOccurrenceLost, testID).
		Scan(&target, &ruleResolved, &signalResolved); err != nil {
		return err
	}
	if ruleResolved >= target && signalResolved >= target {
		if _, err := tx.Exec(`UPDATE strategy_locked_tests SET state=?, ready_at=?, updated_at=?
			WHERE id=? AND state=?`, signals.LockedStateReady, resolvedAt, resolvedAt,
			testID, signals.LockedStateCollecting); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) RevealLockedTest(strategyID string, version int, now time.Time) (signals.LockedTestView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return signals.LockedTestView{}, err
	}
	defer tx.Rollback()
	test, err := getLockedTest(tx, `WHERE strategy_id=? AND strategy_version=?`, strategyID, version)
	if err != nil {
		return signals.LockedTestView{}, err
	}
	if test.State == signals.LockedStateRevealedPass || test.State == signals.LockedStateRevealedFail {
		progress, progressErr := lockedProgress(tx, test.ID, test.Contract.TargetOccurrences)
		if progressErr != nil {
			return signals.LockedTestView{}, progressErr
		}
		return lockedView(test, progress), nil
	}
	if test.State != signals.LockedStateReady {
		return signals.LockedTestView{}, fmt.Errorf("locked test is not ready")
	}
	rows, err := tx.Query(`SELECT payload_json FROM strategy_locked_occurrences
		WHERE test_id=? AND status IN (?, ?) ORDER BY created_at, id`,
		test.ID, signals.LockedOccurrenceWon, signals.LockedOccurrenceLost)
	if err != nil {
		return signals.LockedTestView{}, err
	}
	occurrences, err := scanLockedOccurrences(rows)
	rows.Close()
	if err != nil {
		return signals.LockedTestView{}, err
	}
	report := buildLockedReport(test, occurrences, now)
	state := signals.LockedStateRevealedFail
	if report.Qualified {
		state = signals.LockedStateRevealedPass
	}
	reportJSON, _ := json.Marshal(report)
	test.ValidationReport.LockedTest = &report
	test.ValidationReport.Qualified = report.Qualified
	test.ValidationReport.Failures = append([]string(nil), test.ValidationReport.ValidationFailures...)
	test.ValidationReport.Failures = append(test.ValidationReport.Failures, report.Failures...)
	sort.Strings(test.ValidationReport.Failures)
	qualificationJSON, _ := json.Marshal(test.ValidationReport)
	if _, err := tx.Exec(`UPDATE strategy_locked_tests
		SET state=?, report_json=?, revealed_at=?, updated_at=? WHERE id=?`,
		state, string(reportJSON), now, now, test.ID); err != nil {
		return signals.LockedTestView{}, err
	}
	if _, err := tx.Exec(`UPDATE strategies SET report_json=?, updated_at=?
		WHERE id=? AND version=?`, string(qualificationJSON), now, strategyID, version); err != nil {
		return signals.LockedTestView{}, err
	}
	progress, err := lockedProgress(tx, test.ID, test.Contract.TargetOccurrences)
	if err != nil {
		return signals.LockedTestView{}, err
	}
	if err := tx.Commit(); err != nil {
		return signals.LockedTestView{}, err
	}
	test.State, test.Report, test.RevealedAt, test.UpdatedAt = state, &report, &now, now
	return lockedView(test, progress), nil
}

func lockedView(test signals.LockedTest, progress signals.LockedTestProgress) signals.LockedTestView {
	view := signals.LockedTestView{
		ID: test.ID, StrategyID: test.StrategyID, StrategyVersion: test.StrategyVersion,
		State: test.State, Contract: test.Contract, ContractSHA256: test.ContractSHA256,
		Progress: progress, StartedAt: test.StartedAt, ReadyAt: test.ReadyAt, RevealedAt: test.RevealedAt,
	}
	if test.State == signals.LockedStateRevealedPass || test.State == signals.LockedStateRevealedFail {
		view.Report = test.Report
	}
	return view
}

func buildLockedReport(test signals.LockedTest, occurrences []signals.LockedOccurrence, now time.Time) scenario.LockedTestReport {
	ruleOutcomes := make([]scenario.Occurrence, 0, test.Contract.TargetOccurrences)
	signalOutcomes := make([]scenario.Occurrence, 0, test.Contract.TargetOccurrences)
	var modelLoss, baselineLoss, marketLoss, profit, peak, maxDrawdown float64
	var ruleQuotes, signalMarketCount int
	for _, occurrence := range occurrences {
		won := occurrence.Status == signals.LockedOccurrenceWon
		y := 0.0
		if won {
			y = 1
		}
		if occurrence.RuleCohort && len(ruleOutcomes) < test.Contract.TargetOccurrences {
			ruleOutcomes = append(ruleOutcomes, scenario.Occurrence{MatchID: occurrence.MatchID, Second: occurrence.TriggerSecond, Won: won})
			modelLoss += squaredLoss(occurrence.ModelProbability, y)
			baselineLoss += squaredLoss(occurrence.BaselineProbability, y)
			if occurrence.Quote.Over > 1 {
				ruleQuotes++
			}
		}
		if occurrence.SignalCohort && len(signalOutcomes) < test.Contract.TargetOccurrences {
			signalOutcomes = append(signalOutcomes, scenario.Occurrence{MatchID: occurrence.MatchID, Second: occurrence.TriggerSecond, Won: won})
			if occurrence.MarketProbability > 0 {
				marketLoss += squaredLoss(occurrence.MarketProbability, y)
				signalMarketCount++
			}
			if won {
				profit += occurrence.Quote.Over - 1
			} else {
				profit--
			}
			if profit > peak {
				peak = profit
			}
			if drawdown := peak - profit; drawdown > maxDrawdown {
				maxDrawdown = drawdown
			}
		}
	}
	sc := scenario.Scenario{Name: test.Contract.Definition.Name, AdditionalGoals: test.Contract.Definition.AdditionalGoals}
	ruleResult := scenario.ResultFromOutcomes(sc, ruleOutcomes)
	signalResult := scenario.ResultFromOutcomes(sc, signalOutcomes)
	report := scenario.LockedTestReport{
		Rule: ruleResult, Signal: signalResult, ProfitUnits: profit,
		MaxDrawdownUnits: maxDrawdown, RevealedAt: now,
	}
	if len(ruleOutcomes) > 0 {
		report.ModelBrier = modelLoss / float64(len(ruleOutcomes))
		report.BaselineBrier = baselineLoss / float64(len(ruleOutcomes))
		report.QuoteCoverage = float64(ruleQuotes) / float64(len(ruleOutcomes))
		report.CalibrationError = calibrationError(occurrences, test.Contract.TargetOccurrences)
	}
	if len(signalOutcomes) > 0 {
		report.ROIPct = 100 * profit / float64(len(signalOutcomes))
	}
	if signalMarketCount > 0 {
		report.MarketBrier = marketLoss / float64(signalMarketCount)
	}
	target := test.Contract.TargetOccurrences
	if len(ruleOutcomes) < target {
		report.Failures = append(report.Failures, fmt.Sprintf("teste de regra %d < %d", len(ruleOutcomes), target))
	}
	if len(signalOutcomes) < target {
		report.Failures = append(report.Failures, fmt.Sprintf("teste de sinal %d < %d", len(signalOutcomes), target))
	}
	required := 1 / test.Contract.Definition.MinimumOdds
	if ruleResult.RateLow <= required {
		report.Failures = append(report.Failures,
			fmt.Sprintf("limite inferior bloqueado %.3f <= equilíbrio %.3f", ruleResult.RateLow, required))
	}
	if report.ModelBrier >= report.BaselineBrier {
		report.Failures = append(report.Failures,
			fmt.Sprintf("Brier do modelo %.5f >= baseline %.5f", report.ModelBrier, report.BaselineBrier))
	}
	sort.Strings(report.Failures)
	report.Qualified = len(report.Failures) == 0
	return report
}

func squaredLoss(probability, outcome float64) float64 {
	delta := probability - outcome
	return delta * delta
}

func calibrationError(occurrences []signals.LockedOccurrence, target int) float64 {
	type bin struct {
		count int
		pred  float64
		wins  int
	}
	var bins [10]bin
	used := 0
	for _, occurrence := range occurrences {
		if !occurrence.RuleCohort || used >= target {
			continue
		}
		index := int(math.Floor(occurrence.ModelProbability * 10))
		if index < 0 {
			index = 0
		}
		if index > 9 {
			index = 9
		}
		bins[index].count++
		bins[index].pred += occurrence.ModelProbability
		if occurrence.Status == signals.LockedOccurrenceWon {
			bins[index].wins++
		}
		used++
	}
	if used == 0 {
		return 0
	}
	var ece float64
	for _, bin := range bins {
		if bin.count == 0 {
			continue
		}
		averagePrediction := bin.pred / float64(bin.count)
		observed := float64(bin.wins) / float64(bin.count)
		ece += float64(bin.count) / float64(used) * math.Abs(averagePrediction-observed)
	}
	return ece
}
