// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"errors"
	"fmt"

	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/services/notify"
	"github.com/mattermost/focalboard/server/utils"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

var (
	ErrBoardMemberIsLastAdmin = errors.New("cannot leave a board with no admins")
	ErrNewBoardCannotHaveID   = errors.New("new board cannot have an ID")
	ErrInsufficientLicense    = errors.New("appropriate license required")
)

func (a *App) GetBoard(boardID string) (*model.Board, error) {
	board, err := a.store.GetBoard(boardID)
	if model.IsErrNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return board, nil
}

func (a *App) GetBoardMetadata(boardID string) (*model.Board, *model.BoardMetadata, error) {
	license := a.store.GetLicense()
	if license == nil || !(*license.Features.Compliance) {
		return nil, nil, ErrInsufficientLicense
	}

	board, err := a.GetBoard(boardID)
	if err != nil {
		return nil, nil, err
	}
	if board == nil {
		// Board may have been deleted, retrieve most recent history instead
		board, err = a.getBoardHistory(boardID, true)
		if err != nil {
			return nil, nil, err
		}
	}

	if board == nil {
		// Board not found
		return nil, nil, nil
	}

	earliestTime, _, err := a.getBoardDescendantModifiedInfo(boardID, false)
	if err != nil {
		return nil, nil, err
	}

	latestTime, lastModifiedBy, err := a.getBoardDescendantModifiedInfo(boardID, true)
	if err != nil {
		return nil, nil, err
	}

	boardMetadata := model.BoardMetadata{
		BoardID:                 boardID,
		DescendantFirstUpdateAt: earliestTime,
		DescendantLastUpdateAt:  latestTime,
		CreatedBy:               board.CreatedBy,
		LastModifiedBy:          lastModifiedBy,
	}
	return board, &boardMetadata, nil
}

// getBoardForBlock returns the board that owns the specified block.
func (a *App) getBoardForBlock(blockID string) (*model.Board, error) {
	block, err := a.GetBlockByID(blockID)
	if err != nil {
		return nil, fmt.Errorf("cannot get block %s: %w", blockID, err)
	}

	board, err := a.GetBoard(block.BoardID)
	if err != nil {
		return nil, fmt.Errorf("cannot get board %s: %w", block.BoardID, err)
	}

	return board, nil
}

func (a *App) getBoardHistory(boardID string, latest bool) (*model.Board, error) {
	opts := model.QueryBoardHistoryOptions{
		Limit:      1,
		Descending: latest,
	}
	boards, err := a.store.GetBoardHistory(boardID, opts)
	if err != nil {
		return nil, fmt.Errorf("could not get history for board: %w", err)
	}
	if len(boards) == 0 {
		return nil, nil
	}

	return boards[0], nil
}

func (a *App) getBoardDescendantModifiedInfo(boardID string, latest bool) (int64, string, error) {
	board, err := a.getBoardHistory(boardID, latest)
	if err != nil {
		return 0, "", err
	}
	if board == nil {
		return 0, "", fmt.Errorf("history not found for board: %w", err)
	}

	var timestamp int64
	modifiedBy := board.ModifiedBy
	if latest {
		timestamp = board.UpdateAt
	} else {
		timestamp = board.CreateAt
	}

	// use block_history to fetch blocks in case they were deleted and no longer exist in blocks table.
	opts := model.QueryBlockHistoryOptions{
		Limit:      1,
		Descending: latest,
	}
	blocks, err := a.store.GetBlockHistoryDescendants(boardID, opts)
	if err != nil {
		return 0, "", fmt.Errorf("could not get blocks history descendants for board: %w", err)
	}
	if len(blocks) > 0 {
		// Compare the board history info with the descendant block info, if it exists
		block := &blocks[0]
		if latest && block.UpdateAt > timestamp {
			timestamp = block.UpdateAt
			modifiedBy = block.ModifiedBy
		} else if !latest && block.CreateAt < timestamp {
			timestamp = block.CreateAt
			modifiedBy = block.ModifiedBy
		}
	}
	return timestamp, modifiedBy, nil
}

