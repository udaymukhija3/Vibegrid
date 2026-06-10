package vibegrid

import "fmt"

// PuzzleBank is an evergreen set of editorial puzzles used to keep the daily
// running indefinitely. When no puzzle is explicitly scheduled for a date,
// bankPuzzleSource rotates through this bank by day (see bank_source.go), so the
// daily never runs dry without manual authoring. The bank is also a natural seed
// for the create-from-a-template flow.
//
// These are intentionally separate from any content exposed as create templates:
// the bank backs the *daily*, so its answers must never be served to clients
// except through the validated guess path.
func PuzzleBank() []Puzzle {
	return []Puzzle{
		bankPuzzle("bank-01", DifficultyMedium,
			bankGroup("bank-01-g0", "Sunday reset", "The optimistic start to the week.", 0,
				"meal prep", "clean sheets", "face mask", "fresh towels"),
			bankGroup("bank-01-g1", "Sunday scaries", "The dread that arrives by evening.", 1,
				"unread emails", "monday dread", "11pm panic", "where did it go"),
			bankGroup("bank-01-g2", "Lazy recovery", "What actually happens all afternoon.", 2,
				"3pm nap", "leftover pizza", "same hoodie", "six episodes"),
			bankGroup("bank-01-g3", "Productivity cosplay", "Looking busy, achieving little.", 3,
				"open laptop", "five tabs", "new notebook", "abandoned by noon"),
		),
		bankPuzzle("bank-02", DifficultyMedium,
			bankGroup("bank-02-g0", "Deep work cosplay", "Here to be productive, allegedly.", 0,
				"noise-cancelling", "third refill", "laptop fortress", "do not disturb"),
			bankGroup("bank-02-g1", "First date", "Two people performing ease.", 1,
				"nervous laugh", "shared scone", "phone away", "long pause"),
			bankGroup("bank-02-g2", "Remote meeting", "The call that should've been an email.", 2,
				"you're muted", "circle back", "share the link", "can you see"),
			bankGroup("bank-02-g3", "Friends catching up", "Nothing said, everything covered.", 3,
				"two-hour latte", "no you go", "wait what", "lean in"),
		),
		bankPuzzle("bank-03", DifficultyEasy,
			bankGroup("bank-03-g0", "The planner", "Keeps the group functional.", 0,
				"who's in", "sent a poll", "calendar invite", "just book it"),
			bankGroup("bank-03-g1", "The ghost", "Technically present.", 1,
				"thumbs up", "seen 2pm", "starts typing", "resurfaces later"),
			bankGroup("bank-03-g2", "The oversharer", "No detail too small.", 2,
				"long voice note", "three paragraphs", "you guys", "anyway so"),
			bankGroup("bank-03-g3", "The chaos", "Wild card energy.", 3,
				"random meme", "wrong chat", "47 messages", "lol what"),
		),
		bankPuzzle("bank-04", DifficultyHard,
			bankGroup("bank-04-g0", "Gate gremlin", "Boards before they're called.", 0,
				"hovers early", "group nine", "blocks the lane", "premature line"),
			bankGroup("bank-04-g1", "Duty-free daze", "Buying things you'll regret.", 1,
				"testing perfume", "giant toblerone", "why is this here", "just browsing"),
			bankGroup("bank-04-g2", "Delay despair", "The board turns red.", 2,
				"gate change", "floor outlet", "$9 water", "now boarding... not"),
			bankGroup("bank-04-g3", "Smug frequent flyer", "Has done this before.", 3,
				"one carry-on", "priority lane", "slip-on shoes", "lounge access"),
		),
		bankPuzzle("bank-05", DifficultyHard,
			bankGroup("bank-05-g0", "Replaying it", "The 2am highlight reel of regret.", 0,
				"that text", "said what", "ten years ago", "cringe rewind"),
			bankGroup("bank-05-g1", "Grand 2am plans", "Tomorrow's abandoned ambitions.", 1,
				"learn french", "5am club", "new startup", "sell everything"),
			bankGroup("bank-05-g2", "Existential dread", "The thoughts with no off switch.", 2,
				"heat death", "are we real", "tiny planet", "what's the point"),
			bankGroup("bank-05-g3", "Snack logistics", "The only solvable problem.", 3,
				"is there cheese", "quiet fridge", "one chip", "regret bite"),
		),
		bankPuzzle("bank-06", DifficultyMedium,
			bankGroup("bank-06-g0", "Looking busy", "Productivity theatre.", 0,
				"alt-tab", "walk with mug", "furrowed brow", "reply later"),
			bankGroup("bank-06-g1", "Meeting bingo", "Words that mean nothing.", 1,
				"synergy", "circle back", "low-hanging", "take it offline"),
			bankGroup("bank-06-g2", "Kitchen politics", "Cold war over the fridge.", 2,
				"whose milk", "passive note", "last coffee", "mystery smell"),
			bankGroup("bank-06-g3", "Friday wind-down", "Clocking out in spirit.", 3,
				"camera off", "slow replies", "soft logout", "see you monday"),
		),
		bankPuzzle("bank-07", DifficultyHard,
			bankGroup("bank-07-g0", "We're fine", "Genuinely relaxed texting.", 0,
				"lowercase hey", "haha", "sure!", "sounds good"),
			bankGroup("bank-07-g1", "We are not fine", "Punctuation as a weapon.", 1,
				"period.", "k.", "fine.", "one word"),
			bankGroup("bank-07-g2", "Overthinking it", "Drafting and redrafting.", 2,
				"typing dots", "drafted twice", "deleted that", "double text"),
			bankGroup("bank-07-g3", "Unhinged friend", "No notes, all chaos.", 3,
				"nine texts", "all caps", "voice memo", "no context"),
		),
		bankPuzzle("bank-08", DifficultyEasy,
			bankGroup("bank-08-g0", "New year hopeful", "Motivation has an expiry date.", 0,
				"bought shoes", "day three", "sore everywhere", "where's the locker"),
			bankGroup("bank-08-g1", "The regular", "Owns the place, basically.", 1,
				"own towel", "nods only", "claimed rack", "6am sharp"),
			bankGroup("bank-08-g2", "Mirror crew", "More phone than reps.", 2,
				"flex check", "tank top", "phone propped", "between-set selfie"),
			bankGroup("bank-08-g3", "Cardio escapist", "Just here to disassociate.", 3,
				"podcast on", "incline walk", "just sweating", "leaves early"),
		),
		bankPuzzle("bank-09", DifficultyMedium,
			bankGroup("bank-09-g0", "Kitchen dweller", "Will not leave the snacks.", 0,
				"near the dip", "deep talk", "holds the bowl", "won't move"),
			bankGroup("bank-09-g1", "The connector", "Knows literally everyone.", 1,
				"have you met", "pulls you in", "knows everyone", "name swap"),
			bankGroup("bank-09-g2", "Early ghost", "Gone without a trace.", 2,
				"irish exit", "suddenly gone", "no goodbye", "text from car"),
			bankGroup("bank-09-g3", "Last to leave", "Closing the place down.", 3,
				"helps clean", "one more song", "lingering chat", "lights still on"),
		),
		bankPuzzle("bank-10", DifficultyMedium,
			bankGroup("bank-10-g0", "Cart guilt", "Do I really need this?", 0,
				"added twelve", "removed eleven", "do i need", "sleep on it"),
			bankGroup("bank-10-g1", "Sale brain", "Logic leaves the building.", 1,
				"70% off", "basket full", "free shipping", "ends tonight"),
			bankGroup("bank-10-g2", "Delivery watch", "Refreshing the tracker.", 2,
				"out for delivery", "where's the van", "left at door", "two stops away"),
			bankGroup("bank-10-g3", "Return spiral", "It didn't fit, again.", 3,
				"didn't fit", "repack it", "lost receipt", "maybe keep"),
		),
		bankPuzzle("bank-11", DifficultyHard,
			bankGroup("bank-11-g0", "Open bar arc", "A predictable trajectory.", 0,
				"pacing myself", "third drink", "found the tray", "slow down"),
			bankGroup("bank-11-g1", "Dance floor", "The DJ finally delivered.", 1,
				"cha cha slide", "heels off", "conga line", "last song"),
			bankGroup("bank-11-g2", "Small talk loop", "Same five questions.", 2,
				"how do you know", "lovely venue", "the couple", "table nine"),
			bankGroup("bank-11-g3", "Logistics nerd", "Read the whole invite.", 3,
				"seating chart", "shuttle time", "plus one", "hotel block"),
		),
		bankPuzzle("bank-12", DifficultyHard,
			bankGroup("bank-12-g0", "Productive avoidance", "Cleaning to dodge the real task.", 0,
				"deep clean", "reorganize shelf", "anything but", "alphabetize"),
			bankGroup("bank-12-g1", "The chair", "Not dirty, not clean.", 1,
				"worn once", "not dirty", "not clean", "the pile"),
			bankGroup("bank-12-g2", "Doom drawer", "Where small things go to die.", 2,
				"dead batteries", "mystery key", "old cables", "takeout menu"),
			bankGroup("bank-12-g3", "Adulting wins", "Small victories, big pride.", 3,
				"paid the bill", "watered plant", "made the bed", "called back"),
		),
	}
}

// bankPuzzle builds a bank entry. The top-level id/status/origin are placeholders
// — bankPuzzleSource stamps a date-specific id, publish date, and number when it
// serves an entry as the daily. The group and tile ids, however, are stable and
// used by attempts/guesses, so they must stay unique within the puzzle.
func bankPuzzle(id string, difficulty Difficulty, groups ...PuzzleGroup) Puzzle {
	return Puzzle{
		ID:         id,
		Status:     PuzzleStatusPublished,
		Origin:     OriginEditorial,
		Difficulty: difficulty,
		Groups:     groups,
	}
}

// bankGroup builds a group, deriving stable tile ids from the group id so each
// bank entry stays terse and the ids never collide within a puzzle.
func bankGroup(id, name, explanation string, colorIndex int, tiles ...string) PuzzleGroup {
	built := make([]Tile, len(tiles))
	for index, text := range tiles {
		built[index] = Tile{ID: fmt.Sprintf("%s-t%d", id, index), Text: text}
	}
	return PuzzleGroup{ID: id, Name: name, Explanation: explanation, ColorIndex: colorIndex, Tiles: built}
}
