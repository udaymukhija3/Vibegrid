package vibegrid

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	maxReportDetailsLength = 1000
	maxAppealMessageLength = 1000
	maxContactLength       = 200
)

var validReportReasons = map[string]bool{
	"OFFENSIVE":     true,
	"PERSONAL_INFO": true,
	"SPAM":          true,
	"UNFAIR":        true,
	"COPYRIGHT":     true,
	"OTHER":         true,
}

type createdModerationResponse struct {
	OK bool   `json:"ok"`
	ID string `json:"id"`
}

type resolveReportRequest struct {
	Action string `json:"action"`
	Note   string `json:"note"`
}

type resolveAppealRequest struct {
	Action string `json:"action"`
	Note   string `json:"note"`
}

type moderationQueueResponse struct {
	Reports []ModerationReport `json:"reports"`
}

type appealQueueResponse struct {
	Appeals []ModerationAppeal `json:"appeals"`
}

type auditLogResponse struct {
	Actions []ModerationAction `json:"actions"`
}

func (server *Server) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	if server.moderation == nil {
		writeError(w, http.StatusServiceUnavailable, "Reports require a database.")
		return
	}

	var input ReportInput
	if !decodeJSONBody(w, r, maxAdminBodyBytes, &input, "That report payload is not valid JSON.") {
		return
	}
	input.PuzzleID = strings.TrimSpace(input.PuzzleID)
	input.Reason = strings.ToUpper(strings.TrimSpace(input.Reason))
	input.Details = strings.TrimSpace(input.Details)
	input.Contact = strings.TrimSpace(input.Contact)
	if err := validateReport(input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if _, err := server.publicPuzzleByID(r.Context(), input.PuzzleID); err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	sessionID := EnsureSessionID(w, r, server.secureCookies)
	report, err := server.moderation.CreateReport(r.Context(), input, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save that report.")
		return
	}
	writeJSON(w, http.StatusCreated, createdModerationResponse{OK: true, ID: report.ID})
}

