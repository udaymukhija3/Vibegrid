package vibegrid

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	VibeBoardColumns      = 4
	VibeBoardMinRows      = 3
	VibeBoardMaxRows      = 7
	VibePracticeRows      = 4
	VibeBoardMinTileCount = VibeBoardColumns * VibeBoardMinRows
	VibeBoardMaxTileCount = VibeBoardColumns * VibeBoardMaxRows
	VibePracticeTileCount = VibeBoardColumns * VibePracticeRows
	VibeCardTileCount     = 4
	MaxVibePromptRunes    = 140
	MaxVibeTileRunes      = 28
)

var ErrVibeBoardInvalid = errors.New("vibe board is invalid")

// VibeBoard has no hidden partition or answer key. The stored master palette
// has enough fragments for every supported crew size. A crew receives a
// four-column prefix whose row count is frozen for that crew and date.
type VibeBoard struct {
	ID          string `json:"id"`
	BoardNumber int    `json:"boardNumber"`
	PublishDate string `json:"publishDate"`
	Prompt      string `json:"prompt"`
	Tiles       []Tile `json:"tiles"`
}

type vibeBoardTemplate struct {
	prompt string
	tiles  [VibeBoardMaxTileCount]string
}

var vibeBoardTemplates = []vibeBoardTemplate{
	{
		prompt: "Build a week that is technically under control.",
		tiles: [VibeBoardMaxTileCount]string{
			"meal prep", "unread emails", "3pm nap", "open laptop",
			"clean sheets", "monday dread", "leftover pizza", "five tabs",
			"face mask", "11pm panic", "same hoodie", "new notebook",
			"color-coded list", "forgot password", "calendar block", "empty water bottle",
			"sunday alarm", "laundry chair", "quick grocery run", "rescheduled again",
			"desk snack", "battery at 6%", "reply tomorrow", "clean mug",
			"morning walk", "one good meeting", "closed one tab", "planned a plan",
		},
	},
	{
		prompt: "Build a first impression that will not survive the evening.",
		tiles: [VibeBoardMaxTileCount]string{
			"noise-cancelling", "nervous laugh", "you're muted", "two-hour latte",
			"third refill", "shared scone", "circle back", "no you go",
			"laptop fortress", "phone away", "share the link", "wait what",
			"perfect posture", "too much context", "early by twelve", "knows the bartender",
			"forgot your name", "tiny notebook", "specific compliment", "overdressed",
			"checks the door", "says fascinating", "orders for table", "missed the joke",
			"fresh haircut", "offers a charger", "one fun fact", "leaves at nine",
		},
	},
	{
		prompt: "Build the person every group chat eventually creates.",
		tiles: [VibeBoardMaxTileCount]string{
			"who's in", "thumbs up", "long voice note", "random meme",
			"sent a poll", "seen 2pm", "three paragraphs", "wrong chat",
			"calendar invite", "starts typing", "you guys", "47 messages",
			"pins nothing", "birthday reminder", "late reaction", "screenshots it",
			"changes the name", "weather update", "accidental sticker", "asks for recap",
			"brings receipts", "morning everyone", "sends location", "inside joke",
			"muted a year", "types then vanishes", "checks the date", "calls instead",
		},
	},
	{
		prompt: "Build an airport personality you would deny having.",
		tiles: [VibeBoardMaxTileCount]string{
			"hovers early", "testing perfume", "gate change", "one carry-on",
			"group nine", "giant toblerone", "floor outlet", "priority lane",
			"blocks the lane", "why is this here", "$9 water", "slip-on shoes",
			"shoes already off", "passport check", "window plea", "neck pillow",
			"wrong terminal", "snack inventory", "charging at 12%", "luggage scale",
			"tracks the plane", "aisle stretch", "last boarding call", "empty bottle",
			"socks in public", "buys a novel", "seat map expert", "claps on landing",
		},
	},
	{
		prompt: "Build the 2am version of a reasonable person.",
		tiles: [VibeBoardMaxTileCount]string{
			"that text", "learn french", "heat death", "is there cheese",
			"said what", "5am club", "are we real", "quiet fridge",
			"ten years ago", "new startup", "tiny planet", "one chip",
			"old wikipedia", "ceiling crack", "one more episode", "delete the app",
			"send apology", "parallel universe", "cold leftovers", "screen at 1%",
			"career pivot", "water tastes loud", "ancient memory", "buy a lamp",
			"tomorrow for sure", "search symptoms", "missed chance", "alarm math",
		},
	},
	{
		prompt: "Build a workday with no measurable output.",
		tiles: [VibeBoardMaxTileCount]string{
			"alt-tab", "synergy", "whose milk", "camera off",
			"walk with mug", "circle back", "passive note", "slow replies",
			"furrowed brow", "low-hanging", "last coffee", "soft logout",
			"status green", "meeting notes", "open spreadsheet", "desk lap",
			"reply all", "project tracker", "new tab", "lunch research",
			"quick sync", "muted sigh", "rename final v2", "keyboard noise",
			"book a room", "one easy task", "end of day", "tomorrow morning",
		},
	},
	{
		prompt: "Build a text conversation that is definitely fine.",
		tiles: [VibeBoardMaxTileCount]string{
			"lowercase hey", "period.", "typing dots", "nine texts",
			"haha", "k.", "drafted twice", "all caps",
			"sure!", "fine.", "deleted that", "voice memo",
			"read receipt", "no worries", "double question mark", "reacted heart",
			"unsent message", "maybe later", "long pause", "new subject",
			"thumbs up", "can we talk", "wrong emoji", "screenshot taken",
			"okayyy", "call me", "left on read", "goodnight then",
		},
	},
	{
		prompt: "Build a wellness era with a short expiry date.",
		tiles: [VibeBoardMaxTileCount]string{
			"bought shoes", "own towel", "flex check", "podcast on",
			"day three", "nods only", "tank top", "incline walk",
			"sore everywhere", "claimed rack", "phone propped", "just sweating",
			"green powder", "sleep score", "ice bath", "morning pages",
			"almond butter", "breathwork", "habit tracker", "sunrise alarm",
			"new blender", "ten thousand steps", "phone on grayscale", "magnesium",
			"meal photo", "rest day", "cancelled brunch", "forgot the bottle",
		},
	},
	{
		prompt: "Build the person quietly controlling the party.",
		tiles: [VibeBoardMaxTileCount]string{
			"near the dip", "have you met", "irish exit", "helps clean",
			"deep talk", "pulls you in", "suddenly gone", "one more song",
			"holds the bowl", "knows everyone", "no goodbye", "lingering chat",
			"controls aux", "water round", "opens a window", "takes coats",
			"knows the neighbor", "finds more cups", "dims the lights", "orders late food",
			"fixes the playlist", "starts a game", "moves the lamp", "checks the time",
			"introduces exes", "saves the cake", "calls the ride", "locks the door",
		},
	},
	{
		prompt: "Build an online purchase you will defend in public.",
		tiles: [VibeBoardMaxTileCount]string{
			"added twelve", "70% off", "out for delivery", "didn't fit",
			"removed eleven", "basket full", "where's the van", "repack it",
			"do i need", "free shipping", "left at door", "lost receipt",
			"watched review", "limited edition", "wrong colour", "used once",
			"tiny upgrade", "payment plan", "influencer code", "tracking refresh",
			"box too large", "assembly video", "better in theory", "forgot return date",
			"backup option", "custom engraving", "five star photo", "closet evidence",
		},
	},
	{
		prompt: "Build the full emotional arc of a wedding guest.",
		tiles: [VibeBoardMaxTileCount]string{
			"pacing myself", "cha cha slide", "how do you know", "seating chart",
			"third drink", "heels off", "lovely venue", "shuttle time",
			"found the tray", "conga line", "the couple", "plus one",
			"ceremony tears", "table nine", "speech bingo", "photo booth",
			"name tag panic", "cake timing", "lost boutonniere", "dance floor circle",
			"late RSVP", "borrowed tissues", "sparkler queue", "family lore",
			"gift envelope", "morning after", "one safe song", "missed the bus",
		},
	},
	{
		prompt: "Build a home that has given up on categories.",
		tiles: [VibeBoardMaxTileCount]string{
			"deep clean", "worn once", "dead batteries", "paid the bill",
			"reorganize shelf", "not dirty", "mystery key", "watered plant",
			"anything but", "the pile", "old cables", "made the bed",
			"chair wardrobe", "junk drawer", "mug collection", "plant hospital",
			"delivery boxes", "guest towel", "spare button", "manuals folder",
			"fridge magnet", "one good pan", "unread mail", "shoe corner",
			"bag of bags", "candle stash", "remote unknown", "holiday lights",
		},
	},
}

