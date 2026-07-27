package scenario

// competitionNames maps the provider's numeric league IDs to readable names.
//
// The dataset stores only the ID, because that is what the provider returns
// and what a stored scenario must keep referring to — a league can be renamed
// without its identity changing. The display name lives here so a stale name
// can never invalidate a saved scenario or a sealed test.
//
// An unknown ID is shown as-is rather than hidden: a competition missing from
// this map is still real data the user is measuring against, and silently
// dropping it from the picker would under-report the sample.
var competitionNames = map[string]string{
	"2":    "Champions League",
	"5":    "Europa League",
	"271":  "Superliga (DIN)",
	"648":  "Brasileirão Série A",
	"743":  "Liga MX",
	"779":  "Major League Soccer",
	"1122": "Copa Libertadores",
	"1328": "Supercopa da UEFA",
	"2286": "Conference League",
}

// CompetitionName returns a readable name, falling back to the raw ID.
func CompetitionName(id string) string {
	if name, ok := competitionNames[id]; ok {
		return name
	}
	return "Liga " + id
}
