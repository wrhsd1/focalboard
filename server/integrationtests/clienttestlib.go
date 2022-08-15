package integrationtests

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/mattermost/focalboard/server/client"
	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/server"
	"github.com/mattermost/focalboard/server/services/auth"
	"github.com/mattermost/focalboard/server/services/config"
	"github.com/mattermost/focalboard/server/services/permissions/localpermissions"
	"github.com/mattermost/focalboard/server/services/permissions/mmpermissions"
	"github.com/mattermost/focalboard/server/services/store"
	"github.com/mattermost/focalboard/server/services/store/sqlstore"

	mmModel "github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"

	"github.com/stretchr/testify/require"
)

const (
	user1Username = "user1"
	user2Username = "user2"
	password      = "Pa$$word"
)

const (
	userAnon         string = "anon"
	userNoTeamMember string = "no-team-member"
	userTeamMember   string = "team-member"
	userViewer       string = "viewer"
	userCommenter    string = "commenter"
	userEditor       string = "editor"
	userAdmin        string = "admin"
)

var (
	userAnonID         = userAnon
	userNoTeamMemberID = userNoTeamMember
	userTeamMemberID   = userTeamMember
	userViewerID       = userViewer
	userCommenterID    = userCommenter
	userEditorID       = userEditor
	userAdminID        = userAdmin
)

type LicenseType int

const (
	LicenseNone         LicenseType = iota // 0
	LicenseProfessional                    // 1
	LicenseEnterprise                      // 2
)

type TestHelper struct {
	T       *testing.T
	Server  *server.Server
	Client  *client.Client
	Client2 *client.Client
}

type FakePermissionPluginAPI struct{}

func (*FakePermissionPluginAPI) HasPermissionToTeam(userID string, teamID string, permission *mmModel.Permission) bool {
	if userID == userNoTeamMember {
		return false
	}
	if teamID == "empty-team" {
		return false
	}
	return true
}

func (*FakePermissionPluginAPI) HasPermissionToChannel(userID string, channelID string, permission *mmModel.Permission) bool {
	return channelID == "valid-channel-id"
}

func getTestConfig() (*config.Configuration, error) {
	dbType, connectionString, err := sqlstore.PrepareNewTestDatabase()
	if err != nil {
		return nil, err
	}

	logging := `
	{
		"testing": {
			"type": "console",
			"options": {
				"out": "stdout"
			},
			"format": "plain",
			"format_options": {
				"delim": "  "
			},
			"levels": [
				{"id": 5, "name": "debug"},
				{"id": 4, "name": "info"},
				{"id": 3, "name": "warn"},
				{"id": 2, "name": "error", "stacktrace": true},
				{"id": 1, "name": "fatal", "stacktrace": true},
				{"id": 0, "name": "panic", "stacktrace": true}
			]
		}
	}`

	return &config.Configuration{
		ServerRoot:        "http://localhost:8888",
		Port:              8888,
		DBType:            dbType,
		DBConfigString:    connectionString,
		DBTablePrefix:     "test_",
		WebPath:           "./pack",
		FilesDriver:       "local",
		FilesPath:         "./files",
		LoggingCfgJSON:    logging,
		SessionExpireTime: int64(30 * time.Second),
		AuthMode:          "native",
	}, nil
}

func newTestServer(singleUserToken string) *server.Server {
	return newTestServerWithLicense(singleUserToken, LicenseNone)
}

func newTestServerWithLicense(singleUserToken string, licenseType LicenseType) *server.Server {
	cfg, err := getTestConfig()
	if err != nil {
		panic(err)
	}

	logger, _ := mlog.NewLogger()
	if err = logger.Configure("", cfg.LoggingCfgJSON, nil); err != nil {
		panic(err)
	}
	singleUser := len(singleUserToken) > 0
	innerStore, err := server.NewStore(cfg, singleUser, logger)
	if err != nil {
		panic(err)
	}

	var db store.Store

	switch licenseType {
	case LicenseProfessional:
		db = NewTestProfessionalStore(innerStore)
	case LicenseEnterprise:
		db = NewTestEnterpriseStore(innerStore)
	case LicenseNone:
		fallthrough
	default:
		db = innerStore
	}

	permissionsService := localpermissions.New(db, logger)

	params := server.Params{
		Cfg:                cfg,
		SingleUserToken:    singleUserToken,
		DBStore:            db,
		Logger:             logger,
		PermissionsService: permissionsService,
	}

	srv, err := server.New(params)
	if err != nil {
		panic(err)
	}

	return srv
}

