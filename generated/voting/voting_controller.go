package voting

import (
	"context"
	"database/sql"
)

// CreateVote uses fixed SQL and parameter placeholders.
func CreateVote(ctx context.Context, db *sql.DB, parentID int64, values ...any) (int64, error) {
	result, err := db.ExecContext(ctx, "INSERT INTO vote (voting_event_id, voter_user_id, choice) VALUES (?, ?, ?)", append([]any{parentID}, values...)...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
