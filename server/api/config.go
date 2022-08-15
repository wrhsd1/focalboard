package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

func (a *API) registerConfigRoutes(r *mux.Router) {
	// Config APIs
	r.HandleFunc("/clientConfig", a.getClientConfig).Methods("GET")
}

func (a *API) getClientConfig(w http.ResponseWriter, r *http.Request) {
	// swagger:operation GET /clientConfig getClientConfig
	//
	// Returns the client configuration
	//
	// ---
	// produces:
	// - application/json
	// responses:
	//   '200':
	//     description: success
	//     schema:
	//       "$ref": "#/definitions/ClientConfig"
	//   default:
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"

	clientConfig := a.app.GetClientConfig()

	configData, err := json.Marshal(clientConfig)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}
	jsonBytesResponse(w, http.StatusOK, configData)
}
