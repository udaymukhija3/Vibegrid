package vibegrid

import (
	"regexp"
	"strings"
	"testing"
)

// The public surfaces must never reveal which tiles share a group. These tests
// assert that invariant directly, from the outside, rather than asserting a
// particular id format — so they keep holding if the derivation changes again.

// TestPublicTileIDsDoNotEncodeGrouping is the regression test for the daily
// answer leak: bank tile ids were "<groupID>-t<index>", so trimming the numeric
// suffix partitioned the board into the four correct groups.
func TestPublicTileIDsDoNotEncodeGrouping(t *testing.T) {
	suffix := regexp.MustCompile(`[-_]?\d+$`)

	for _, puzzle := range PuzzleBank() {
		public := ToPublicPuzzle(puzzle)

		// Which group each public tile id really belongs to.
		groupOfTile := map[string]string{}
		for _, group := range puzzle.Groups {
			for _, tile := range group.Tiles {
				groupOfTile[tile.ID] = group.ID
			}
		}

		// An attacker clusters the public ids by any shared prefix. No cluster may
		// line up with a real group.
		for _, cut := range []func(string) string{
			func(id string) string { return suffix.ReplaceAllString(id, "") },
			func(id string) string { return id[:max(len(id)-1, 1)] },
			func(id string) string { return id[:max(len(id)-2, 1)] },
		} {
			clusters := map[string][]string{}
			for _, tile := range public.Tiles {
				key := cut(tile.ID)
				clusters[key] = append(clusters[key], groupOfTile[tile.ID])
			}
			// A cluster of one tile reveals nothing. Two or more tiles sharing a
			// prefix *and* a group is the leak: that prefix is the answer.
			for key, groups := range clusters {
				if len(groups) < 2 {
					continue
				}
				distinct := map[string]bool{}
				for _, g := range groups {
					distinct[g] = true
				}
				if len(distinct) == 1 {
					t.Errorf("puzzle %s: %d public tile ids share prefix %q and all belong to group %v — the id encodes group membership",
						puzzle.ID, len(groups), key, groups[0])
				}
			}
		}

		// Belt and braces: no public tile id may contain its own group id.
		for _, tile := range public.Tiles {
			for _, group := range puzzle.Groups {
				if strings.Contains(tile.ID, group.ID) {
					t.Errorf("puzzle %s: tile id %q contains group id %q", puzzle.ID, tile.ID, group.ID)
				}
			}
		}
	}
}

// TestBankTileIDsAreStableAndUnique guards the two properties the store relies
// on: ids are unique inside a puzzle, and stable across process restarts (an
// in-flight attempt stores tile ids, so a reshuffle would orphan it).
func TestBankTileIDsAreStableAndUnique(t *testing.T) {
	first, second := PuzzleBank(), PuzzleBank()
	if len(first) != len(second) {
		t.Fatalf("bank size changed between calls: %d vs %d", len(first), len(second))
	}

	for index, puzzle := range first {
		seen := map[string]bool{}
		for _, group := range puzzle.Groups {
			for _, tile := range group.Tiles {
				if tile.ID == "" {
					t.Fatalf("puzzle %s: empty tile id", puzzle.ID)
				}
				if seen[tile.ID] {
					t.Errorf("puzzle %s: duplicate tile id %q", puzzle.ID, tile.ID)
				}
				seen[tile.ID] = true

				if !validPublicIdentifier(tile.ID, maxTileIDLength) {
					t.Errorf("puzzle %s: tile id %q is not a valid public identifier (guesses would be rejected)", puzzle.ID, tile.ID)
				}
			}
		}

		for groupIndex, group := range puzzle.Groups {
			for tileIndex, tile := range group.Tiles {
				if other := second[index].Groups[groupIndex].Tiles[tileIndex].ID; other != tile.ID {
					t.Errorf("puzzle %s: tile id is not stable across calls: %q vs %q", puzzle.ID, tile.ID, other)
				}
			}
		}
	}
}

// TestOGImageDoesNotRevealGrouping is the regression test for the share card:
// it iterated puzzle.Groups, so it drew one group per row, each row filled with
// that group's colour — the solved grid, as the og:image of every share link.
func TestOGImageDoesNotRevealGrouping(t *testing.T) {
	puzzle := PuzzleBank()[0]
	puzzle.PuzzleNumber = 42
	puzzle.PublishDate = "2026-08-16"

	svg := renderPuzzleOGImage(puzzle)

	tile := regexp.MustCompile(`<rect x="\d+" y="\d+" width="\d+" height="\d+" rx="14" fill="(#[0-9a-fA-F]{6})"[^>]*/><text[^>]*>([^<]*)</text>`)
	matches := tile.FindAllStringSubmatch(svg, -1)
	if len(matches) != 16 {
		t.Fatalf("expected 16 tiles in the OG image, got %d", len(matches))
	}

	// A single fill for every tile: colour cannot carry group identity.
	fills := map[string]bool{}
	for _, m := range matches {
		fills[m[1]] = true
	}
	if len(fills) != 1 {
		t.Errorf("OG image uses %d distinct tile fills %v — colour encodes group membership", len(fills), fills)
	}

	// The tiles must appear in the public display order, not group order.
	want := ToPublicPuzzle(puzzle).Tiles
	for index, m := range matches {
		if got, expected := m[2], truncateOGText(want[index].Text, 11); got != expected {
			t.Errorf("OG tile %d = %q, want %q (image is not in public display order)", index, got, expected)
		}
	}

	// Each row of four must span more than one group.
	groupOfText := map[string]string{}
	for _, group := range puzzle.Groups {
		for _, t := range group.Tiles {
			groupOfText[truncateOGText(t.Text, 11)] = group.ID
		}
	}
	for row := 0; row < 4; row++ {
		groups := map[string]bool{}
		for col := 0; col < 4; col++ {
			groups[groupOfText[matches[row*4+col][2]]] = true
		}
		if len(groups) == 1 {
			t.Errorf("OG image row %d contains exactly one group %v — the row is an answer", row, groups)
		}
	}
}
