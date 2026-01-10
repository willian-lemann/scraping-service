package database

import (
	"context"
	"fiber/structs"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func UpdateListing(conn *pgxpool.Pool, ctx context.Context, id string, data structs.Listing) error {
	query := `
		UPDATE listings SET
			content = $1,
			photos = $2,
			updated_at = NOW()
		WHERE ref = $3
	`

	res, err := conn.Exec(ctx, query,
		data.Content,
		data.Photos,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to update listing: %w", err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("no listing found with ref %s", data.Ref)
	}

	UpdateScrappedInfo(conn, ctx, structs.ScrappedInfo{LinksFailed: []string{""}})

	return nil
}
