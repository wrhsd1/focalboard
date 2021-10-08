package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/mattermost/focalboard/server/auth"
	"github.com/mattermost/focalboard/server/server"
	"github.com/mattermost/focalboard/server/services/config"
	"github.com/mattermost/focalboard/server/services/notify"
	"github.com/mattermost/focalboard/server/services/store"
	"github.com/mattermost/focalboard/server/services/store/mattermostauthlayer"
	"github.com/mattermost/focalboard/server/services/store/sqlstore"
	"github.com/mattermost/focalboard/server/ws"

	pluginapi "github.com/mattermost/mattermost-plugin-api"

	mmModel "github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/plugin"
	"github.com/mattermost/mattermost-server/v6/shared/markdown"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const PostEmbedBoards mmModel.PostEmbedType = "boards"

type BoardsEmbed struct {
	OriginalPath string `json:"originalPath"`
	WorkspaceID  string `json:"workspaceID"`
	ViewID       string `json:"viewID"`
	BoardID      string `json:"boardID"`
	CardID       string `json:"cardID"`
	ReadToken    string `json:"readToken,omitempty"`
}

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	server          *server.Server
	wsPluginAdapter ws.PluginAdapterInterface
}

func (p *Plugin) OnActivate() error {
	mmconfig := p.API.GetUnsanitizedConfig()
	filesS3Config := config.AmazonS3Config{}
	if mmconfig.FileSettings.AmazonS3AccessKeyId != nil {
		filesS3Config.AccessKeyID = *mmconfig.FileSettings.AmazonS3AccessKeyId
	}
	if mmconfig.FileSettings.AmazonS3SecretAccessKey != nil {
		filesS3Config.SecretAccessKey = *mmconfig.FileSettings.AmazonS3SecretAccessKey
	}
	if mmconfig.FileSettings.AmazonS3Bucket != nil {
		filesS3Config.Bucket = *mmconfig.FileSettings.AmazonS3Bucket
	}
	if mmconfig.FileSettings.AmazonS3PathPrefix != nil {
		filesS3Config.PathPrefix = *mmconfig.FileSettings.AmazonS3PathPrefix
	}
	if mmconfig.FileSettings.AmazonS3Region != nil {
		filesS3Config.Region = *mmconfig.FileSettings.AmazonS3Region
	}
	if mmconfig.FileSettings.AmazonS3Endpoint != nil {
		filesS3Config.Endpoint = *mmconfig.FileSettings.AmazonS3Endpoint
	}
	if mmconfig.FileSettings.AmazonS3SSL != nil {
		filesS3Config.SSL = *mmconfig.FileSettings.AmazonS3SSL
	}
	if mmconfig.FileSettings.AmazonS3SignV2 != nil {
		filesS3Config.SignV2 = *mmconfig.FileSettings.AmazonS3SignV2
	}
	if mmconfig.FileSettings.AmazonS3SSE != nil {
		filesS3Config.SSE = *mmconfig.FileSettings.AmazonS3SSE
	}
	if mmconfig.FileSettings.AmazonS3Trace != nil {
		filesS3Config.Trace = *mmconfig.FileSettings.AmazonS3Trace
	}

	client := pluginapi.NewClient(p.API, p.Driver)
	sqlDB, err := client.Store.GetMasterDB()
	if err != nil {
		return fmt.Errorf("error initializing the DB: %w", err)
	}

	logger, _ := mlog.NewLogger()
	pluginTargetFactory := newPluginTargetFactory(&client.Log)
	factories := &mlog.Factories{
		TargetFactory: pluginTargetFactory.createTarget,
	}
	cfgJSON := defaultLoggingConfig()
	err = logger.Configure("", cfgJSON, factories)
	if err != nil {
		return err
	}

	baseURL := ""
	if mmconfig.ServiceSettings.SiteURL != nil {
		baseURL = *mmconfig.ServiceSettings.SiteURL
	}

	serverID := client.System.GetDiagnosticID()

	enableTelemetry := false
	if mmconfig.LogSettings.EnableDiagnostics != nil {
		enableTelemetry = *mmconfig.LogSettings.EnableDiagnostics
	}

	enablePublicSharedBoards := false
	if mmconfig.PluginSettings.Plugins["focalboard"]["enablepublicsharedboards"] == true {
		enablePublicSharedBoards = true
	}

	cfg := &config.Configuration{
		ServerRoot:               baseURL + "/plugins/focalboard",
		Port:                     -1,
		DBType:                   *mmconfig.SqlSettings.DriverName,
		DBConfigString:           *mmconfig.SqlSettings.DataSource,
		DBTablePrefix:            "focalboard_",
		UseSSL:                   false,
		SecureCookie:             true,
		WebPath:                  path.Join(*mmconfig.PluginSettings.Directory, "focalboard", "pack"),
		FilesDriver:              *mmconfig.FileSettings.DriverName,
		FilesPath:                *mmconfig.FileSettings.Directory,
		FilesS3Config:            filesS3Config,
		Telemetry:                enableTelemetry,
		TelemetryID:              serverID,
		WebhookUpdate:            []string{},
		SessionExpireTime:        2592000,
		SessionRefreshTime:       18000,
		LocalOnly:                false,
		EnableLocalMode:          false,
		LocalModeSocketLocation:  "",
		AuthMode:                 "mattermost",
		EnablePublicSharedBoards: enablePublicSharedBoards,
	}
	var db store.Store
	db, err = sqlstore.New(cfg.DBType, cfg.DBConfigString, cfg.DBTablePrefix, logger, sqlDB, true)
	if err != nil {
		return fmt.Errorf("error initializing the DB: %w", err)
	}
	if cfg.AuthMode == server.MattermostAuthMod {
		layeredStore, err2 := mattermostauthlayer.New(cfg.DBType, sqlDB, db, logger)
		if err2 != nil {
			return fmt.Errorf("error initializing the DB: %w", err2)
		}
		db = layeredStore
	}

	p.wsPluginAdapter = ws.NewPluginAdapter(p.API, auth.New(cfg, db))

	mentionsBackend, err := createMentionsNotifyBackend(client, cfg.ServerRoot, logger)
	if err != nil {
		return fmt.Errorf("error creating mentions notifications backend: %w", err)
	}

	params := server.Params{
		Cfg:             cfg,
		SingleUserToken: "",
		DBStore:         db,
		Logger:          logger,
		ServerID:        serverID,
		WSAdapter:       p.wsPluginAdapter,
		NotifyBackends:  []notify.Backend{mentionsBackend},
	}

	server, err := server.New(params)
	if err != nil {
		fmt.Println("ERROR INITIALIZING THE SERVER", err)
		return err
	}

	p.server = server
	return server.Start()
}

