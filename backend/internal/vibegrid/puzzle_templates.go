package vibegrid

// PuzzleTemplate is a ready-made starter for the "make your own" flow. Unlike the
// daily PuzzleBank, templates are fully exposed to clients (that is their whole
// point), so they are kept as a SEPARATE content set: nothing here ever feeds the
// daily, so serving template answers can never spoil a daily puzzle.
//
// The shape mirrors the create form's draft input (difficulty + groups of
// name/explanation/tiles), so the client can either submit a template as-is
// ("play this") or load it into the builder to tweak ("use as template").
type PuzzleTemplate struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Difficulty Difficulty      `json:"difficulty"`
	Groups     []TemplateGroup `json:"groups"`
}

type TemplateGroup struct {
	Name        string   `json:"name"`
	Explanation string   `json:"explanation"`
	Tiles       []string `json:"tiles"`
}

// PuzzleTemplates returns the curated starter packs offered on the create page.
func PuzzleTemplates() []PuzzleTemplate {
	return []PuzzleTemplate{
		template("tpl-01", "Coffee order energy", DifficultyMedium,
			tgroup("Main character", "Here for the aesthetic.",
				"oat milk matcha", "oversized cup", "corner seat", "journaling"),
			tgroup("No-nonsense", "Caffeine, no ceremony.",
				"black coffee", "large", "to go", "no name"),
			tgroup("Secretly dessert", "Coffee in name only.",
				"extra whip", "caramel drizzle", "three pumps", "venti"),
			tgroup("Decision paralysis", "Holding up the line.",
				"um", "what's good", "surprise me", "still deciding"),
		),
		template("tpl-02", "Types of crying", DifficultyEasy,
			tgroup("Happy tears", "The good kind.",
				"wedding speech", "dog reunion", "proud parent", "finally won"),
			tgroup("Stress cry", "It all piled up.",
				"car cry", "bathroom break", "deadline", "too much"),
			tgroup("Media meltdown", "It's just a movie.",
				"pixar opening", "that one ad", "season finale", "sad song"),
			tgroup("Ugly sob", "No dignity left.",
				"can't breathe", "snot phase", "the hiccups", "full body"),
		),
		template("tpl-03", "Roommate red flags", DifficultyMedium,
			tgroup("Kitchen crimes", "The sink situation.",
				"dish tower", "my leftovers", "mystery smell", "no soap left"),
			tgroup("Ghost tenant", "Do they live here?",
				"never home", "where's rent", "locked door", "who are they"),
			tgroup("Too much", "No sense of space.",
				"borrows clothes", "walks right in", "deep talks", "no knock"),
			tgroup("Passive war", "Conflict by note.",
				"sticky notes", "the group text", "labeled food", "loud sighs"),
		),
		template("tpl-04", "Vacation modes", DifficultyMedium,
			tgroup("Itinerary tyrant", "Fun, on a schedule.",
				"color-coded", "7am tour", "no free time", "spreadsheet"),
			tgroup("Pure rot", "Doing nothing, perfectly.",
				"poolside", "all-inclusive", "third nap", "no shoes"),
			tgroup("Culture grind", "Every museum, all of it.",
				"every museum", "ten miles walked", "local food", "sore feet"),
			tgroup("Chaos trip", "It went wrong, gloriously.",
				"missed flight", "lost passport", "wrong hotel", "great story"),
		),
		template("tpl-05", "Desk archetypes", DifficultyEasy,
			tgroup("Minimalist", "Suspiciously empty.",
				"one plant", "clear top", "single notebook", "no cables"),
			tgroup("Snack station", "More pantry than desk.",
				"crumb field", "energy drinks", "gum stash", "mug zoo"),
			tgroup("Paper avalanche", "A filing system, allegedly.",
				"sticky forest", "old receipts", "dead pens", "where's the thing"),
			tgroup("The shrine", "Personality on display.",
				"figurines", "framed pet", "fairy lights", "joke mug"),
		),
		template("tpl-06", "Plans that died", DifficultyHard,
			tgroup("Flaked politely", "Maybe next time.",
				"rain check", "next week", "so tired", "soon i promise"),
			tgroup("Never real", "Was it ever happening?",
				"we should", "one day", "let's plan", "stay in touch"),
			tgroup("Last-minute out", "The classic excuses.",
				"sudden headache", "car trouble", "work thing", "my bad"),
			tgroup("Lowkey relieved", "Best night in.",
				"canceled plans", "free evening", "pajamas on", "phone off"),
		),
	}
}

func template(id, title string, difficulty Difficulty, groups ...TemplateGroup) PuzzleTemplate {
	return PuzzleTemplate{ID: id, Title: title, Difficulty: difficulty, Groups: groups}
}

func tgroup(name, explanation string, tiles ...string) TemplateGroup {
	return TemplateGroup{Name: name, Explanation: explanation, Tiles: tiles}
}
