package sqlstore

import (
	"context"
	"fmt"
	"strconv"

	sq "github.com/Masterminds/squirrel"

	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/utils"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	// we group the inserts on batches of 1000 because PostgreSQL
	// supports a limit of around 64K values (not rows) on an insert
	// query, so we want to stay safely below.
	CategoryInsertBatch = 1000

	TemplatesToTeamsMigrationKey        = "TemplatesToTeamsMigrationComplete"
	UniqueIDsMigrationKey               = "UniqueIDsMigrationComplete"
	CategoryUUIDIDMigrationKey          = "CategoryUuidIdMigrationComplete"
	TeamLessBoardsMigrationKey          = "TeamLessBoardsMigrationComplete"
	DeletedMembershipBoardsMigrationKey = "DeletedMembershipBoardsMigrationComplete"
)

func (s *SQLStore) getBlocksWithSameID(db sq.BaseRunner) ([]model.Block, error) {
	subquery, _, _ := s.getQueryBuilder(db).
		Select("id").
		From(s.tablePrefix + "blocks").
		Having("count(id) > 1").
		GroupBy("id").
		ToSql()

	blocksFields := []string{
		"id",
		"parent_id",
		"root_id",
		"created_by",
		"modified_by",
		s.escapeField("schema"),
		"type",
		"title",
		"COALESCE(fields, '{}')",
		s.timestampToCharField("insert_at", "insertAt"),
		"create_at",
		"update_at",
		"delete_at",
		"COALESCE(workspace_id, '0')",
	}

	rows, err := s.getQueryBuilder(db).
		Select(blocksFields...).
		From(s.tablePrefix + "blocks").
		Where(fmt.Sprintf("id IN (%s)", subquery)).
		Query()
	if err != nil {
		s.logger.Error(`getBlocksWithSameID ERROR`, mlog.Err(err))
		return nil, err
	}
	defer s.CloseRows(rows)

	return s.blocksFromRows(rows)
}

func (s *SQLStore) runUniqueIDsMigration() error {
	setting, err := s.GetSystemSetting(UniqueIDsMigrationKey)
	if err != nil {
		return fmt.Errorf("cannot get migration state: %w", err)
	}

	// If the migration is already completed, do not run it again.
	if hasAlreadyRun, _ := strconv.ParseBool(setting); hasAlreadyRun {
		return nil
	}

	s.logger.Debug("Running Unique IDs migration")

	tx, txErr := s.db.BeginTx(context.Background(), nil)
	if txErr != nil {
		return txErr
	}

	blocks, err := s.getBlocksWithSameID(tx)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			s.logger.Error("Unique IDs transaction rollback error", mlog.Err(rollbackErr), mlog.String("methodName", "getBlocksWithSameID"))
		}
		return fmt.Errorf("cannot get blocks with same ID: %w", err)
	}

	blocksByID := map[string][]model.Block{}
	for _, block := range blocks {
		blocksByID[block.ID] = append(blocksByID[block.ID], block)
	}

	for _, blocks := range blocksByID {
		for i, block := range blocks {
			if i == 0 {
				// do nothing for the first ID, only updating the others
				continue
			}

			newID := utils.NewID(model.BlockType2IDType(block.Type))
			if err := s.replaceBlockID(tx, block.ID, newID, block.WorkspaceID); err != nil {
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					s.logger.Error("Unique IDs transaction rollback error", mlog.Err(rollbackErr), mlog.String("methodName", "replaceBlockID"))
				}
				return fmt.Errorf("cannot replace blockID %s: %w", block.ID, err)
			}
		}
	}

	if err := s.setSystemSetting(tx, UniqueIDsMigrationKey, strconv.FormatBool(true)); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			s.logger.Error("Unique IDs transaction rollback error", mlog.Err(rollbackErr), mlog.String("methodName", "setSystemSetting"))
		}
		return fmt.Errorf("cannot mark migration as completed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit unique IDs transaction: %w", err)
	}

	s.logger.Debug("Unique IDs migration finished successfully")
	return nil
}

