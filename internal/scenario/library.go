package scenario

// Library is the set of scenarios measured by default.
//
// Each is deliberately specific. A vague trigger ("the match is open") fires
// constantly and measures nothing beyond the base rate of football; the whole
// premise of scenario betting is that a narrow, well-chosen situation departs
// from that base rate. Whether any of these actually do is what the numbers
// are for — and "none of them" is a legitimate answer.
var Library = []Scenario{
	{
		Name:        "baseline",
		Description: "Qualquer instante da partida — a taxa base contra a qual todo cenário deve ser comparado",
		Trigger:     func(State) bool { return true },
	},
	{
		Name:        "70min-vencendo-por-2",
		Description: "A partir dos 70min, um time vencendo por 2 ou mais (cenário do vídeo)",
		Trigger: func(s State) bool {
			return s.Second >= 70*60 && s.AbsScoreDiff() >= 2
		},
	},
	{
		Name:        "70min-empate",
		Description: "A partir dos 70min, jogo empatado",
		Trigger: func(s State) bool {
			return s.Second >= 70*60 && s.ScoreDiff() == 0
		},
	},
	{
		Name:        "70min-diferenca-1",
		Description: "A partir dos 70min, diferença de exatamente 1 gol",
		Trigger: func(s State) bool {
			return s.Second >= 70*60 && s.AbsScoreDiff() == 1
		},
	},
	{
		Name:        "expulsao-antes-60min",
		Description: "Time com um a menos, antes dos 60min",
		Trigger: func(s State) bool {
			return s.Second <= 60*60 && s.RedHome+s.RedAway >= 1
		},
	},
	{
		Name:        "0x0-apos-60min",
		Description: "Sem gols após os 60min",
		Trigger: func(s State) bool {
			return s.Second >= 60*60 && s.GoalsSoFar == 0
		},
	},
	{
		Name:        "primeiro-tempo-aberto",
		Description: "Dois ou mais gols antes do intervalo",
		Trigger: func(s State) bool {
			return s.Second >= 45*60 && s.Second <= 60*60 && s.GoalsSoFar >= 2
		},
	},
	{
		Name:        "over25-ao-vivo",
		Description: "Placar em 2 gols — mais um resolve o over 2.5",
		Trigger: func(s State) bool {
			return s.GoalsSoFar == 2
		},
	},
	{
		Name:        "over15-ao-vivo",
		Description: "Placar em 1 gol — mais um resolve o over 1.5",
		Trigger: func(s State) bool {
			return s.GoalsSoFar == 1
		},
	},
	{
		Name:        "ultimos-15min-empate",
		Description: "Últimos 15min com jogo empatado",
		Trigger: func(s State) bool {
			return s.Remaining <= 15*60 && s.ScoreDiff() == 0
		},
	},
}
