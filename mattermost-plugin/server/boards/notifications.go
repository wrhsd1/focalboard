package boards

import (
	"fmt"
	"time"

	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/services/config"
	"github.com/mattermost/focalboard/server/services/notify/notifymentions"
	"github.com/mattermost/focalboard/server/services/notify/notifysubscriptions"
	"github.com/mattermost/focalboard/server/services/notify/plugindelivery"
	"github.com/mattermost/focalboard/server/services/permissions"
	"github.com/mattermost/focalboard/server/services/store"

	mm_model "github.com/mattermost/mattermost-server/v6/model"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	botUsername    = "boards"
	botDisplayname = "Boards"
	botDescription = "Created by Boards plugin."
)

type notifyBackendParams struct {
	cfg         *config.Configuration
	servicesAPI model.ServicesAPI
	permissions permissions.PermissionsService
	appAPI      *appAPI
	serverRoot  string
	logger      mlog.LoggerIFace
}

func createMentionsNotifyBackend(params notifyBackendParams) (*notifymentions.Backend, error) {
	delivery, err := createDelivery(params.servicesAPI, params.serverRoot)
	if err != nil {
		return nil, err
	}

	backendParams := notifymentions.BackendParams{
		AppAPI:      params.appAPI,
		Permissions: params.permissions,
		Delivery:    delivery,
		Logger:      params.logger,
	}

	backend := notifymentions.New(backendParams)

	return backend, nil
}

func createSubscriptionsNotifyBackend(params notifyBackendParams) (*notifysubscriptions.Backend, error) {
	delivery, err := createDelivery(params.servicesAPI, params.serverRoot)
	if err != nil {
		return nil, err
	}

	backendParams := notifysubscriptions.BackendParams{
		ServerRoot:             params.serverRoot,
		AppAPI:                 params.appAPI,
		Permissions:            params.permissions,
		Delivery:               delivery,
		Logger:                 params.logger,
		NotifyFreqCardSeconds:  params.cfg.NotifyFreqCardSeconds,
		NotifyFreqBoardSeconds: params.cfg.NotifyFreqBoardSeconds,
	}
	backend := notifysubscriptions.New(backendParams)

	return backend, nil
}

func createDelivery(servicesAPI model.ServicesAPI, serverRoot string) (*plugindelivery.PluginDelivery, error) {
	bot := &mm_model.Bot{
		Username:    botUsername,
		DisplayName: botDisplayname,
		Description: botDescription,
		OwnerId:     model.SystemUserID,
	}
	botID, err := servicesAPI.EnsureBot(bot)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure %s bot: %w", botDisplayname, err)
	}

	return plugindelivery.New(botID, serverRoot, servicesAPI), nil
}

type appIface interface {
	CreateSubscription(sub *model.Subscription) (*model.Subscription, error)
	AddMemberToBoard(member *model.BoardMember) (*model.BoardMember, error)
}

// appAPI provides app and store APIs for notification services. Where appropriate calls are made to the
// app layer to leverage the additional websocket notification logic present there, and other times the
// store APIs are called directly.
type appAPI struct {
	store store.Store
	app   appIface
}

func (a *appAPI) init(store store.Store, app appIface) {
	a.store = store
	a.app = app
}

func (a *appAPI) GetBlockHistory(blockID string, opts model.QueryBlockHistoryOptions) ([]model.Block, error) {
	return a.store.GetBlockHistory(blockID, opts)
}

func (a *appAPI) GetSubTree2(boardID, blockID string, opts model.QuerySubtreeOptions) ([]model.Block, error) {
	return a.store.GetSubTree2(boardID, blockID, opts)
}

func (a *appAPI) GetBoardAndCardByID(blockID string) (board *model.Board, card *model.Block, err error) {
	return a.store.GetBoardAndCardByID(blockID)
}

func (a *appAPI) GetUserByID(userID string) (*model.User, error) {
	return a.store.GetUserByID(userID)
}

func (a *appAPI) CreateSubscription(sub *model.Subscription) (*model.Subscription, error) {
	return a.app.CreateSubscription(sub)
}

func (a *appAPI) GetSubscribersForBlock(blockID string) ([]*model.Subscriber, error) {
	return a.store.GetSubscribersForBlock(blockID)
}

func (a *appAPI) UpdateSubscribersNotifiedAt(blockID string, notifyAt int64) error {
	return a.store.UpdateSubscribersNotifiedAt(blockID, notifyAt)
}

func (a *appAPI) UpsertNotificationHint(hint *model.NotificationHint, notificationFreq time.Duration) (*model.NotificationHint, error) {
	return a.store.UpsertNotificationHint(hint, notificationFreq)
}

func (a *appAPI) GetNextNotificationHint(remove bool) (*model.NotificationHint, error) {
	return a.store.GetNextNotificationHint(remove)
}

func (a *appAPI) GetMemberForBoard(boardID, userID string) (*model.BoardMember, error) {
	return a.store.GetMemberForBoard(boardID, userID)
}

func (a *appAPI) AddMemberToBoard(member *model.BoardMember) (*model.BoardMember, error) {
	return a.app.AddMemberToBoard(member)
}