// runCategoryUUIDIDMigration takes care of deriving the categories
// from the boards and its memberships. The name references UUID
// because of the preexisting purpose of this migration, and has been
// preserved for compatibility with already migrated instances.
func (s *SQLStore) runCategoryUUIDIDMigration() error {
	setting, err := s.GetSystemSetting(CategoryUUIDIDMigrationKey)
	if err != nil {
		return fmt.Errorf("cannot get migration state: %w", err)
	}

	// If the migration is already completed, do not run it again.
	if hasAlreadyRun, _ := strconv.ParseBool(setting); hasAlreadyRun {
		return nil
	}

	s.logger.Debug("Running category UUID ID migration")

	tx, txErr := s.db.BeginTx(context.Background(), nil)
	if txErr != nil {
		return txErr
	}

	if s.isPlugin {
		if err := s.createCategories(tx); err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				s.logger.Error("category UUIDs insert categories transaction rollback error", mlog.Err(rollbackErr), mlog.String("methodName", "setSystemSetting"))
			}
			return err
		}

		if err := s.createCategoryBoards(tx); err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				s.logger.Error("category UUIDs insert category boards transaction rollback error", mlog.Err(rollbackErr), mlog.String("methodName", "setSystemSetting"))
			}
			return err
		}
	}

	if err := s.setSystemSetting(tx, CategoryUUIDIDMigrationKey, strconv.FormatBool(true)); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			s.logger.Error("category UUIDs transaction rollback error", mlog.Err(rollbackErr), mlog.String("methodName", "setSystemSetting"))
		}
		return fmt.Errorf("cannot mark migration as completed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit category UUIDs transaction: %w", err)
	}

	s.logger.Debug("category UUIDs migration finished successfully")
	return nil
}

func (s *SQLStore) createCategories(db sq.BaseRunner) error {
	rows, err := s.getQueryBuilder(db).
		Select("c.DisplayName, cm.UserId, c.TeamId, cm.ChannelId").
		From(s.tablePrefix + "boards boards").
		Join("ChannelMembers cm on boards.channel_id = cm.ChannelId").
		Join("Channels c on cm.ChannelId = c.id and (c.Type = 'O' or c.Type = 'P')").
		GroupBy("cm.UserId, c.TeamId, cm.ChannelId, c.DisplayName").
		Query()

	if err != nil {
		s.logger.Error("get boards data error", mlog.Err(err))
		return err
	}
	defer s.CloseRows(rows)

	initQuery := func() sq.InsertBuilder {
		return s.getQueryBuilder(db).
			Insert(s.tablePrefix+"categories").
			Columns(
				"id",
				"name",
				"user_id",
				"team_id",
				"channel_id",
				"create_at",
				"update_at",
				"delete_at",
			)
	}
	// query will accumulate the insert values until the limit is
	// reached, and then it will be stored and reset
	query := initQuery()
	// queryList stores those queries that already reached the limit
	// to be run when all the data is processed
	queryList := []sq.InsertBuilder{}
	counter := 0
	now := model.GetMillis()

	for rows.Next() {
		var displayName string
		var userID string
		var teamID string
		var channelID string

		err := rows.Scan(
			&displayName,
			&userID,
			&teamID,
			&channelID,
		)
		if err != nil {
			return fmt.Errorf("cannot scan result while trying to create categories: %w", err)
		}

		query = query.Values(
			utils.NewID(utils.IDTypeNone),
			displayName,
			userID,
			teamID,
			channelID,
			now,
			0,
			0,
		)

		counter++
		if counter%CategoryInsertBatch == 0 {
			queryList = append(queryList, query)
			query = initQuery()
		}
	}

	if counter%CategoryInsertBatch != 0 {
		queryList = append(queryList, query)
	}

	for _, q := range queryList {
		if _, err := q.Exec(); err != nil {
			return fmt.Errorf("cannot create category values: %w", err)
		}
	}

	return nil
}

