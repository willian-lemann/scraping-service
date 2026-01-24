package database

import (
	"context"
	"encoding/json"
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

	photosJSON, err := json.Marshal(data.Photos)
	if err != nil {
		return fmt.Errorf("failed to marshal photos: %w", err)
	}

	_, err = conn.Exec(ctx, query,
		data.Content,
		photosJSON,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to update listing: %w", err)
	}

	UpdateScrappedInfo(conn, ctx, structs.ScrappedInfo{LinksFailed: []string{""}})

	return nil
}