func (a *App) setBoardCategoryFromSource(sourceBoardID, destinationBoardID, userID, teamID string) error {
	// find source board's category ID for the user
	userCategoryBoards, err := a.GetUserCategoryBoards(userID, teamID)
	if err != nil {
		return err
	}

	var destinationCategoryID string

	for _, categoryBoard := range userCategoryBoards {
		for _, boardID := range categoryBoard.BoardIDs {
			if boardID == sourceBoardID {
				// category found!
				destinationCategoryID = categoryBoard.ID
				break
			}
		}
	}

	// if source board is not mapped to a category for this user,
	// then we have nothing more to do.
	if destinationCategoryID == "" {
		return nil
	}

	// now that we have source board's category,
	// we send destination board to the same category
	return a.AddUpdateUserCategoryBoard(teamID, userID, destinationCategoryID, destinationBoardID)
}

func (a *App) DuplicateBoard(boardID, userID, toTeam string, asTemplate bool) (*model.BoardsAndBlocks, []*model.BoardMember, error) {
	bab, members, err := a.store.DuplicateBoard(boardID, userID, toTeam, asTemplate)
	if err != nil {
		return nil, nil, err
	}

	// copy any file attachments from the duplicated blocks.
	if err = a.CopyCardFiles(boardID, bab.Blocks); err != nil {
		a.logger.Error("Could not copy files while duplicating board", mlog.String("BoardID", boardID), mlog.Err(err))
	}

	for _, board := range bab.Boards {
		if categoryErr := a.setBoardCategoryFromSource(boardID, board.ID, userID, board.TeamID); categoryErr != nil {
			return nil, nil, categoryErr
		}
	}

	// bab.Blocks now has updated file ids for any blocks containing files.  We need to store them.
	blockIDs := make([]string, 0)
	blockPatches := make([]model.BlockPatch, 0)

	for _, block := range bab.Blocks {
		if fileID, ok := block.Fields["fileId"]; ok {
			blockIDs = append(blockIDs, block.ID)
			blockPatches = append(blockPatches, model.BlockPatch{
				UpdatedFields: map[string]interface{}{
					"fileId": fileID,
				},
			})
		}
	}
	a.logger.Debug("Duplicate boards patching file IDs", mlog.Int("count", len(blockIDs)))

	if len(blockIDs) != 0 {
		patches := &model.BlockPatchBatch{
			BlockIDs:     blockIDs,
			BlockPatches: blockPatches,
		}
		if err = a.store.PatchBlocks(patches, userID); err != nil {
			dbab := model.NewDeleteBoardsAndBlocksFromBabs(bab)
			if err = a.store.DeleteBoardsAndBlocks(dbab, userID); err != nil {
				a.logger.Error("Cannot delete board after duplication error when updating block's file info", mlog.String("boardID", bab.Boards[0].ID), mlog.Err(err))
			}
			return nil, nil, fmt.Errorf("could not patch file IDs while duplicating board %s: %w", boardID, err)
		}
	}

	a.blockChangeNotifier.Enqueue(func() error {
		teamID := ""
		for _, board := range bab.Boards {
			teamID = board.TeamID
			a.wsAdapter.BroadcastBoardChange(teamID, board)
		}
		for _, block := range bab.Blocks {
			blk := block
			a.wsAdapter.BroadcastBlockChange(teamID, blk)
			a.notifyBlockChanged(notify.Add, &blk, nil, userID)
		}
		for _, member := range members {
			a.wsAdapter.BroadcastMemberChange(teamID, member.BoardID, member)
		}
		return nil
	})

	if len(bab.Blocks) != 0 {
		go func() {
			if uErr := a.UpdateCardLimitTimestamp(); uErr != nil {
				a.logger.Error(
					"UpdateCardLimitTimestamp failed after duplicating a board",
					mlog.Err(uErr),
				)
			}
		}()
	}

	return bab, members, err
}

func (a *App) GetBoardsForUserAndTeam(userID, teamID string) ([]*model.Board, error) {
	return a.store.GetBoardsForUserAndTeam(userID, teamID)
}

func (a *App) GetTemplateBoards(teamID, userID string) ([]*model.Board, error) {
	return a.store.GetTemplateBoards(teamID, userID)
}

