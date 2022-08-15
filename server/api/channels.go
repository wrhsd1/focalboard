package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/services/audit"

	mm_model "github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

func (a *API) registerChannelsRoutes(r *mux.Router) {
	r.HandleFunc("/teams/{teamID}/channels/{channelID}", a.sessionRequired(a.handleGetChannel)).Methods("GET")
}

func (a *API) handleGetChannel(w http.ResponseWriter, r *http.Request) {
	// swagger:operation GET /teams/{teamID}/channels/{channelID} getChannel
	//
	// Returns the requested channel
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
	// - name: channelID
	//   in: path
	//   description: Channel ID
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
	//         "$ref": "#/definitions/Channel"
	//   default:
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"

	if !a.MattermostAuth {
		a.errorResponse(w, r.URL.Path, http.StatusNotImplemented, "not permitted in standalone mode", nil)
		return
	}

	teamID := mux.Vars(r)["teamID"]
	channelID := mux.Vars(r)["channelID"]
	userID := getUserID(r)

	if !a.permissions.HasPermissionToTeam(userID, teamID, model.PermissionViewTeam) {
		a.errorResponse(w, r.URL.Path, http.StatusForbidden, "", PermissionError{"access denied to team"})
		return
	}

	if !a.permissions.HasPermissionToChannel(userID, channelID, model.PermissionReadChannel) {
		a.errorResponse(w, r.URL.Path, http.StatusForbidden, "", PermissionError{"access denied to channel"})
		return
	}

	auditRec := a.makeAuditRecord(r, "getChannel", audit.Fail)
	defer a.audit.LogRecord(audit.LevelRead, auditRec)
	auditRec.AddMeta("teamID", teamID)
	auditRec.AddMeta("channelID", teamID)

	channel, err := a.app.GetChannel(teamID, channelID)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	a.logger.Debug("GetChannel",
		mlog.String("teamID", teamID),
		mlog.String("channelID", channelID),
	)

	if channel.TeamId != teamID {
		if channel.Type != mm_model.ChannelTypeDirect && channel.Type != mm_model.ChannelTypeGroup {
			a.errorResponse(w, r.URL.Path, http.StatusNotFound, "", nil)
			return
		}
	}

	data, err := json.Marshal(channel)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	// response
	jsonBytesResponse(w, http.StatusOK, data)

	auditRec.Success()
}