func (p *Plugin) OnWebSocketConnect(webConnID, userID string) {
	p.wsPluginAdapter.OnWebSocketConnect(webConnID, userID)
}

func (p *Plugin) OnWebSocketDisconnect(webConnID, userID string) {
	p.wsPluginAdapter.OnWebSocketDisconnect(webConnID, userID)
}

func (p *Plugin) WebSocketMessageHasBeenPosted(webConnID, userID string, req *mmModel.WebSocketRequest) {
	p.wsPluginAdapter.WebSocketMessageHasBeenPosted(webConnID, userID, req)
}

func (p *Plugin) OnDeactivate() error {
	return p.server.Shutdown()
}

func (p *Plugin) OnPluginClusterEvent(_ *plugin.Context, ev mmModel.PluginClusterEvent) {
	p.wsPluginAdapter.HandleClusterEvent(ev)
}

// ServeHTTP demonstrates a plugin that handles HTTP requests by greeting the world.
func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	router := p.server.GetRootRouter()
	router.ServeHTTP(w, r)
}

func defaultLoggingConfig() string {
	return `
	{
		"def": {
			"type": "focalboard_plugin_adapter",
			"options": {},
			"format": "plain",
			"format_options": {
				"delim": " ",
				"min_level_len": 0,
				"min_msg_len": 0,
				"enable_color": false,
				"enable_caller": true
			},
			"levels": [
				{"id": 5, "name": "debug"},
				{"id": 4, "name": "info", "color": 36},
				{"id": 3, "name": "warn"},
				{"id": 2, "name": "error", "color": 31},
				{"id": 1, "name": "fatal", "stacktrace": true},
				{"id": 0, "name": "panic", "stacktrace": true}
			]
		}
	}`
}

func (p *Plugin) MessageWillBePosted(_ *plugin.Context, post *mmModel.Post) (*mmModel.Post, string) { //nolint
	firstLink := getFirstLink(post.Message)
	u, err := url.Parse(firstLink)

	if err != nil {
		return post, ""
	}

	// Trim away the first / because otherwise after we split the string, the first element in the array is a empty element
	path := strings.ToLower(u.Path[1:])
	pathSplit := strings.Split(path, "/")
	queryParams := u.Query()

	if len(pathSplit) == 0 {
		return post, ""
	}

	workspaceID := ""
	boardID := ""
	viewID := ""
	cardID := ""
	readToken := ""

	// If the first parameter in the path is boards,
	// then we've copied this directly as logged in user of that board

	// If the first parameter in the path is plugins,
	// then we've copied this from a shared board

	// For card links copied on a non-shared board, the path looks like boards/workspace/workspaceID/boardID/viewID/cardID

	// For card links copied on a shared board, the path looks like
	// plugins/focalboard/workspace/workspaceID/shared/boardID/viewID/cardID?r=read_token

	// This is a non-shared board card link
	if len(pathSplit) == 6 && pathSplit[0] == "boards" && pathSplit[1] == "workspace" {
		workspaceID = pathSplit[2]
		boardID = pathSplit[3]
		viewID = pathSplit[4]
		cardID = pathSplit[5]
	} else if len(pathSplit) == 8 && pathSplit[0] == "plugins" &&
		pathSplit[1] == "focalboard" && pathSplit[2] == "workspace" && pathSplit[4] == "shared" { // This is a shared board card link
		workspaceID = pathSplit[3]
		boardID = pathSplit[5]
		viewID = pathSplit[6]
		cardID = pathSplit[7]
		readToken = queryParams.Get("r")
	}

	if workspaceID != "" && boardID != "" && viewID != "" && cardID != "" {
		b, _ := json.Marshal(BoardsEmbed{
			WorkspaceID:  workspaceID,
			BoardID:      boardID,
			ViewID:       viewID,
			CardID:       cardID,
			ReadToken:    readToken,
			OriginalPath: u.RequestURI(),
		})

		BoardsPostEmbed := &mmModel.PostEmbed{
			Type: PostEmbedBoards,
			Data: string(b),
		}
		post.Metadata.Embeds = []*mmModel.PostEmbed{BoardsPostEmbed}
		post.AddProp("boards", string(b))
	}
	return post, ""
}

func getFirstLink(str string) string {
	firstLink := ""

	markdown.Inspect(str, func(blockOrInline interface{}) bool {
		if _, ok := blockOrInline.(*markdown.Autolink); ok {
			if link := blockOrInline.(*markdown.Autolink).Destination(); firstLink == "" {
				firstLink = link
			}
		}
		return true
	})

	return firstLink
}