// VibeBoardForDate deterministically selects editorial ingredients, then stamps
// a date-specific id. Persisted stores keep the first snapshot for a date, so
// editing this bank later cannot rewrite an already-played round.
func VibeBoardForDate(date string) (VibeBoard, error) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return VibeBoard{}, ErrVibeBoardInvalid
	}
	launch, _ := time.Parse("2006-01-02", "2026-08-19")
	days := int(parsed.Sub(launch).Hours() / 24)
	index := days % len(vibeBoardTemplates)
	if index < 0 {
		index += len(vibeBoardTemplates)
	}
	number := 47 + days
	if number < 1 {
		number = int(parsed.Unix()/86400) + 1
	}

	template := vibeBoardTemplates[index]
	boardID := "vibe-" + date
	tiles := make([]Tile, VibeBoardMaxTileCount)
	for tileIndex, text := range template.tiles {
		tiles[tileIndex] = Tile{ID: bankTileID(boardID, tileIndex), Text: text}
	}
	board := VibeBoard{
		ID:          boardID,
		BoardNumber: number,
		PublishDate: date,
		Prompt:      template.prompt,
		Tiles:       tiles,
	}
	if err := validateVibeBoard(board); err != nil {
		return VibeBoard{}, err
	}
	return board, nil
}

// UnlimitedVibeBoard deals a deterministic 4x4 practice palette without
// reading or mutating durable crew state. The sequence chooses one coherent
// 28-fragment editorial master, then varies its rotation and coprime stride.
// That keeps every visible fragment on-prompt while allowing the client to
// request another round indefinitely; content eventually cycles and is never
// represented as a newly authored daily board.
func UnlimitedVibeBoard(sequence uint64) (VibeBoard, error) {
	templateCount := uint64(len(vibeBoardTemplates))
	templateIndex := int(sequence % templateCount)
	variation := sequence / templateCount
	rotation := int(variation % VibeBoardMaxTileCount)
	strides := [...]int{1, 3, 5, 9, 11, 13, 15, 17, 19, 23, 25, 27}
	stride := strides[int(variation/uint64(VibeBoardMaxTileCount))%len(strides)]
	template := vibeBoardTemplates[templateIndex]

	boardID := fmt.Sprintf("unlimited-%d", sequence)
	tiles := make([]Tile, VibePracticeTileCount)
	for index := range tiles {
		masterIndex := (rotation + index*stride) % VibeBoardMaxTileCount
		tiles[index] = Tile{
			ID:   fmt.Sprintf("%s-tile-%02d", boardID, index+1),
			Text: template.tiles[masterIndex],
		}
	}
	publishDate := time.Date(2026, time.August, 19, 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, templateIndex).Format("2006-01-02")
	board := VibeBoard{
		ID:          boardID,
		BoardNumber: int(sequence%1_000_000_000) + 1,
		PublishDate: publishDate,
		Prompt:      template.prompt,
		Tiles:       tiles,
	}
	if err := validateVibeBoard(board); err != nil {
		return VibeBoard{}, err
	}
	return board, nil
}

