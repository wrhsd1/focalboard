package sqlstore

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/mattermost/focalboard/server/model"

	sq "github.com/Masterminds/squirrel"
	mmModel "github.com/mattermost/mattermost-server/v6/model"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

func (s *SQLStore) getTeamBoardsInsights(db sq.BaseRunner, teamID string, userID string, since int64, offset int, limit int, boardIDs []string) (*model.BoardInsightsList, error) {
	boardsHistoryQuery := s.getQueryBuilder(db).
		Select("boards.id, boards.icon, boards.title, count(boards_history.id) as count, boards_history.modified_by, boards.created_by").
		From(s.tablePrefix + "boards_history as boards_history").
		Join(s.tablePrefix + "boards as boards on boards_history.id = boards.id").
		Where(sq.Gt{"boards_history.insert_at": mmModel.GetTimeForMillis(since).Format(time.RFC3339)}).
		Where(sq.Eq{"boards.team_id": teamID}).
		Where(sq.Eq{"boards.id": boardIDs}).
		Where(sq.NotEq{"boards_history.modified_by": "system"}).
		Where(sq.Eq{"boards.delete_at": 0}).
		GroupBy("boards.id, boards_history.id, boards_history.modified_by")

	blocksHistoryQuery := s.getQueryBuilder(db).
		Select("boards.id, boards.icon, boards.title, count(blocks_history.id) as count, blocks_history.modified_by, boards.created_by").
		Prefix("UNION ALL").
		From(s.tablePrefix + "blocks_history as blocks_history").
		Join(s.tablePrefix + "boards as boards on blocks_history.board_id = boards.id").
		Where(sq.Gt{"blocks_history.insert_at": mmModel.GetTimeForMillis(since).Format(time.RFC3339)}).
		Where(sq.Eq{"boards.team_id": teamID}).
		Where(sq.Eq{"boards.id": boardIDs}).
		Where(sq.NotEq{"blocks_history.modified_by": "system"}).
		Where(sq.Eq{"boards.delete_at": 0}).
		GroupBy("boards.id, blocks_history.board_id, blocks_history.modified_by")

	boardsActivity := boardsHistoryQuery.SuffixExpr(blocksHistoryQuery)

	insightsQuery := s.getQueryBuilder(db).Select(
		fmt.Sprintf("id, title, icon, sum(count) as activity_count, %s as active_users, created_by", s.concatenationSelector("distinct modified_by", ",")),
	).
		FromSelect(boardsActivity, "boards_and_blocks_history").
		GroupBy("id, title, icon, created_by").
		OrderBy("activity_count desc").
		Offset(uint64(offset)).
		Limit(uint64(limit))

	rows, err := insightsQuery.Query()
	if err != nil {
		s.logger.Error(`Team insights query ERROR`, mlog.Err(err))
		return nil, err
	}
	defer s.CloseRows(rows)

	boardsInsights, err := boardsInsightsFromRows(rows)
	if err != nil {
		return nil, err
	}
	boardInsightsPaginated := model.GetTopBoardInsightsListWithPagination(boardsInsights, limit)

	return boardInsightsPaginated, nil
}

func (s *SQLStore) getUserBoardsInsights(db sq.BaseRunner, teamID string, userID string, since int64, offset int, limit int, boardIDs []string) (*model.BoardInsightsList, error) {
	boardsHistoryQuery := s.getQueryBuilder(db).
		Select("boards.id, boards.icon, boards.title, count(boards_history.id) as count, boards_history.modified_by, boards.created_by").
		From(s.tablePrefix + "boards_history as boards_history").
		Join(s.tablePrefix + "boards as boards on boards_history.id = boards.id").
		Where(sq.Gt{"boards_history.insert_at": mmModel.GetTimeForMillis(since).Format(time.RFC3339)}).
		Where(sq.Eq{"boards.team_id": teamID}).
		Where(sq.Eq{"boards.id": boardIDs}).
		Where(sq.NotEq{"boards_history.modified_by": "system"}).
		Where(sq.Eq{"boards.delete_at": 0}).
		GroupBy("boards.id, boards_history.id, boards_history.modified_by")

	blocksHistoryQuery := s.getQueryBuilder(db).
		Select("boards.id, boards.icon, boards.title, count(blocks_history.id) as count, blocks_history.modified_by, boards.created_by").
		Prefix("UNION ALL").
		From(s.tablePrefix + "blocks_history as blocks_history").
		Join(s.tablePrefix + "boards as boards on blocks_history.board_id = boards.id").
		Where(sq.Gt{"blocks_history.insert_at": mmModel.GetTimeForMillis(since).Format(time.RFC3339)}).
		Where(sq.Eq{"boards.team_id": teamID}).
		Where(sq.Eq{"boards.id": boardIDs}).
		Where(sq.NotEq{"blocks_history.modified_by": "system"}).
		Where(sq.Eq{"boards.delete_at": 0}).
		GroupBy("boards.id, blocks_history.board_id, blocks_history.modified_by")

	boardsActivity := boardsHistoryQuery.SuffixExpr(blocksHistoryQuery)

	insightsQuery := s.getQueryBuilder(db).Select(
		fmt.Sprintf("id, title, icon, sum(count) as activity_count, %s as active_users, created_by", s.concatenationSelector("distinct modified_by", ",")),
	).
		FromSelect(boardsActivity, "boards_and_blocks_history").
		GroupBy("id, title, icon, created_by").
		OrderBy("activity_count desc")

	userQuery := s.getQueryBuilder(db).Select("*").
		FromSelect(insightsQuery, "boards_and_blocks_history_for_user").
		Where(sq.Or{
			sq.Eq{
				"created_by": userID,
			},
			sq.Expr(s.elementInColumn("active_users"), userID),
		}).
		Offset(uint64(offset)).
		Limit(uint64(limit))

	rows, err := userQuery.Query()

	if err != nil {
		s.logger.Error(`Team insights query ERROR`, mlog.Err(err))
		return nil, err
	}
	defer s.CloseRows(rows)

	boardsInsights, err := boardsInsightsFromRows(rows)
	if err != nil {
		return nil, err
	}
	boardInsightsPaginated := model.GetTopBoardInsightsListWithPagination(boardsInsights, limit)

	return boardInsightsPaginated, nil
}

func boardsInsightsFromRows(rows *sql.Rows) ([]*model.BoardInsight, error) {
	boardsInsights := []*model.BoardInsight{}
	for rows.Next() {
		var boardInsight model.BoardInsight

		err := rows.Scan(
			&boardInsight.BoardID,
			&boardInsight.Title,
			&boardInsight.Icon,
			&boardInsight.ActivityCount,
			&boardInsight.ActiveUsers,
			&boardInsight.CreatedBy,
		)
		if err != nil {
			return nil, err
		}
		boardsInsights = append(boardsInsights, &boardInsight)
	}
	return boardsInsights, nil
}