func (s *SQLStore) createCategoryBoards(db sq.BaseRunner) error {
	rows, err := s.getQueryBuilder(db).
		Select("categories.user_id, categories.id, boards.id").
		From(s.tablePrefix + "categories categories").
		Join(s.tablePrefix + "boards boards on categories.channel_id = boards.channel_id AND boards.is_template = false").
		Query()

	if err != nil {
		s.logger.Error("get categories data error", mlog.Err(err))
		return err
	}
	defer s.CloseRows(rows)

	initQuery := func() sq.InsertBuilder {
		return s.getQueryBuilder(db).
			Insert(s.tablePrefix+"category_boards").
			Columns(
				"id",
				"user_id",
				"category_id",
				"board_id",
				"create_at",
				"update_at",
				"delete_at",
			)
	}
	// query will accumulate the insert values until the limit is
	// reached, and then it will be stored and reset
	query := initQuery()
	// queryList stores those queries that already reached the limit
	// to be run when all the data is processed
	queryList := []sq.InsertBuilder{}
	counter := 0
	now := model.GetMillis()

	for rows.Next() {
		var userID string
		var categoryID string
		var boardID string

		err := rows.Scan(
			&userID,
			&categoryID,
			&boardID,
		)
		if err != nil {
			return fmt.Errorf("cannot scan result while trying to create category boards: %w", err)
		}

		query = query.Values(
			utils.NewID(utils.IDTypeNone),
			userID,
			categoryID,
			boardID,
			now,
			0,
			0,
		)

		counter++
		if counter%CategoryInsertBatch == 0 {
			queryList = append(queryList, query)
			query = initQuery()
		}
	}

	if counter%CategoryInsertBatch != 0 {
		queryList = append(queryList, query)
	}

	for _, q := range queryList {
		if _, err := q.Exec(); err != nil {
			return fmt.Errorf("cannot create category boards values: %w", err)
		}
	}

	return nil
}

// We no longer support boards existing in DMs and private
// group messages. This function migrates all boards
// belonging to a DM to the best possible team.
func (s *SQLStore) runTeamLessBoardsMigration() error {
	if !s.isPlugin {
		return nil
	}

	setting, err := s.GetSystemSetting(TeamLessBoardsMigrationKey)
	if err != nil {
		return fmt.Errorf("cannot get teamless boards migration state: %w", err)
	}

	// If the migration is already completed, do not run it again.
	if hasAlreadyRun, _ := strconv.ParseBool(setting); hasAlreadyRun {
		return nil
	}

	boards, err := s.getDMBoards(s.db)
	if err != nil {
		return err
	}

	s.logger.Debug("Migrating teamless boards to a team", mlog.Int("count", len(boards)))

	// cache for best suitable team for a DM. Since a DM can
	// contain multiple boards, caching this avoids
	// duplicate queries for the same DM.
	channelToTeamCache := map[string]string{}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		s.logger.Error("error starting transaction in runTeamLessBoardsMigration", mlog.Err(err))
		return err
	}

	for i := range boards {
		// check the cache first
		teamID, ok := channelToTeamCache[boards[i].ChannelID]

		// query DB if entry not found in cache
		if !ok {
			teamID, err = s.getBestTeamForBoard(s.db, boards[i])
			if err != nil {
				// don't let one board's error spoil
				// the mood for others
				s.logger.Error("could not find the best team for board during team less boards migration. Continuing", mlog.String("boardID", boards[i].ID))
				continue
			}
		}

		channelToTeamCache[boards[i].ChannelID] = teamID
		boards[i].TeamID = teamID

		query := s.getQueryBuilder(tx).
			Update(s.tablePrefix+"boards").
			Set("team_id", teamID).
			Set("type", model.BoardTypePrivate).
			Where(sq.Eq{"id": boards[i].ID})

		if _, err := query.Exec(); err != nil {
			s.logger.Error("failed to set team id for board", mlog.String("board_id", boards[i].ID), mlog.String("team_id", teamID), mlog.Err(err))
			return err
		}
	}

	if err := s.setSystemSetting(tx, TeamLessBoardsMigrationKey, strconv.FormatBool(true)); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			s.logger.Error("transaction rollback error", mlog.Err(rollbackErr), mlog.String("methodName", "runTeamLessBoardsMigration"))
		}
		return fmt.Errorf("cannot mark migration as completed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("failed to commit runTeamLessBoardsMigration transaction", mlog.Err(err))
		return err
	}

	return nil
}

