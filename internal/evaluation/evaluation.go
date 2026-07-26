// Package evaluation computes real calibration/error metrics (Brier score,
// log loss, Expected Calibration Error) from stored predictions and their
// resolved outcomes. It has no dependency on eventstore or any other
// internal package besides domain — callers assemble the inputs.
//
// The reported top-level metrics evaluate the "goal before full time"
// horizon specifically, per the blueprint's North Star metric
// (context/Golo_Blueprint_MVP.md §3.3): Brier Score and calibration of
// P(goal before full time), not binary accuracy and not the shorter
// 5m/10m horizons.
package evaluation

import (
	"math"
	"strconv"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

// Sample is one (predicted probability, realized outcome) pair used for
// evaluation.
type Sample struct {
	Predicted float64
	Observed  int // 0 or 1
}

// CalibrationBin is one row of a reliability diagram.
type CalibrationBin struct {
	Bin       string  `json:"bin"`
	Predicted float64 `json:"predicted"`
	Observed  float64 `json:"observed"`
	Count     int     `json:"count"`
}

// Report is the full aggregate evaluation, matching the JSON shape the
// frontend's EvaluationMetrics type expects.
type Report struct {
	BrierScore float64 `json:"brierScore"`
	LogLoss    float64 `json:"logLoss"`
	ECE        float64 `json:"ece"`
	// HitRatePct is the simpler, user-facing "how often are we right"
	// figure: % of resolved goal-before-full-time calls (0.5 threshold)
	// that matched the actual outcome. Brier/LogLoss/ECE remain the
	// statistically rigorous metrics for the admin evaluation dashboard;
	// this one is for the plain-language "good at guessing" index shown
	// to end users.
	HitRatePct       float64          `json:"hitRatePct"`
	CalibrationCurve []CalibrationBin `json:"calibrationCurve"`
	// TotalSnapshots counts every prediction considered; ResolvedCount counts
	// only those whose outcome is actually known. They are wildly different
	// numbers and confusing them hides an empty measurement behind a large
	// one — a dashboard gated on TotalSnapshots will happily render 0.000
	// across the board as though the model were perfect.
	TotalSnapshots int `json:"totalSnapshots"`
	ResolvedCount  int `json:"resolvedCount"`
	// MatchCount is the effective sample size. Predictions within one match
	// are not independent observations — they share a single outcome.
	MatchCount      int     `json:"matchCount"`
	DataQualityAvg  float64 `json:"dataQualityAvg"`
	StaleFeedPct    float64 `json:"staleFeedPct"`
	ModelVersion    string  `json:"modelVersion"`
	FeatureVersion  string  `json:"featureVersion"`
	EvaluatedPeriod string  `json:"evaluatedPeriod"`
}

// TrackRecord is a single match's own recent-call accuracy for a specific
// horizon — the per-card "how right have we been on this match" index,
// distinct from the global HitRatePct above (which pools every match).
type TrackRecord struct {
	AccuracyPct   float64 `json:"accuracyPct"`
	ResolvedCount int     `json:"resolvedCount"`
}

// HitRate is the % of samples where the 0.5-threshold call ("yes" if
// Predicted >= 0.5) matched the actual outcome. Returns 0 for no samples —
// callers should treat ResolvedCount == 0 as "not enough history yet"
// rather than reading 0% as a real accuracy figure.
func HitRate(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0
	}
	correct := 0
	for _, s := range samples {
		predictedYes := s.Predicted >= 0.5
		actualYes := s.Observed == 1
		if predictedYes == actualYes {
			correct++
		}
	}
	return 100 * float64(correct) / float64(len(samples))
}

