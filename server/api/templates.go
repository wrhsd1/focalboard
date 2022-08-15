package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/services/audit"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

func (a *API) registerTemplatesRoutes(r *mux.Router) {
	r.HandleFunc("/teams/{teamID}/templates", a.sessionRequired(a.handleGetTemplates)).Methods("GET")
}

func (a *API) handleGetTemplates(w http.ResponseWriter, r *http.Request) {
	// swagger:operation GET /teams/{teamID}/templates getTemplates
	//
	// Returns team templates
	//
	// ---
	// produces:
	// - application/json
	// parameters:
	// - name: teamID
	//   in: path
	//   description: Team ID
	//   required: true
	//   type: string
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//     schema:
	//       type: array
	//       items:
	//         "$ref": "#/definitions/Board"
	//   default:
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"

	teamID := mux.Vars(r)["teamID"]
	userID := getUserID(r)

	if teamID != model.GlobalTeamID && !a.permissions.HasPermissionToTeam(userID, teamID, model.PermissionViewTeam) {
		a.errorResponse(w, r.URL.Path, http.StatusForbidden, "", PermissionError{"access denied to team"})
		return
	}

	auditRec := a.makeAuditRecord(r, "getTemplates", audit.Fail)
	defer a.audit.LogRecord(audit.LevelRead, auditRec)
	auditRec.AddMeta("teamID", teamID)

	// retrieve boards list
	boards, err := a.app.GetTemplateBoards(teamID, userID)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	results := []*model.Board{}
	for _, board := range boards {
		if board.Type == model.BoardTypeOpen {
			results = append(results, board)
		} else if a.permissions.HasPermissionToBoard(userID, board.ID, model.PermissionViewBoard) {
			results = append(results, board)
		}
	}

	a.logger.Debug("GetTemplates",
		mlog.String("teamID", teamID),
		mlog.Int("boardsCount", len(results)),
	)

	data, err := json.Marshal(results)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	// response
	jsonBytesResponse(w, http.StatusOK, data)

	auditRec.AddMeta("templatesCount", len(results))
	auditRec.Success()
}
