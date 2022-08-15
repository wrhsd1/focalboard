package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/focalboard/server/app"
	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/services/audit"
)

func (a *API) registerCategoriesRoutes(r *mux.Router) {
	// Category APIs
	r.HandleFunc("/teams/{teamID}/categories", a.sessionRequired(a.handleCreateCategory)).Methods(http.MethodPost)
	r.HandleFunc("/teams/{teamID}/categories/{categoryID}", a.sessionRequired(a.handleUpdateCategory)).Methods(http.MethodPut)
	r.HandleFunc("/teams/{teamID}/categories/{categoryID}", a.sessionRequired(a.handleDeleteCategory)).Methods(http.MethodDelete)
	r.HandleFunc("/teams/{teamID}/categories", a.sessionRequired(a.handleGetUserCategoryBoards)).Methods(http.MethodGet)
	r.HandleFunc("/teams/{teamID}/categories/{categoryID}/boards/{boardID}", a.sessionRequired(a.handleUpdateCategoryBoard)).Methods(http.MethodPost)
}

func (a *API) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	// swagger:operation POST /teams/{teamID}/categories createCategory
	//
	// Create a category for boards
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
	// - name: Body
	//   in: body
	//   description: category to create
	//   required: true
	//   schema:
	//     "$ref": "#/definitions/Category"
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//     schema:
	//       "$ref": "#/definitions/Category"
	//   default:
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"

	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	var category model.Category

	err = json.Unmarshal(requestBody, &category)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	auditRec := a.makeAuditRecord(r, "createCategory", audit.Fail)
	defer a.audit.LogRecord(audit.LevelModify, auditRec)

	ctx := r.Context()
	session := ctx.Value(sessionContextKey).(*model.Session)

	// user can only create category for themselves
	if category.UserID != session.UserID {
		a.errorResponse(
			w,
			r.URL.Path,
			http.StatusBadRequest,
			fmt.Sprintf("userID %s and category userID %s mismatch", session.UserID, category.UserID),
			nil,
		)
		return
	}

	vars := mux.Vars(r)
	teamID := vars["teamID"]

	if category.TeamID != teamID {
		a.errorResponse(
			w,
			r.URL.Path,
			http.StatusBadRequest,
			"teamID mismatch",
			nil,
		)
		return
	}

	createdCategory, err := a.app.CreateCategory(&category)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	data, err := json.Marshal(createdCategory)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	jsonBytesResponse(w, http.StatusOK, data)
	auditRec.AddMeta("categoryID", createdCategory.ID)
	auditRec.Success()
}

func (a *API) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	// swagger:operation PUT /teams/{teamID}/categories/{categoryID} updateCategory
	//
	// Create a category for boards
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
	// - name: categoryID
	//   in: path
	//   description: Category ID
	//   required: true
	//   type: string
	// - name: Body
	//   in: body
	//   description: category to update
	//   required: true
	//   schema:
	//     "$ref": "#/definitions/Category"
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//     schema:
	//       "$ref": "#/definitions/Category"
	//   default:
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"

	vars := mux.Vars(r)
	categoryID := vars["categoryID"]

	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	var category model.Category
	err = json.Unmarshal(requestBody, &category)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	auditRec := a.makeAuditRecord(r, "updateCategory", audit.Fail)
	defer a.audit.LogRecord(audit.LevelModify, auditRec)

	if categoryID != category.ID {
		a.errorResponse(w, r.URL.Path, http.StatusBadRequest, "categoryID mismatch in patch and body", nil)
		return
	}

	ctx := r.Context()
	session := ctx.Value(sessionContextKey).(*model.Session)

	// user can only update category for themselves
	if category.UserID != session.UserID {
		a.errorResponse(w, r.URL.Path, http.StatusBadRequest, "user ID mismatch in session and category", nil)
		return
	}

	teamID := vars["teamID"]
	if category.TeamID != teamID {
		a.errorResponse(
			w,
			r.URL.Path,
			http.StatusBadRequest,
			"teamID mismatch",
			nil,
		)
		return
	}

	updatedCategory, err := a.app.UpdateCategory(&category)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrorCategoryDeleted):
			a.errorResponse(w, r.URL.Path, http.StatusNotFound, "", err)
		case errors.Is(err, app.ErrorCategoryPermissionDenied):
			// TODO: The permissions should be handled as much as possible at
			// the API level, this needs to be changed
			a.errorResponse(w, r.URL.Path, http.StatusForbidden, "", err)
		default:
			a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		}
		return
	}

	data, err := json.Marshal(updatedCategory)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	jsonBytesResponse(w, http.StatusOK, data)
	auditRec.Success()
}

