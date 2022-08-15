package model

import (
	mmModel "github.com/mattermost/mattermost-server/v6/model"
)

var (
	PermissionViewTeam              = mmModel.PermissionViewTeam
	PermissionReadChannel           = mmModel.PermissionReadChannel
	PermissionViewMembers           = mmModel.PermissionViewMembers
	PermissionCreatePublicChannel   = mmModel.PermissionCreatePublicChannel
	PermissionCreatePrivateChannel  = mmModel.PermissionCreatePrivateChannel
	PermissionManageBoardType       = &mmModel.Permission{Id: "manage_board_type", Name: "", Description: "", Scope: ""}
	PermissionDeleteBoard           = &mmModel.Permission{Id: "delete_board", Name: "", Description: "", Scope: ""}
	PermissionViewBoard             = &mmModel.Permission{Id: "view_board", Name: "", Description: "", Scope: ""}
	PermissionManageBoardRoles      = &mmModel.Permission{Id: "manage_board_roles", Name: "", Description: "", Scope: ""}
	PermissionShareBoard            = &mmModel.Permission{Id: "share_board", Name: "", Description: "", Scope: ""}
	PermissionManageBoardCards      = &mmModel.Permission{Id: "manage_board_cards", Name: "", Description: "", Scope: ""}
	PermissionManageBoardProperties = &mmModel.Permission{Id: "manage_board_properties", Name: "", Description: "", Scope: ""}
)
