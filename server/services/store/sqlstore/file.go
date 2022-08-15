package sqlstore

import (
	"database/sql"
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

func (s *SQLStore) saveFileInfo(db sq.BaseRunner, fileInfo *model.FileInfo) error {
	query := s.getQueryBuilder(db).
		Insert(s.tablePrefix+"file_info").
		Columns(
			"id",
			"create_at",
			"name",
			"extension",
			"size",
			"delete_at",
			"archived",
		).
		Values(
			fileInfo.Id,
			fileInfo.CreateAt,
			fileInfo.Name,
			fileInfo.Extension,
			fileInfo.Size,
			fileInfo.DeleteAt,
			false,
		)

	if _, err := query.Exec(); err != nil {
		s.logger.Error(
			"failed to save fileinfo",
			mlog.String("file_name", fileInfo.Name),
			mlog.Int64("size", fileInfo.Size),
			mlog.Err(err),
		)
		return err
	}

	return nil
}

func (s *SQLStore) getFileInfo(db sq.BaseRunner, id string) (*model.FileInfo, error) {
	query := s.getQueryBuilder(db).
		Select(
			"id",
			"create_at",
			"delete_at",
			"name",
			"extension",
			"size",
			"archived",
		).
		From(s.tablePrefix + "file_info").
		Where(sq.Eq{"Id": id})

	row := query.QueryRow()

	fileInfo := model.FileInfo{}

	err := row.Scan(
		&fileInfo.Id,
		&fileInfo.CreateAt,
		&fileInfo.DeleteAt,
		&fileInfo.Name,
		&fileInfo.Extension,
		&fileInfo.Size,
		&fileInfo.Archived,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		s.logger.Error("error scanning fileinfo row", mlog.String("id", id), mlog.Err(err))
		return nil, err
	}

	return &fileInfo, nil
}