func (a *API) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	// swagger:operation DELETE /teams/{teamID}/categories/{categoryID} deleteCategory
	//
	// Delete a category
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
	// - name: categoryID
	//   in: path
	//   description: Category ID
	//   required: true
	//   type: string
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//   default:
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"

	ctx := r.Context()
	session := ctx.Value(sessionContextKey).(*model.Session)
	vars := mux.Vars(r)

	userID := session.UserID
	teamID := vars["teamID"]
	categoryID := vars["categoryID"]

	auditRec := a.makeAuditRecord(r, "deleteCategory", audit.Fail)
	defer a.audit.LogRecord(audit.LevelModify, auditRec)

	deletedCategory, err := a.app.DeleteCategory(categoryID, userID, teamID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrorInvalidCategory):
			a.errorResponse(w, r.URL.Path, http.StatusBadRequest, "", err)
		case errors.Is(err, app.ErrorCategoryPermissionDenied):
			// TODO: The permissions should be handled as much as possible at
			// the API level, this needs to be changed
			a.errorResponse(w, r.URL.Path, http.StatusForbidden, "", err)
		default:
			a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		}
		return
	}

	data, err := json.Marshal(deletedCategory)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	jsonBytesResponse(w, http.StatusOK, data)
	auditRec.Success()
}

func (a *API) handleGetUserCategoryBoards(w http.ResponseWriter, r *http.Request) {
	// swagger:operation GET /teams/{teamID}/categories getUserCategoryBoards
	//
	// Gets the user's board categories
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
	//       items:
	//         "$ref": "#/definitions/CategoryBoards"
	//       type: array
	//   default:
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"

	ctx := r.Context()
	session := ctx.Value(sessionContextKey).(*model.Session)
	userID := session.UserID

	vars := mux.Vars(r)
	teamID := vars["teamID"]

	auditRec := a.makeAuditRecord(r, "getUserCategoryBoards", audit.Fail)
	defer a.audit.LogRecord(audit.LevelModify, auditRec)

	categoryBlocks, err := a.app.GetUserCategoryBoards(userID, teamID)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	data, err := json.Marshal(categoryBlocks)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	jsonBytesResponse(w, http.StatusOK, data)
	auditRec.Success()
}

func (a *API) handleUpdateCategoryBoard(w http.ResponseWriter, r *http.Request) {
	// swagger:operation POST /teams/{teamID}/categories/{categoryID}/boards/{boardID} updateCategoryBoard
	//
	// Set the category of a board
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
	// - name: categoryID
	//   in: path
	//   description: Category ID
	//   required: true
	//   type: string
	// - name: boardID
	//   in: path
	//   description: Board ID
	//   required: true
	//   type: string
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//   default:
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"

	auditRec := a.makeAuditRecord(r, "updateCategoryBoard", audit.Fail)
	defer a.audit.LogRecord(audit.LevelModify, auditRec)

	vars := mux.Vars(r)
	categoryID := vars["categoryID"]
	boardID := vars["boardID"]
	teamID := vars["teamID"]

	ctx := r.Context()
	session := ctx.Value(sessionContextKey).(*model.Session)
	userID := session.UserID

	// TODO: Check the category and the team matches
	err := a.app.AddUpdateUserCategoryBoard(teamID, userID, categoryID, boardID)
	if err != nil {
		a.errorResponse(w, r.URL.Path, http.StatusInternalServerError, "", err)
		return
	}

	jsonBytesResponse(w, http.StatusOK, []byte("ok"))
	auditRec.Success()
}
