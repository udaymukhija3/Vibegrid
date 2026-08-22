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
	VibeBoardTileCount = 12
	VibeCardTileCount  = 4
	MaxVibePromptRunes = 140
	MaxVibeTileRunes   = 28
)

var ErrVibeBoardInvalid = errors.New("vibe board is invalid")

// VibeBoard has no hidden partition or answer key. Its twelve fragments are a
// constrained creative palette shared by every crew on one date.
type VibeBoard struct {
	ID          string `json:"id"`
	BoardNumber int    `json:"boardNumber"`
	PublishDate string `json:"publishDate"`
	Prompt      string `json:"prompt"`
	Tiles       []Tile `json:"tiles"`
}

type vibeBoardTemplate struct {
	prompt string
	tiles  [VibeBoardTileCount]string
}

var vibeBoardTemplates = []vibeBoardTemplate{
	{
		prompt: "Build a week that is technically under control.",
		tiles: [VibeBoardTileCount]string{
			"meal prep", "unread emails", "3pm nap", "open laptop",
			"clean sheets", "monday dread", "leftover pizza", "five tabs",
			"face mask", "11pm panic", "same hoodie", "new notebook",
		},
	},
	{
		prompt: "Build a first impression that will not survive the evening.",
		tiles: [VibeBoardTileCount]string{
			"noise-cancelling", "nervous laugh", "you're muted", "two-hour latte",
			"third refill", "shared scone", "circle back", "no you go",
			"laptop fortress", "phone away", "share the link", "wait what",
		},
	},
	{
		prompt: "Build the person every group chat eventually creates.",
		tiles: [VibeBoardTileCount]string{
			"who's in", "thumbs up", "long voice note", "random meme",
			"sent a poll", "seen 2pm", "three paragraphs", "wrong chat",
			"calendar invite", "starts typing", "you guys", "47 messages",
		},
	},
	{
		prompt: "Build an airport personality you would deny having.",
		tiles: [VibeBoardTileCount]string{
			"hovers early", "testing perfume", "gate change", "one carry-on",
			"group nine", "giant toblerone", "floor outlet", "priority lane",
			"blocks the lane", "why is this here", "$9 water", "slip-on shoes",
		},
	},
	{
		prompt: "Build the 2am version of a reasonable person.",
		tiles: [VibeBoardTileCount]string{
			"that text", "learn french", "heat death", "is there cheese",
			"said what", "5am club", "are we real", "quiet fridge",
			"ten years ago", "new startup", "tiny planet", "one chip",
		},
	},
	{
		prompt: "Build a workday with no measurable output.",
		tiles: [VibeBoardTileCount]string{
			"alt-tab", "synergy", "whose milk", "camera off",
			"walk with mug", "circle back", "passive note", "slow replies",
			"furrowed brow", "low-hanging", "last coffee", "soft logout",
		},
	},
	{
		prompt: "Build a text conversation that is definitely fine.",
		tiles: [VibeBoardTileCount]string{
			"lowercase hey", "period.", "typing dots", "nine texts",
			"haha", "k.", "drafted twice", "all caps",
			"sure!", "fine.", "deleted that", "voice memo",
		},
	},
	{
		prompt: "Build a wellness era with a short expiry date.",
		tiles: [VibeBoardTileCount]string{
			"bought shoes", "own towel", "flex check", "podcast on",
			"day three", "nods only", "tank top", "incline walk",
			"sore everywhere", "claimed rack", "phone propped", "just sweating",
		},
	},
	{
		prompt: "Build the person quietly controlling the party.",
		tiles: [VibeBoardTileCount]string{
			"near the dip", "have you met", "irish exit", "helps clean",
			"deep talk", "pulls you in", "suddenly gone", "one more song",
			"holds the bowl", "knows everyone", "no goodbye", "lingering chat",
		},
	},
	{
		prompt: "Build an online purchase you will defend in public.",
		tiles: [VibeBoardTileCount]string{
			"added twelve", "70% off", "out for delivery", "didn't fit",
			"removed eleven", "basket full", "where's the van", "repack it",
			"do i need", "free shipping", "left at door", "lost receipt",
		},
	},
	{
		prompt: "Build the full emotional arc of a wedding guest.",
		tiles: [VibeBoardTileCount]string{
			"pacing myself", "cha cha slide", "how do you know", "seating chart",
			"third drink", "heels off", "lovely venue", "shuttle time",
			"found the tray", "conga line", "the couple", "plus one",
		},
	},
	{
		prompt: "Build a home that has given up on categories.",
		tiles: [VibeBoardTileCount]string{
			"deep clean", "worn once", "dead batteries", "paid the bill",
			"reorganize shelf", "not dirty", "mystery key", "watered plant",
			"anything but", "the pile", "old cables", "made the bed",
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
	tiles := make([]Tile, VibeBoardTileCount)
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

func validateVibeBoard(board VibeBoard) error {
	if board.ID == "" || board.BoardNumber < 1 {
		return ErrVibeBoardInvalid
	}
	if _, err := time.Parse("2006-01-02", board.PublishDate); err != nil {
		return ErrVibeBoardInvalid
	}
	if !validVibeText(board.Prompt, MaxVibePromptRunes) || len(board.Tiles) != VibeBoardTileCount {
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
