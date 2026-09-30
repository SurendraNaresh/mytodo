package voting

import (
	"context"
	"database/sql"
)

func CreateVotingEvent(ctx context.Context, db *sql.DB, values ...any) (int64, error) {
	result, err := db.ExecContext(ctx, "INSERT INTO voting_event (title, description, event_type, opens_at, closes_at) VALUES (?, ?, ?, ?, ?)", values...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetVotingEvent(ctx context.Context, db *sql.DB, id int64) (VotingEvent, error) {
	var value VotingEvent
	err := db.QueryRowContext(ctx, "SELECT id, title, description, event_type, opens_at, closes_at FROM voting_event WHERE id = ?", id).Scan(&value.ID, &value.Title, &value.Description, &value.EventType, &value.OpensAt, &value.ClosesAt)
	return value, err
}

func ListVotingEvents(ctx context.Context, db *sql.DB) ([]VotingEvent, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, title, description, event_type, opens_at, closes_at FROM voting_event")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []VotingEvent
	for rows.Next() {
		var value VotingEvent
		if err := rows.Scan(&value.ID, &value.Title, &value.Description, &value.EventType, &value.OpensAt, &value.ClosesAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func UpdateVotingEvent(ctx context.Context, db *sql.DB, id int64, values ...any) error {
	args := append(append([]any(nil), values...), id)
	_, err := db.ExecContext(ctx, "UPDATE voting_event SET title = ?, description = ?, event_type = ?, opens_at = ?, closes_at = ? WHERE id = ?", args...)
	return err
}

func DeleteVotingEvent(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM voting_event WHERE id = ?", id)
	return err
}

func CreateVote(ctx context.Context, db *sql.DB, parentID int64, values ...any) (int64, error) {
	result, err := db.ExecContext(ctx, "INSERT INTO vote (voting_event_id, voter_user_id, choice, comments) VALUES (?, ?, ?, ?)", append([]any{parentID}, values...)...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetVote(ctx context.Context, db *sql.DB, id int64) (Vote, error) {
	var value Vote
	err := db.QueryRowContext(ctx, "SELECT id, voting_event_id, voter_user_id, choice, comments FROM vote WHERE id = ?", id).Scan(&value.ID, &value.VotingEventID, &value.VoterUserId, &value.Choice, &value.Comments)
	return value, err
}

func ListVotesByVotingEvent(ctx context.Context, db *sql.DB, parentID int64) ([]Vote, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, voting_event_id, voter_user_id, choice, comments FROM vote WHERE voting_event_id = ?", parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []Vote
	for rows.Next() {
		var value Vote
		if err := rows.Scan(&value.ID, &value.VotingEventID, &value.VoterUserId, &value.Choice, &value.Comments); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func UpdateVote(ctx context.Context, db *sql.DB, id int64, values ...any) error {
	args := append(append([]any(nil), values...), id)
	_, err := db.ExecContext(ctx, "UPDATE vote SET voter_user_id = ?, choice = ?, comments = ? WHERE id = ?", args...)
	return err
}

func DeleteVote(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM vote WHERE id = ?", id)
	return err
}