// BuildWindowSamples labels one match's own predictions for a fixed-window
// horizon (e.g. "goal in the next 10 minutes"): observed=1 if a goal occurs
// in (asOf, asOf+horizonSeconds].
//
// A fixed window resolves two ways, and both count: either the match has run
// far enough past the window to see the answer, or the match ended first, in
// which case the window closed early and whatever happened is final. Treating
// only the first as resolved would silently discard every prediction made in
// the closing minutes — which is exactly where goals are most likely.
func BuildWindowSamples(predictions []domain.Prediction, outcome MatchOutcome, horizonSeconds int, predicted func(domain.Probabilities) float64) []Sample {
	samples := make([]Sample, 0, len(predictions))

	for _, p := range predictions {
		windowEnd := p.AsOfMatchSecond + horizonSeconds
		resolved := outcome.FinalSecond >= windowEnd || outcome.Finished
		if !resolved {
			continue
		}

		// The window cannot outlive the match.
		until := windowEnd
		if outcome.FinalSecond < until {
			until = outcome.FinalSecond
		}
		if until <= p.AsOfMatchSecond {
			continue
		}

		samples = append(samples, Sample{
			Predicted: predicted(p.Probabilities),
			Observed:  goalAfter(outcome.GoalSeconds, p.AsOfMatchSecond, until),
		})
	}

	return samples
}

// ComputeMatchTrackRecord is the per-match "how right have we been" index
// shown on the live match card: hit-rate of this match's own past
// "goal in the next 10 minutes" calls that have resolved so far.
func ComputeMatchTrackRecord(predictions []domain.Prediction, outcome MatchOutcome) TrackRecord {
	const tenMinutes = 600
	samples := BuildWindowSamples(predictions, outcome, tenMinutes, func(p domain.Probabilities) float64 {
		return p.GoalNext10m
	})
	return TrackRecord{
		AccuracyPct:   HitRate(samples),
		ResolvedCount: len(samples),
	}
}

// MatchOutcome is everything the evaluator needs to know about one match:
// when goals happened, how far the match actually got, and — crucially —
// whether it is over.
//
// Finished is separate from FinalSecond because they answer different
// questions. FinalSecond says how much of the match has been observed, which
// is what a fixed window like "a goal in the next 10 minutes" needs. Finished
// says no further goal is possible, which is the only thing that resolves
// "a goal before full time". Conflating the two is what made every
// still-running match count as a settled no-goal.
type MatchOutcome struct {
	GoalSeconds []int
	FinalSecond int
	Finished    bool
}

// Evaluate builds the full Report from raw predictions and match outcomes.
// It never fails — an empty or not-yet-resolvable dataset simply
// produces a Report with zeroed metrics, since that's an honest reflection
// of "no evidence yet" rather than an error condition.
func Evaluate(predictions []domain.Prediction, outcomes map[string]MatchOutcome) Report {
	samples := BuildFullTimeSamples(predictions, outcomes)
	ece, curve := ECEAndCurve(samples, 5)
	if curve == nil {
		// Always serialize as [], never null — the frontend calls
		// .map() directly on calibrationCurve.
		curve = []CalibrationBin{}
	}

	report := Report{
		BrierScore:       BrierScore(samples),
		LogLoss:          LogLoss(samples),
		ECE:              ece,
		HitRatePct:       HitRate(samples),
		CalibrationCurve: curve,
		TotalSnapshots:   len(predictions),
		ResolvedCount:    len(samples),
		MatchCount:       countResolvedMatches(predictions, outcomes),
		EvaluatedPeriod:  time.Now().Format("2006-01-02"),
	}

	if len(predictions) == 0 {
		return report
	}

	var qualitySum float64
	staleCount := 0
	latest := predictions[0]
	for _, p := range predictions {
		qualitySum += p.DataQuality
		if p.Status == domain.PredictionStatusStale || p.Status == domain.PredictionStatusDegraded {
			staleCount++
		}
		if p.CalculatedAt.After(latest.CalculatedAt) {
			latest = p
		}
	}

	report.DataQualityAvg = qualitySum / float64(len(predictions))
	report.StaleFeedPct = 100 * float64(staleCount) / float64(len(predictions))
	report.ModelVersion = latest.ModelVersion
	report.FeatureVersion = latest.FeatureVersion

	return report
}

