package vibegrid

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/lib/pq"
)

// wrongGuessView is one heatmap row with human-readable tile text.
type wrongGuessView struct {
	Tiles []string `json:"tiles"`
	Count int      `json:"count"`
}

type adminAnalyticsResponse struct {
	Stats        PuzzleStats      `json:"stats"`
	WrongGuesses []wrongGuessView `json:"wrongGuesses"`
}

// handleStats serves public completion stats for any puzzle. Without a database
// it returns empty stats so the UI can simply show nothing.
func (server *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if server.stats == nil {
		writeJSON(w, http.StatusOK, PuzzleStats{})
		return
	}

	stats, err := server.stats.PuzzleStats(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load stats.")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleAdminAnalytics serves the admin wrong-guess heatmap: stats plus the most
// common incorrect groupings, with tile ids resolved to their text.
func (server *Server) handleAdminAnalytics(w http.ResponseWriter, r *http.Request) {
	if server.stats == nil {
		writeError(w, http.StatusServiceUnavailable, "Analytics require a database.")
		return
	}
	puzzleID := r.PathValue("id")

	stats, err := server.stats.PuzzleStats(r.Context(), puzzleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load stats.")
		return
	}

	groupings, err := server.stats.WrongGuessGroupings(r.Context(), puzzleID, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load analytics.")
		return
	}

	tileText := server.tileTextLookup(r.Context(), puzzleID)
	wrongGuesses := make([]wrongGuessView, 0, len(groupings))
	for _, grouping := range groupings {
		tiles := make([]string, 0, len(grouping.TileIDs))
		for _, id := range grouping.TileIDs {
			if text, ok := tileText[id]; ok {
				tiles = append(tiles, text)
			} else {
				tiles = append(tiles, id)
			}
		}
		wrongGuesses = append(wrongGuesses, wrongGuessView{Tiles: tiles, Count: grouping.Count})
	}

	writeJSON(w, http.StatusOK, adminAnalyticsResponse{Stats: stats, WrongGuesses: wrongGuesses})
}

// tileTextLookup builds an id->text map for a puzzle, used to render heatmap rows
// readably. Returns an empty map if the puzzle can't be loaded.
func (server *Server) tileTextLookup(ctx context.Context, puzzleID string) map[string]string {
	lookup := map[string]string{}
	puzzles, err := server.puzzles.Puzzles(ctx)
	if err != nil {
		return lookup
	}
	puzzle, err := FindPuzzleByID(puzzles, puzzleID)
	if err != nil {
		return lookup
	}
	for _, group := range puzzle.Groups {
		for _, tile := range group.Tiles {
			lookup[tile.ID] = tile.Text
		}
	}
	return lookup
}

// PuzzleStats is the public, aggregate view of how a puzzle is going: how many
// people have actually played it and how they're faring. Everything is derived
// from the attempts table, so there is no separate analytics pipeline to keep
// in sync.
type PuzzleStats struct {
	Players            int      `json:"players"`
	SolveRate          float64  `json:"solveRate"` // 0..1, of players who made a guess
	FailRate           float64  `json:"failRate"`
	MedianMistakes     float64  `json:"medianMistakes"`
	MedianSolveSeconds *float64 `json:"medianSolveSeconds,omitempty"`
}

// WrongGuessGrouping is one frequently-submitted incorrect set of four tiles,
// the heart of the admin wrong-guess heatmap.
type WrongGuessGrouping struct {
	TileIDs []string `json:"tileIds"`
	Count   int      `json:"count"`
}

// StatsStore exposes read-only analytics over attempts and guesses. Only the
// Postgres implementation exists; without a database the server reports empty
// stats rather than failing.
type StatsStore interface {
	PuzzleStats(ctx context.Context, puzzleID string) (PuzzleStats, error)
	WrongGuessGroupings(ctx context.Context, puzzleID string, limit int) ([]WrongGuessGrouping, error)
}

type PostgresStatsStore struct {
	db *sql.DB
}

func NewPostgresStatsStore(database *sql.DB) *PostgresStatsStore {
	return &PostgresStatsStore{db: database}
}

func (store *PostgresStatsStore) PuzzleStats(ctx context.Context, puzzleID string) (PuzzleStats, error) {
	var (
		players      int
		completed    int
		failed       int
		medMistakes  sql.NullFloat64
		medSolveSecs sql.NullFloat64
	)
	err := store.db.QueryRowContext(ctx,
		`select
		   count(*) filter (where guess_count > 0)                                as players,
		   count(*) filter (where completed)                                      as completed,
		   count(*) filter (where failed)                                         as failed,
		   percentile_cont(0.5) within group (order by mistakes)
		     filter (where guess_count > 0)                                       as median_mistakes,
		   percentile_cont(0.5) within group (order by extract(epoch from (completed_at - started_at)))
		     filter (where completed and completed_at is not null)                as median_solve_seconds
		 from attempts
		 where puzzle_id = $1`,
		puzzleID,
	).Scan(&players, &completed, &failed, &medMistakes, &medSolveSecs)
	if err != nil {
		return PuzzleStats{}, fmt.Errorf("puzzle stats: %w", err)
	}

	stats := PuzzleStats{Players: players}
	if players > 0 {
		stats.SolveRate = float64(completed) / float64(players)
		stats.FailRate = float64(failed) / float64(players)
	}
	if medMistakes.Valid {
		stats.MedianMistakes = medMistakes.Float64
	}
	if medSolveSecs.Valid {
		seconds := medSolveSecs.Float64
		stats.MedianSolveSeconds = &seconds
	}
	return stats, nil
}

func (store *PostgresStatsStore) WrongGuessGroupings(ctx context.Context, puzzleID string, limit int) ([]WrongGuessGrouping, error) {
	rows, err := store.db.QueryContext(ctx,
		`select g.selected_tile_ids, count(*) as n
		 from attempt_guesses g
		 join attempts a on a.id = g.attempt_id
		 where a.puzzle_id = $1 and g.is_correct = false
		 group by g.selected_tile_ids
		 order by n desc
		 limit $2`,
		puzzleID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("wrong guess groupings: %w", err)
	}
	defer rows.Close()

	groupings := []WrongGuessGrouping{}
	for rows.Next() {
		var grouping WrongGuessGrouping
		if err := rows.Scan(pq.Array(&grouping.TileIDs), &grouping.Count); err != nil {
			return nil, fmt.Errorf("scan grouping: %w", err)
		}
		groupings = append(groupings, grouping)
	}
	return groupings, rows.Err()
}
