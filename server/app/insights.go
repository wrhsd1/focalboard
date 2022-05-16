package app

import "github.com/mattermost/focalboard/server/model"

func (a *App) GetUserBoardsInsights(userID string, duration string) ([]*model.BoardInsight, error) {
	return a.store.GetUserBoardsInsights(userID, duration)
}

func (a *App) GetTeamBoardsInsights(teamID string, duration string) ([]*model.BoardInsight, error) {
	return a.store.GetTeamBoardsInsights(teamID, duration)
}
