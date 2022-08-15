package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

func (a *API) registerLimitsRoutes(r *mux.Router) {
	// limits
	r.HandleFunc("/limits", a.sessionRequired(a.handleCloudLimits)).Methods("GET")
	r.HandleFunc("/teams/{teamID}/notifyadminupgrade", a.sessionRequired(a.handleNotifyAdminUpgrade)).Methods(http.MethodPost)
}

func (a *API) handleCloudLimits(w http.ResponseWriter, r *http.Request) {
	// swagger:operation GET /limits cloudLimits
	//
	// Fetches the cloud limits of the server.
	//
	// ---
	// produces:
	// - application/json
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//     schema:
	//         "$ref": "#/definitions/BoardsCloudLimits"
	//   default:
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"

	boardsCloudLimits, err := a.app.GetBoardsCloudLimits()
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	data, err := json.Marshal(boardsCloudLimits)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	jsonBytesResponse(w, http.StatusOK, data)
}

func (a *API) handleNotifyAdminUpgrade(w http.ResponseWriter, r *http.Request) {
	// swagger:operation GET /api/v2/teams/{teamID}/notifyadminupgrade handleNotifyAdminUpgrade
	//
	// Notifies admins for upgrade request.
	//
	// ---
	// produces:
	// - application/json
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//   default:
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"

	if !a.MattermostAuth {
		a.errorResponse(w, r.URL.Path, http.StatusNotFound, "", errAPINotSupportedInStandaloneMode)
		return
	}

	vars := mux.Vars(r)
	teamID := vars["teamID"]

	if err := a.app.NotifyPortalAdminsUpgradeRequest(teamID); err != nil {
		jsonStringResponse(w, http.StatusOK, "{}")
	}
}
