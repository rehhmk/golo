// Command scenarios measures how often specific in-match situations are
// followed by another goal, across the historical dataset.
//
// It answers the only question that decides whether a situation is worth
// betting: what price would you need, and is the sample big enough to trust
// that price?
//
//	go run ./cmd/scenarios
//	go run ./cmd/scenarios -dataset ml/data/fixtures.jsonl -drawdown 0.20
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/enzotriches/golo/internal/scenario"
)

func main() {
	dataset := flag.String("dataset", "ml/data/fixtures.jsonl", "caminho do dataset histórico")
	drawdown := flag.Float64("drawdown", 0.20, "fração da banca que você aceita perder numa sequência ruim")
	targetOdds := flag.Float64("odds", 1.50, "odd que você consegue na prática, para julgar cada cenário")
	flag.Parse()

	matches, err := scenario.LoadTimelines(*dataset)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	goals := 0
	for _, m := range matches {
		goals += len(m.Goals)
	}
	fmt.Printf("Dataset: %d partidas, %d gols (%.2f por partida)\n", len(matches), goals, float64(goals)/float64(len(matches)))
	fmt.Printf("Odd alvo: %.2f  (ponto de equilíbrio: acerto de %.1f%%)\n\n", *targetOdds, 100/(*targetOdds))

	results := make([]scenario.Result, 0, len(scenario.Library))
	for _, sc := range scenario.Library {
		results = append(results, scenario.Evaluate(sc, matches))
	}

	// Most promising first, but the verdict column is what decides.
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].RateLow > results[j].RateLow
	})

	fmt.Printf("%-26s %8s %7s %7s %17s %9s %6s\n",
		"cenário", "n", "partid", "acerto", "IC 95% do acerto", "odd mín.", "pior")
	fmt.Println(rule(92))

	for _, r := range results {
		if r.Occurrences == 0 {
			fmt.Printf("%-26s %8s\n", r.Scenario.Name, "nunca")
			continue
		}
		fmt.Printf("%-26s %8d %7d %6.1f%% %7.1f%% - %-6.1f%% %9s %6d\n",
			r.Scenario.Name, r.Occurrences, r.MatchCount, r.HitRate*100,
			r.RateLow*100, r.RateHigh*100, oddsCell(r.OddsHigh), r.LongestLossStreak)
	}

	fmt.Println()
	fmt.Println("Leitura das colunas:")
	fmt.Println("  acerto    — frequência observada de mais um gol depois do gatilho")
	fmt.Println("  IC 95%    — onde a taxa real provavelmente está; estreita conforme n cresce")
	fmt.Println("  odd mín.  — preço necessário no pior caso do intervalo (1 / limite inferior)")
	fmt.Println("  pior      — maior sequência de perdas realmente observada")
	fmt.Println()

	verdicts(results, *targetOdds, *drawdown)
}

// verdicts reports which scenarios clear the target price on the pessimistic
// bound, which is the only bar that means anything.
func verdicts(results []scenario.Result, targetOdds, drawdown float64) {
	breakEven := 1 / targetOdds

	fmt.Printf("=== Veredito para odd %.2f ===\n\n", targetOdds)

	any := false
	for _, r := range results {
		if r.Occurrences == 0 {
			continue
		}
		// The pessimistic end of the interval has to clear break-even. Using
		// the point estimate instead is how a coin-flip gets mistaken for an
		// edge.
		if r.RateLow > breakEven {
			any = true
			stake := scenario.StakeFraction(drawdown, r.LongestLossStreak)
			fmt.Printf("  ✓ %-24s acerto %.1f%% (mínimo provável %.1f%%) — supera o equilíbrio de %.1f%%\n",
				r.Scenario.Name, r.HitRate*100, r.RateLow*100, breakEven*100)
			fmt.Printf("    %-24s banca: %.2f%% por entrada (%.0f%% de drawdown ÷ %d perdas seguidas)\n\n",
				"", stake*100, drawdown*100, maxInt(r.LongestLossStreak, 1))
		}
	}

	if !any {
		fmt.Printf("  Nenhum cenário supera o equilíbrio de %.1f%% com 95%% de confiança.\n\n", breakEven*100)
		fmt.Println("  Isso não é uma falha da medição — é a resposta. Nesta odd, apostar")
		fmt.Println("  qualquer um destes cenários é esperar prejuízo no longo prazo.")
		fmt.Println()

		best := bestCandidate(results, breakEven)
		if best != nil {
			needed := scenario.MinimumSampleFor(best.HitRate, targetOdds)
			if needed > 0 {
				fmt.Printf("  Mais perto: %s, com acerto de %.1f%%. Se essa taxa for real,\n",
					best.Scenario.Name, best.HitRate*100)
				fmt.Printf("  seriam necessárias ~%d ocorrências para provar (tem %d).\n",
					needed, best.Occurrences)
			} else {
				fmt.Printf("  Mais perto: %s, com acerto de %.1f%% — ainda abaixo do equilíbrio\n",
					best.Scenario.Name, best.HitRate*100)
				fmt.Printf("  de %.1f%%, então mais amostra não resolveria.\n", breakEven*100)
			}
		}
	}
}

// bestCandidate is the scenario with the highest observed rate, whether or
// not it clears the bar.
func bestCandidate(results []scenario.Result, breakEven float64) *scenario.Result {
	var best *scenario.Result
	for i := range results {
		if results[i].Occurrences == 0 || results[i].Scenario.Name == "baseline" {
			continue
		}
		if best == nil || results[i].HitRate > best.HitRate {
			best = &results[i]
		}
	}
	return best
}

func oddsCell(odds float64) string {
	if math.IsInf(odds, 1) {
		return "—"
	}
	return fmt.Sprintf("%.2f", odds)
}

func rule(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = '-'
	}
	return string(out)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