func NewTestServerPluginMode() *server.Server {
	cfg, err := getTestConfig()
	if err != nil {
		panic(err)
	}
	cfg.AuthMode = "mattermost"
	cfg.EnablePublicSharedBoards = true

	logger, _ := mlog.NewLogger()
	if err = logger.Configure("", cfg.LoggingCfgJSON, nil); err != nil {
		panic(err)
	}
	innerStore, err := server.NewStore(cfg, false, logger)
	if err != nil {
		panic(err)
	}

	db := NewPluginTestStore(innerStore)

	permissionsService := mmpermissions.New(db, &FakePermissionPluginAPI{}, logger)

	params := server.Params{
		Cfg:                cfg,
		DBStore:            db,
		Logger:             logger,
		PermissionsService: permissionsService,
	}

	srv, err := server.New(params)
	if err != nil {
		panic(err)
	}

	return srv
}

func newTestServerLocalMode() *server.Server {
	cfg, err := getTestConfig()
	if err != nil {
		panic(err)
	}
	cfg.EnablePublicSharedBoards = true

	logger, _ := mlog.NewLogger()
	if err = logger.Configure("", cfg.LoggingCfgJSON, nil); err != nil {
		panic(err)
	}

	db, err := server.NewStore(cfg, false, logger)
	if err != nil {
		panic(err)
	}

	permissionsService := localpermissions.New(db, logger)

	params := server.Params{
		Cfg:                cfg,
		DBStore:            db,
		Logger:             logger,
		PermissionsService: permissionsService,
	}

	srv, err := server.New(params)
	if err != nil {
		panic(err)
	}

	// Reduce password has strength for unit tests to dramatically speed up account creation and login
	auth.PasswordHashStrength = 4

	return srv
}

func SetupTestHelperWithToken(t *testing.T) *TestHelper {
	sessionToken := "TESTTOKEN"
	th := &TestHelper{T: t}
	th.Server = newTestServer(sessionToken)
	th.Client = client.NewClient(th.Server.Config().ServerRoot, sessionToken)
	th.Client2 = client.NewClient(th.Server.Config().ServerRoot, sessionToken)
	return th
}

func SetupTestHelper(t *testing.T) *TestHelper {
	return SetupTestHelperWithLicense(t, LicenseNone)
}

func SetupTestHelperPluginMode(t *testing.T) *TestHelper {
	th := &TestHelper{T: t}
	th.Server = NewTestServerPluginMode()
	th.Start()
	return th
}

func SetupTestHelperLocalMode(t *testing.T) *TestHelper {
	th := &TestHelper{T: t}
	th.Server = newTestServerLocalMode()
	th.Start()
	return th
}

func SetupTestHelperWithLicense(t *testing.T, licenseType LicenseType) *TestHelper {
	th := &TestHelper{T: t}
	th.Server = newTestServerWithLicense("", licenseType)
	th.Client = client.NewClient(th.Server.Config().ServerRoot, "")
	th.Client2 = client.NewClient(th.Server.Config().ServerRoot, "")
	return th
}

// Start starts the test server and ensures that it's correctly
// responding to requests before returning.
func (th *TestHelper) Start() *TestHelper {
	go func() {
		if err := th.Server.Start(); err != nil {
			panic(err)
		}
	}()

	for {
		URL := th.Server.Config().ServerRoot
		th.Server.Logger().Info("Polling server", mlog.String("url", URL))
		resp, err := http.Get(URL) //nolint:gosec
		if err != nil {
			th.Server.Logger().Error("Polling failed", mlog.Err(err))
			time.Sleep(100 * time.Millisecond)
			continue
		}
		resp.Body.Close()

		// Currently returns 404
		// if resp.StatusCode != http.StatusOK {
		// 	th.Server.Logger().Error("Not OK", mlog.Int("statusCode", resp.StatusCode))
		// 	continue
		// }

		// Reached this point: server is up and running!
		th.Server.Logger().Info("Server ping OK", mlog.Int("statusCode", resp.StatusCode))

		break
	}

	return th
}