func validateVibeBoard(board VibeBoard) error {
	if board.ID == "" || board.BoardNumber < 1 {
		return ErrVibeBoardInvalid
	}
	if _, err := time.Parse("2006-01-02", board.PublishDate); err != nil {
		return ErrVibeBoardInvalid
	}
	if !validVibeText(board.Prompt, MaxVibePromptRunes) || !validVibeBoardTileCount(len(board.Tiles)) {
		return ErrVibeBoardInvalid
	}
	seenIDs := make(map[string]struct{}, len(board.Tiles))
	seenText := make(map[string]struct{}, len(board.Tiles))
	for _, tile := range board.Tiles {
		if tile.ID == "" || !validVibeText(tile.Text, MaxVibeTileRunes) {
			return ErrVibeBoardInvalid
		}
		folded := strings.ToLower(strings.TrimSpace(tile.Text))
		if _, exists := seenIDs[tile.ID]; exists {
			return fmt.Errorf("%w: duplicate tile id", ErrVibeBoardInvalid)
		}
		if _, exists := seenText[folded]; exists {
			return fmt.Errorf("%w: duplicate tile text", ErrVibeBoardInvalid)
		}
		seenIDs[tile.ID] = struct{}{}
		seenText[folded] = struct{}{}
	}
	return nil
}

func validVibeBoardTileCount(count int) bool {
	return count >= VibeBoardMinTileCount && count <= VibeBoardMaxTileCount && count%VibeBoardColumns == 0
}

// vibeBoardRowsForMembers adds one row for each four-person membership band.
// The product cap of 20 members therefore maps to 3..7 rows. Small crews retain
// enough ambiguity to make multiple cards without turning the palette into a
// cramped eight-fragment answer hunt.
func vibeBoardRowsForMembers(memberCount int) int {
	if memberCount < 1 {
		memberCount = 1
	}
	if memberCount > maxCrewMembers {
		memberCount = maxCrewMembers
	}
	return VibeBoardMinRows + (memberCount-1)/VibeBoardColumns
}

func projectVibeBoard(board VibeBoard, tileCount int) VibeBoard {
	if tileCount < VibeBoardMinTileCount {
		tileCount = VibeBoardMinTileCount
	}
	if tileCount > len(board.Tiles) {
		tileCount = len(board.Tiles)
	}
	tileCount -= tileCount % VibeBoardColumns
	projected := board
	projected.Tiles = append([]Tile(nil), board.Tiles[:tileCount]...)
	return projected
}

func projectVibeBoardForMembers(board VibeBoard, memberCount int) VibeBoard {
	return projectVibeBoard(board, vibeBoardRowsForMembers(memberCount)*VibeBoardColumns)
}

func validVibeText(value string, limit int) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > limit {
		return false
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