// BuildFullTimeSamples labels each prediction with whether a goal actually
// occurred after it, using the blueprint's definition (§5.1):
// y(t) = 1 if a goal occurs in (t, matchEnd].
//
// Only finished matches are included. "Will there be a goal before the final
// whistle?" has no answer until the final whistle: a match still in progress
// with no goal yet is not a wrong prediction, it is an unresolved one.
//
// This previously admitted any prediction the match had progressed a single
// second past, which scored every live match as a settled no-goal. That
// dragged the observed rate below the predicted rate in every bin, made the
// model look systematically overconfident, and inflated Brier and log loss —
// all while the number of samples looked reassuringly large.
func BuildFullTimeSamples(predictions []domain.Prediction, outcomes map[string]MatchOutcome) []Sample {
	samples := make([]Sample, 0, len(predictions))

	for _, p := range predictions {
		outcome, known := outcomes[p.MatchID]
		if !known || !outcome.Finished {
			continue
		}

		samples = append(samples, Sample{
			Predicted: p.Probabilities.GoalBeforeFullTime,
			Observed:  goalAfter(outcome.GoalSeconds, p.AsOfMatchSecond, outcome.FinalSecond),
		})
	}

	return samples
}

// goalAfter reports whether a goal fell in the half-open interval
// (fromSecond, untilSecond].
func goalAfter(goalSeconds []int, fromSecond, untilSecond int) int {
	for _, goalSecond := range goalSeconds {
		if goalSecond > fromSecond && goalSecond <= untilSecond {
			return 1
		}
	}
	return 0
}

// countResolvedMatches reports how many distinct matches contributed samples.
//
// It is reported alongside the sample count because the two differ by orders
// of magnitude and only one of them is the effective sample size: every
// prediction made during a match shares that match's single outcome, so a few
// thousand full-time samples drawn from eleven matches carry roughly eleven
// matches' worth of evidence, not eleven thousand.
func countResolvedMatches(predictions []domain.Prediction, outcomes map[string]MatchOutcome) int {
	seen := make(map[string]struct{})
	for _, p := range predictions {
		if outcome, known := outcomes[p.MatchID]; known && outcome.Finished {
			seen[p.MatchID] = struct{}{}
		}
	}
	return len(seen)
}

// BrierScore is the mean squared error between predicted probability and
// realized outcome. Returns 0 for an empty sample set.
func BrierScore(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		diff := s.Predicted - float64(s.Observed)
		sum += diff * diff
	}
	return sum / float64(len(samples))
}

// LogLoss is the binary cross-entropy loss, clamping predictions away from
// 0/1 to avoid taking log(0). Returns 0 for an empty sample set.
func LogLoss(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0
	}
	const eps = 1e-15
	var sum float64
	for _, s := range samples {
		p := math.Max(eps, math.Min(1-eps, s.Predicted))
		y := float64(s.Observed)
		sum += y*math.Log(p) + (1-y)*math.Log(1-p)
	}
	return -sum / float64(len(samples))
}

// ECEAndCurve computes the Expected Calibration Error and a reliability
// diagram over nBins equal-width probability bins in [0, 1].
func ECEAndCurve(samples []Sample, nBins int) (float64, []CalibrationBin) {
	if len(samples) == 0 || nBins <= 0 {
		return 0, nil
	}

	type bucket struct {
		sumPredicted float64
		sumObserved  int
		count        int
	}
	buckets := make([]bucket, nBins)

	for _, s := range samples {
		idx := int(s.Predicted * float64(nBins))
		if idx >= nBins {
			idx = nBins - 1
		}
		if idx < 0 {
			idx = 0
		}
		buckets[idx].sumPredicted += s.Predicted
		buckets[idx].sumObserved += s.Observed
		buckets[idx].count++
	}

	n := float64(len(samples))
	ece := 0.0
	curve := make([]CalibrationBin, 0, nBins)

	for i, b := range buckets {
		if b.count == 0 {
			continue
		}
		avgPredicted := b.sumPredicted / float64(b.count)
		avgObserved := float64(b.sumObserved) / float64(b.count)
		ece += (float64(b.count) / n) * math.Abs(avgObserved-avgPredicted)

		low := float64(i) / float64(nBins)
		high := float64(i+1) / float64(nBins)
		curve = append(curve, CalibrationBin{
			Bin:       formatBinLabel(low, high),
			Predicted: avgPredicted,
			Observed:  avgObserved,
			Count:     b.count,
		})
	}

	return ece, curve
}

// formatBinLabel renders e.g. (0.0, 0.2) as "0.0-0.2", matching the bin
// label style the frontend already renders.
func formatBinLabel(low, high float64) string {
	return strconv.FormatFloat(low, 'f', 1, 64) + "-" + strconv.FormatFloat(high, 'f', 1, 64)
}