func (s *SQLStore) getDMBoards(tx sq.BaseRunner) ([]*model.Board, error) {
	conditions := sq.And{
		sq.Eq{"team_id": ""},
		sq.Or{
			sq.Eq{"type": "D"},
			sq.Eq{"type": "G"},
		},
	}

	boards, err := s.getLegacyBoardsByCondition(tx, conditions)
	if err != nil && model.IsErrNotFound(err) {
		return []*model.Board{}, nil
	}

	return boards, err
}

// The destination is selected as the first team where all members
// of the DM are a part of. If no such team exists,
// we use the first team to which DM creator belongs to.
func (s *SQLStore) getBestTeamForBoard(tx sq.BaseRunner, board *model.Board) (string, error) {
	userTeams, err := s.getBoardUserTeams(tx, board)
	if err != nil {
		return "", err
	}

	teams := [][]interface{}{}
	for _, userTeam := range userTeams {
		userTeamInterfaces := make([]interface{}, len(userTeam))
		for i := range userTeam {
			userTeamInterfaces[i] = userTeam[i]
		}
		teams = append(teams, userTeamInterfaces)
	}

	commonTeams := utils.Intersection(teams...)
	var teamID string
	if len(commonTeams) > 0 {
		teamID = commonTeams[0].(string)
	} else {
		// no common teams found. Let's try finding the best suitable team
		if board.Type == "D" {
			// get DM's creator and pick one of their team
			channel, err := (s.servicesAPI).GetChannelByID(board.ChannelID)
			if err != nil {
				s.logger.Error("failed to fetch DM channel for board",
					mlog.String("board_id", board.ID),
					mlog.String("channel_id", board.ChannelID),
					mlog.Err(err),
				)
				return "", err
			}

			if _, ok := userTeams[channel.CreatorId]; !ok {
				s.logger.Error("channel creator not found in user teams",
					mlog.String("board_id", board.ID),
					mlog.String("channel_id", board.ChannelID),
					mlog.String("creator_id", channel.CreatorId),
				)
				err := fmt.Errorf("%w board_id: %s, channel_id: %s, creator_id: %s", errChannelCreatorNotInTeam, board.ID, board.ChannelID, channel.CreatorId)
				return "", err
			}

			teamID = userTeams[channel.CreatorId][0]
		} else if board.Type == "G" {
			// pick the team that has the most users as members
			teamFrequency := map[string]int{}
			highestFrequencyTeam := ""
			highestFrequencyTeamFrequency := -1

			for _, teams := range userTeams {
				for _, teamID := range teams {
					teamFrequency[teamID]++

					if teamFrequency[teamID] > highestFrequencyTeamFrequency {
						highestFrequencyTeamFrequency = teamFrequency[teamID]
						highestFrequencyTeam = teamID
					}
				}
			}

			teamID = highestFrequencyTeam
		}
	}

	return teamID, nil
}

