package scenario

import "math"

// z95 is the standard normal quantile for a two-sided 95% interval.
const z95 = 1.959963984540054

// wilsonInterval returns a 95% confidence interval for a hit rate observed
// as wins out of n.
//
// The Wilson score interval needs no special functions and behaves well
// exactly where the naive textbook
// interval fails: small samples, and rates near 0 or 1. The plain
// normal-approximation interval can run below zero or above one and badly
// undercovers at the extremes — a scenario that won 9 of 10 would get an
// upper bound above 100%, which is not a probability.
//
// The interval is the whole point of this package. A hit rate quoted without
// one invites betting at a price the evidence does not support: 25 wins in 50
// looks like exactly break-even odds of 2.00, while the interval puts the
// true break-even anywhere between 1.57 and 2.74.
func wilsonInterval(wins, n int) (low, high float64) {
	if n <= 0 {
		return 0, 1
	}

	phat := float64(wins) / float64(n)
	nf := float64(n)
	z2 := z95 * z95

	denom := 1 + z2/nf
	center := (phat + z2/(2*nf)) / denom
	spread := (z95 / denom) * math.Sqrt(phat*(1-phat)/nf+z2/(4*nf*nf))

	low = center - spread
	high = center + spread

	return clamp01(low), clamp01(high)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