func (a *App) CreateBoard(board *model.Board, userID string, addMember bool) (*model.Board, error) {
	if board.ID != "" {
		return nil, ErrNewBoardCannotHaveID
	}
	board.ID = utils.NewID(utils.IDTypeBoard)

	var newBoard *model.Board
	var member *model.BoardMember
	var err error
	if addMember {
		newBoard, member, err = a.store.InsertBoardWithAdmin(board, userID)
	} else {
		newBoard, err = a.store.InsertBoard(board, userID)
	}

	if err != nil {
		return nil, err
	}

	a.blockChangeNotifier.Enqueue(func() error {
		a.wsAdapter.BroadcastBoardChange(newBoard.TeamID, newBoard)

		if newBoard.ChannelID != "" {
			members, err := a.GetMembersForBoard(board.ID)
			if err != nil {
				a.logger.Error("Unable to get the board members", mlog.Err(err))
			}
			for _, member := range members {
				a.wsAdapter.BroadcastMemberChange(newBoard.TeamID, member.BoardID, member)
			}
		} else if addMember {
			a.wsAdapter.BroadcastMemberChange(newBoard.TeamID, newBoard.ID, member)
		}
		return nil
	})

	return newBoard, nil
}

func (a *App) PatchBoard(patch *model.BoardPatch, boardID, userID string) (*model.Board, error) {
	var oldMembers []*model.BoardMember
	if patch.ChannelID != nil && *patch.ChannelID == "" {
		var err error
		oldMembers, err = a.GetMembersForBoard(boardID)
		if err != nil {
			a.logger.Error("Unable to get the board members", mlog.Err(err))
		}
	}
	updatedBoard, err := a.store.PatchBoard(boardID, patch, userID)
	if err != nil {
		return nil, err
	}

	a.blockChangeNotifier.Enqueue(func() error {
		a.wsAdapter.BroadcastBoardChange(updatedBoard.TeamID, updatedBoard)
		if patch.ChannelID != nil && *patch.ChannelID != "" {
			members, err := a.GetMembersForBoard(updatedBoard.ID)
			if err != nil {
				a.logger.Error("Unable to get the board members", mlog.Err(err))
			}
			for _, member := range members {
				if member.Synthetic {
					a.wsAdapter.BroadcastMemberChange(updatedBoard.TeamID, member.BoardID, member)
				}
			}
		} else if patch.ChannelID != nil && *patch.ChannelID == "" {
			for _, oldMember := range oldMembers {
				if oldMember.Synthetic {
					a.wsAdapter.BroadcastMemberDelete(updatedBoard.TeamID, boardID, oldMember.UserID)
				}
			}
		}
		return nil
	})

	return updatedBoard, nil
}

func (a *App) DeleteBoard(boardID, userID string) error {
	board, err := a.store.GetBoard(boardID)
	if model.IsErrNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := a.store.DeleteBoard(boardID, userID); err != nil {
		return err
	}

	a.blockChangeNotifier.Enqueue(func() error {
		a.wsAdapter.BroadcastBoardDelete(board.TeamID, boardID)
		return nil
	})

	go func() {
		if err := a.UpdateCardLimitTimestamp(); err != nil {
			a.logger.Error(
				"UpdateCardLimitTimestamp failed after deleting a board",
				mlog.Err(err),
			)
		}
	}()

	return nil
}

func (a *App) GetMembersForBoard(boardID string) ([]*model.BoardMember, error) {
	return a.store.GetMembersForBoard(boardID)
}

func (a *App) GetMembersForUser(userID string) ([]*model.BoardMember, error) {
	return a.store.GetMembersForUser(userID)
}

func (a *App) GetMemberForBoard(boardID string, userID string) (*model.BoardMember, error) {
	return a.store.GetMemberForBoard(boardID, userID)
}

func (a *App) AddMemberToBoard(member *model.BoardMember) (*model.BoardMember, error) {
	board, err := a.store.GetBoard(member.BoardID)
	if model.IsErrNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	existingMembership, err := a.store.GetMemberForBoard(member.BoardID, member.UserID)
	if err != nil && !model.IsErrNotFound(err) {
		return nil, err
	}

	if existingMembership != nil && !existingMembership.Synthetic {
		return existingMembership, nil
	}

	newMember, err := a.store.SaveMember(member)
	if err != nil {
		return nil, err
	}

	a.blockChangeNotifier.Enqueue(func() error {
		a.wsAdapter.BroadcastMemberChange(board.TeamID, member.BoardID, member)
		return nil
	})

	return newMember, nil
}