func (s *SQLStore) getBoardUserTeams(tx sq.BaseRunner, board *model.Board) (map[string][]string, error) {
	query := s.getQueryBuilder(tx).
		Select("tm.UserId", "tm.TeamId").
		From("ChannelMembers cm").
		Join("TeamMembers tm ON cm.UserId = tm.UserId").
		Join("Teams t ON tm.TeamId = t.Id").
		Where(sq.Eq{
			"cm.ChannelId": board.ChannelID,
			"t.DeleteAt":   0,
			"tm.DeleteAt":  0,
		})

	rows, err := query.Query()
	if err != nil {
		s.logger.Error("failed to fetch user teams for board", mlog.String("boardID", board.ID), mlog.String("channelID", board.ChannelID), mlog.Err(err))
		return nil, err
	}

	defer rows.Close()

	userTeams := map[string][]string{}

	for rows.Next() {
		var userID, teamID string
		err := rows.Scan(&userID, &teamID)
		if err != nil {
			s.logger.Error("getBoardUserTeams failed to scan SQL query result", mlog.String("boardID", board.ID), mlog.String("channelID", board.ChannelID), mlog.Err(err))
			return nil, err
		}

		userTeams[userID] = append(userTeams[userID], teamID)
	}

	return userTeams, nil
}

func (s *SQLStore) runDeletedMembershipBoardsMigration() error {
	if !s.isPlugin {
		return nil
	}

	setting, err := s.GetSystemSetting(DeletedMembershipBoardsMigrationKey)
	if err != nil {
		return fmt.Errorf("cannot get deleted membership boards migration state: %w", err)
	}

	// If the migration is already completed, do not run it again.
	if hasAlreadyRun, _ := strconv.ParseBool(setting); hasAlreadyRun {
		return nil
	}

	boards, err := s.getDeletedMembershipBoards(s.db)
	if err != nil {
		return err
	}

	if len(boards) == 0 {
		s.logger.Debug("No boards with owner not anymore on their team found, marking runDeletedMembershipBoardsMigration as done")
		if sErr := s.SetSystemSetting(DeletedMembershipBoardsMigrationKey, strconv.FormatBool(true)); sErr != nil {
			return fmt.Errorf("cannot mark migration as completed: %w", sErr)
		}
		return nil
	}

	s.logger.Debug("Migrating boards with owner not anymore on their team", mlog.Int("count", len(boards)))

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		s.logger.Error("error starting transaction in runDeletedMembershipBoardsMigration", mlog.Err(err))
		return err
	}

	for i := range boards {
		teamID, err := s.getBestTeamForBoard(s.db, boards[i])
		if err != nil {
			// don't let one board's error spoil
			// the mood for others
			s.logger.Error("could not find the best team for board during deleted membership boards migration. Continuing", mlog.String("boardID", boards[i].ID))
			continue
		}

		boards[i].TeamID = teamID

		query := s.getQueryBuilder(tx).
			Update(s.tablePrefix+"boards").
			Set("team_id", teamID).
			Where(sq.Eq{"id": boards[i].ID})

		if _, err := query.Exec(); err != nil {
			s.logger.Error("failed to set team id for board", mlog.String("board_id", boards[i].ID), mlog.String("team_id", teamID), mlog.Err(err))
			return err
		}
	}

	if err := s.setSystemSetting(tx, DeletedMembershipBoardsMigrationKey, strconv.FormatBool(true)); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			s.logger.Error("transaction rollback error", mlog.Err(rollbackErr), mlog.String("methodName", "runDeletedMembershipBoardsMigration"))
		}
		return fmt.Errorf("cannot mark migration as completed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("failed to commit runDeletedMembershipBoardsMigration transaction", mlog.Err(err))
		return err
	}

	return nil
}

// getDeletedMembershipBoards retrieves those boards whose creator is
// associated to the board's team with a deleted team membership.
func (s *SQLStore) getDeletedMembershipBoards(tx sq.BaseRunner) ([]*model.Board, error) {
	rows, err := s.getQueryBuilder(tx).
		Select(legacyBoardFields("b.")...).
		From(s.tablePrefix + "boards b").
		Join("TeamMembers tm ON b.created_by = tm.UserId").
		Where("b.team_id = tm.TeamId").
		Where(sq.NotEq{"tm.DeleteAt": 0}).
		Query()
	if err != nil {
		return nil, err
	}
	defer s.CloseRows(rows)

	boards, err := s.boardsFromRows(rows)
	if err != nil {
		return nil, err
	}

	return boards, err
}
