package database

import (
	"context"
	"fiber/structs"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func UpdateScrappedInfo(conn *pgxpool.Pool, ctx context.Context, data structs.ScrappedInfo) error {
	var id int
	querySelect := `
		SELECT id FROM scrapped_infos
		ORDER BY created_at DESC
		LIMIT 1
	`

	err := conn.QueryRow(ctx, querySelect).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to get last scrapped info: %w", err)
	}

	query := `
		UPDATE scrapped_infos SET
			links_failed = $1, 
			updated_at = NOW()
		WHERE id = $2
	`

	res, err := conn.Exec(ctx, query,
		data.LinksFailed,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to update scrapped info: %w", err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("no scrapped info found with ref %d", id)
	}

	return nil
}