func (a *App) UpdateBoardMember(member *model.BoardMember) (*model.BoardMember, error) {
	board, bErr := a.store.GetBoard(member.BoardID)
	if model.IsErrNotFound(bErr) {
		return nil, nil
	}
	if bErr != nil {
		return nil, bErr
	}

	oldMember, err := a.store.GetMemberForBoard(member.BoardID, member.UserID)
	if model.IsErrNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// if we're updating an admin, we need to check that there is at
	// least still another admin on the board
	if oldMember.SchemeAdmin && !member.SchemeAdmin {
		isLastAdmin, err2 := a.isLastAdmin(member.UserID, member.BoardID)
		if err2 != nil {
			return nil, err2
		}
		if isLastAdmin {
			return nil, ErrBoardMemberIsLastAdmin
		}
	}

	newMember, err := a.store.SaveMember(member)
	if err != nil {
		return nil, err
	}

	a.blockChangeNotifier.Enqueue(func() error {
		a.wsAdapter.BroadcastMemberChange(board.TeamID, member.BoardID, member)
		return nil
	})

	return newMember, nil
}

func (a *App) isLastAdmin(userID, boardID string) (bool, error) {
	members, err := a.store.GetMembersForBoard(boardID)
	if err != nil {
		return false, err
	}

	for _, m := range members {
		if m.SchemeAdmin && m.UserID != userID {
			return false, nil
		}
	}
	return true, nil
}

func (a *App) DeleteBoardMember(boardID, userID string) error {
	board, bErr := a.store.GetBoard(boardID)
	if model.IsErrNotFound(bErr) {
		return nil
	}
	if bErr != nil {
		return bErr
	}

	oldMember, err := a.store.GetMemberForBoard(boardID, userID)
	if model.IsErrNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// if we're removing an admin, we need to check that there is at
	// least still another admin on the board
	if oldMember.SchemeAdmin {
		isLastAdmin, err := a.isLastAdmin(userID, boardID)
		if err != nil {
			return err
		}
		if isLastAdmin {
			return ErrBoardMemberIsLastAdmin
		}
	}

	if err := a.store.DeleteMember(boardID, userID); err != nil {
		return err
	}

	a.blockChangeNotifier.Enqueue(func() error {
		if synteticMember, _ := a.store.GetMemberForBoard(boardID, userID); synteticMember != nil {
			a.wsAdapter.BroadcastMemberChange(board.TeamID, boardID, synteticMember)
		} else {
			a.wsAdapter.BroadcastMemberDelete(board.TeamID, boardID, userID)
		}
		return nil
	})

	return nil
}

func (a *App) SearchBoardsForUser(term, userID string) ([]*model.Board, error) {
	return a.store.SearchBoardsForUser(term, userID)
}

func (a *App) SearchBoardsForUserInTeam(teamID, term, userID string) ([]*model.Board, error) {
	return a.store.SearchBoardsForUserInTeam(teamID, term, userID)
}

func (a *App) UndeleteBoard(boardID string, modifiedBy string) error {
	boards, err := a.store.GetBoardHistory(boardID, model.QueryBoardHistoryOptions{Limit: 1, Descending: true})
	if err != nil {
		return err
	}

	if len(boards) == 0 {
		// undeleting non-existing board not considered an error
		return nil
	}

	err = a.store.UndeleteBoard(boardID, modifiedBy)
	if err != nil {
		return err
	}

	board, err := a.store.GetBoard(boardID)
	if err != nil {
		return err
	}

	if board == nil {
		a.logger.Error("Error loading the board after undelete, not propagating through websockets or notifications")
		return nil
	}

	a.blockChangeNotifier.Enqueue(func() error {
		a.wsAdapter.BroadcastBoardChange(board.TeamID, board)
		return nil
	})

	go func() {
		if err := a.UpdateCardLimitTimestamp(); err != nil {
			a.logger.Error(
				"UpdateCardLimitTimestamp failed after undeleting a board",
				mlog.Err(err),
			)
		}
	}()

	return nil
}