func (server *Server) handleCreateAppeal(w http.ResponseWriter, r *http.Request) {
	if server.moderation == nil {
		writeError(w, http.StatusServiceUnavailable, "Appeals require a database.")
		return
	}

	var input AppealInput
	if !decodeJSONBody(w, r, maxAdminBodyBytes, &input, "That appeal payload is not valid JSON.") {
		return
	}
	input.PuzzleID = strings.TrimSpace(input.PuzzleID)
	input.Contact = strings.TrimSpace(input.Contact)
	input.Message = strings.TrimSpace(input.Message)
	if err := validateAppeal(input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if _, err := server.puzzles.PuzzleByID(r.Context(), input.PuzzleID); err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	appeal, err := server.moderation.CreateAppeal(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save that appeal.")
		return
	}
	writeJSON(w, http.StatusCreated, createdModerationResponse{OK: true, ID: appeal.ID})
}

func (server *Server) handleAdminReports(w http.ResponseWriter, r *http.Request) {
	if server.moderation == nil {
		writeError(w, http.StatusServiceUnavailable, "Moderation requires a database.")
		return
	}
	reports, err := server.moderation.ListReports(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load reports.")
		return
	}
	writeJSON(w, http.StatusOK, moderationQueueResponse{Reports: reports})
}

func (server *Server) handleAdminResolveReport(w http.ResponseWriter, r *http.Request) {
	if server.moderation == nil {
		writeError(w, http.StatusServiceUnavailable, "Moderation requires a database.")
		return
	}

	reportID := r.PathValue("id")
	var request resolveReportRequest
	if !decodeJSONBody(w, r, maxAdminBodyBytes, &request, "That moderation payload is not valid JSON.") {
		return
	}
	action := strings.ToUpper(strings.TrimSpace(request.Action))
	note := strings.TrimSpace(request.Note)

	status := ReportStatusDismissed
	if action == "ARCHIVE" {
		status = ReportStatusActioned
	} else if action != "DISMISS" {
		writeError(w, http.StatusUnprocessableEntity, "Action must be ARCHIVE or DISMISS.")
		return
	}

	report, err := server.moderation.ResolveReport(r.Context(), reportID, status, note, adminActor(r))
	if err != nil {
		if errors.Is(err, ErrReportNotFound) {
			writeError(w, http.StatusNotFound, "Report not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not resolve that report.")
		return
	}

	if action == "ARCHIVE" {
		if err := server.adminPuzzles.Archive(r.Context(), report.PuzzleID); err != nil {
			writeError(w, http.StatusInternalServerError, "Report saved, but could not archive that puzzle.")
			return
		}
		_ = server.moderation.AddAction(r.Context(), ModerationActionInput{
			ReportID: report.ID,
			PuzzleID: report.PuzzleID,
			Actor:    adminActor(r),
			Action:   "PUZZLE_ARCHIVED_FROM_REPORT",
			Reason:   report.Reason,
			Note:     note,
		})
	}
	writeJSON(w, http.StatusOK, report)
}

func (server *Server) handleAdminAppeals(w http.ResponseWriter, r *http.Request) {
	if server.moderation == nil {
		writeError(w, http.StatusServiceUnavailable, "Moderation requires a database.")
		return
	}
	appeals, err := server.moderation.ListAppeals(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load appeals.")
		return
	}
	writeJSON(w, http.StatusOK, appealQueueResponse{Appeals: appeals})
}

func (server *Server) handleAdminResolveAppeal(w http.ResponseWriter, r *http.Request) {
	if server.moderation == nil {
		writeError(w, http.StatusServiceUnavailable, "Moderation requires a database.")
		return
	}

	appealID := r.PathValue("id")
	var request resolveAppealRequest
	if !decodeJSONBody(w, r, maxAdminBodyBytes, &request, "That appeal payload is not valid JSON.") {
		return
	}
	action := strings.ToUpper(strings.TrimSpace(request.Action))
	note := strings.TrimSpace(request.Note)
	if action != "REINSTATE" && action != "CLOSE" {
		writeError(w, http.StatusUnprocessableEntity, "Action must be REINSTATE or CLOSE.")
		return
	}

	appeal, err := server.moderation.ResolveAppeal(r.Context(), appealID, note, adminActor(r))
	if err != nil {
		if errors.Is(err, ErrAppealNotFound) {
			writeError(w, http.StatusNotFound, "Appeal not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not resolve that appeal.")
		return
	}

	if action == "REINSTATE" {
		if err := server.adminPuzzles.Reinstate(r.Context(), appeal.PuzzleID); err != nil {
			writeError(w, http.StatusInternalServerError, "Appeal saved, but could not reinstate that puzzle.")
			return
		}
		_ = server.moderation.AddAction(r.Context(), ModerationActionInput{
			AppealID: appeal.ID,
			PuzzleID: appeal.PuzzleID,
			Actor:    adminActor(r),
			Action:   "PUZZLE_REINSTATED_FROM_APPEAL",
			Note:     note,
		})
	}
	writeJSON(w, http.StatusOK, appeal)
}

func (server *Server) handleAdminAuditLog(w http.ResponseWriter, r *http.Request) {
	if server.moderation == nil {
		writeError(w, http.StatusServiceUnavailable, "Moderation requires a database.")
		return
	}
	actions, err := server.moderation.AuditLog(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load audit log.")
		return
	}
	writeJSON(w, http.StatusOK, auditLogResponse{Actions: actions})
}

func validateReport(input ReportInput) error {
	if input.PuzzleID == "" {
		return errors.New("Puzzle id is required.")
	}
	if !validReportReasons[input.Reason] {
		return errors.New("Pick a valid report reason.")
	}
	if overRuneLimit(input.Details, maxReportDetailsLength) {
		return errors.New("Report details are too long.")
	}
	if overRuneLimit(input.Contact, maxContactLength) {
		return errors.New("Contact is too long.")
	}
	return nil
}

func validateAppeal(input AppealInput) error {
	if input.PuzzleID == "" {
		return errors.New("Puzzle id is required.")
	}
	if input.Message == "" {
		return errors.New("Tell us why this grid should be reviewed.")
	}
	if !utf8.ValidString(input.Message) || overRuneLimit(input.Message, maxAppealMessageLength) {
		return errors.New("Appeal message is too long.")
	}
	if overRuneLimit(input.Contact, maxContactLength) {
		return errors.New("Contact is too long.")
	}
	return nil
}
