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
	TotalSnapshots   int              `json:"totalSnapshots"`
	DataQualityAvg   float64          `json:"dataQualityAvg"`
	StaleFeedPct     float64          `json:"staleFeedPct"`
	ModelVersion     string           `json:"modelVersion"`
	FeatureVersion   string           `json:"featureVersion"`
	EvaluatedPeriod  string           `json:"evaluatedPeriod"`
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
// in (asOf, asOf+horizonSeconds]. Only includes predictions where the match
// has progressed far enough past that window to know the true outcome
// (endSecond >= asOf+horizonSeconds) — otherwise the call isn't resolved yet.
func BuildWindowSamples(predictions []domain.Prediction, goalSeconds []int, endSecond int, horizonSeconds int, predicted func(domain.Probabilities) float64) []Sample {
	samples := make([]Sample, 0, len(predictions))

	for _, p := range predictions {
		windowEnd := p.AsOfMatchSecond + horizonSeconds
		if endSecond < windowEnd {
			continue // not resolved yet
		}

		observed := 0
		for _, goalSecond := range goalSeconds {
			if goalSecond > p.AsOfMatchSecond && goalSecond <= windowEnd {
				observed = 1
				break
			}
		}

		samples = append(samples, Sample{Predicted: predicted(p.Probabilities), Observed: observed})
	}

	return samples
}

// ComputeMatchTrackRecord is the per-match "how right have we been" index
// shown on the live match card: hit-rate of this match's own past
// "goal in the next 10 minutes" calls that have resolved so far.
func ComputeMatchTrackRecord(predictions []domain.Prediction, goalSeconds []int, endSecond int) TrackRecord {
	const tenMinutes = 600
	samples := BuildWindowSamples(predictions, goalSeconds, endSecond, tenMinutes, func(p domain.Probabilities) float64 {
		return p.GoalNext10m
	})
	return TrackRecord{
		AccuracyPct:   HitRate(samples),
		ResolvedCount: len(samples),
	}
}

// Evaluate builds the full Report from raw predictions and resolved goal
// events. It never fails — an empty or not-yet-resolvable dataset simply
// produces a Report with zeroed metrics, since that's an honest reflection
// of "no evidence yet" rather than an error condition.
func Evaluate(predictions []domain.Prediction, goalSeconds map[string][]int, endSeconds map[string]int) Report {
	samples := BuildFullTimeSamples(predictions, goalSeconds, endSeconds)
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
// y(t) = 1 if a goal occurs in (t, matchEnd]. A prediction is only included
// once the match has progressed past its as-of instant (endSecond > asOf) —
// otherwise the outcome isn't resolved yet and including it would bias the
// metric toward "no goal yet" for matches still in progress.
func BuildFullTimeSamples(predictions []domain.Prediction, goalSeconds map[string][]int, endSeconds map[string]int) []Sample {
	samples := make([]Sample, 0, len(predictions))

	for _, p := range predictions {
		endSecond, known := endSeconds[p.MatchID]
		if !known || endSecond <= p.AsOfMatchSecond {
			continue
		}

		observed := 0
		for _, goalSecond := range goalSeconds[p.MatchID] {
			if goalSecond > p.AsOfMatchSecond {
				observed = 1
				break
			}
		}

		samples = append(samples, Sample{
			Predicted: p.Probabilities.GoalBeforeFullTime,
			Observed:  observed,
		})
	}

	return samples
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