// InitBasic starts the test server and initializes the clients of the
// helper, registering them and logging them into the system.
func (th *TestHelper) InitBasic() *TestHelper {
	// Reduce password has strength for unit tests to dramatically speed up account creation and login
	auth.PasswordHashStrength = 4

	th.Start()

	// user1
	th.RegisterAndLogin(th.Client, user1Username, "user1@sample.com", password, "")

	// get token
	team, resp := th.Client.GetTeam(model.GlobalTeamID)
	th.CheckOK(resp)
	require.NotNil(th.T, team)
	require.NotNil(th.T, team.SignupToken)

	// user2
	th.RegisterAndLogin(th.Client2, user2Username, "user2@sample.com", password, team.SignupToken)

	return th
}

var ErrRegisterFail = errors.New("register failed")

func (th *TestHelper) TearDown() {
	logger := th.Server.Logger()

	if l, ok := logger.(*mlog.Logger); ok {
		defer func() { _ = l.Shutdown() }()
	}

	err := th.Server.Shutdown()
	if err != nil {
		panic(err)
	}

	os.RemoveAll(th.Server.Config().FilesPath)

	if err := os.Remove(th.Server.Config().DBConfigString); err == nil {
		logger.Debug("Removed test database", mlog.String("file", th.Server.Config().DBConfigString))
	}
}

func (th *TestHelper) RegisterAndLogin(client *client.Client, username, email, password, token string) {
	req := &model.RegisterRequest{
		Username: username,
		Email:    email,
		Password: password,
		Token:    token,
	}

	success, resp := th.Client.Register(req)
	th.CheckOK(resp)
	require.True(th.T, success)

	th.Login(client, username, password)
}

func (th *TestHelper) Login(client *client.Client, username, password string) {
	req := &model.LoginRequest{
		Type:     "normal",
		Username: username,
		Password: password,
	}
	data, resp := client.Login(req)
	th.CheckOK(resp)
	require.NotNil(th.T, data)
}

func (th *TestHelper) Login1() {
	th.Login(th.Client, user1Username, password)
}

func (th *TestHelper) Login2() {
	th.Login(th.Client2, user2Username, password)
}

func (th *TestHelper) Logout(client *client.Client) {
	client.Token = ""
}

func (th *TestHelper) Me(client *client.Client) *model.User {
	user, resp := client.GetMe()
	th.CheckOK(resp)
	require.NotNil(th.T, user)
	return user
}

func (th *TestHelper) CreateBoard(teamID string, boardType model.BoardType) *model.Board {
	newBoard := &model.Board{
		TeamID: teamID,
		Type:   boardType,
	}
	board, resp := th.Client.CreateBoard(newBoard)
	th.CheckOK(resp)
	return board
}

func (th *TestHelper) GetUser1() *model.User {
	return th.Me(th.Client)
}

func (th *TestHelper) GetUser2() *model.User {
	return th.Me(th.Client2)
}

func (th *TestHelper) CheckOK(r *client.Response) {
	require.Equal(th.T, http.StatusOK, r.StatusCode)
	require.NoError(th.T, r.Error)
}

func (th *TestHelper) CheckBadRequest(r *client.Response) {
	require.Equal(th.T, http.StatusBadRequest, r.StatusCode)
	require.Error(th.T, r.Error)
}

func (th *TestHelper) CheckNotFound(r *client.Response) {
	require.Equal(th.T, http.StatusNotFound, r.StatusCode)
	require.Error(th.T, r.Error)
}

func (th *TestHelper) CheckUnauthorized(r *client.Response) {
	require.Equal(th.T, http.StatusUnauthorized, r.StatusCode)
	require.Error(th.T, r.Error)
}

func (th *TestHelper) CheckForbidden(r *client.Response) {
	require.Equal(th.T, http.StatusForbidden, r.StatusCode)
	require.Error(th.T, r.Error)
}

func (th *TestHelper) CheckRequestEntityTooLarge(r *client.Response) {
	require.Equal(th.T, http.StatusRequestEntityTooLarge, r.StatusCode)
	require.Error(th.T, r.Error)
}

func (th *TestHelper) CheckNotImplemented(r *client.Response) {
	require.Equal(th.T, http.StatusNotImplemented, r.StatusCode)
	require.Error(th.T, r.Error)
}
