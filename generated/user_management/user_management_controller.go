package user_management

import (
	"context"
	"database/sql"
)

func CreateUser(ctx context.Context, db *sql.DB, values ...any) (int64, error) {
	result, err := db.ExecContext(ctx, "INSERT INTO user (username, email) VALUES (?, ?)", values...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetUser(ctx context.Context, db *sql.DB, id int64) (User, error) {
	var value User
	err := db.QueryRowContext(ctx, "SELECT id, username, email FROM user WHERE id = ?", id).Scan(&value.ID, &value.Username, &value.Email)
	return value, err
}

func ListUsers(ctx context.Context, db *sql.DB) ([]User, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, username, email FROM user")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []User
	for rows.Next() {
		var value User
		if err := rows.Scan(&value.ID, &value.Username, &value.Email); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func UpdateUser(ctx context.Context, db *sql.DB, id int64, values ...any) error {
	args := append(append([]any(nil), values...), id)
	_, err := db.ExecContext(ctx, "UPDATE user SET username = ?, email = ? WHERE id = ?", args...)
	return err
}

func DeleteUser(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM user WHERE id = ?", id)
	return err
}

func CreateUserdetails(ctx context.Context, db *sql.DB, parentID int64, values ...any) (int64, error) {
	result, err := db.ExecContext(ctx, "INSERT INTO userdetails (user_id, phone1, phone2) VALUES (?, ?, ?)", append([]any{parentID}, values...)...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetUserdetails(ctx context.Context, db *sql.DB, id int64) (Userdetails, error) {
	var value Userdetails
	err := db.QueryRowContext(ctx, "SELECT id, user_id, phone1, phone2 FROM userdetails WHERE id = ?", id).Scan(&value.ID, &value.UserID, &value.Phone1, &value.Phone2)
	return value, err
}

func ListUserdetailsesByUser(ctx context.Context, db *sql.DB, parentID int64) ([]Userdetails, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, user_id, phone1, phone2 FROM userdetails WHERE user_id = ?", parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []Userdetails
	for rows.Next() {
		var value Userdetails
		if err := rows.Scan(&value.ID, &value.UserID, &value.Phone1, &value.Phone2); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func UpdateUserdetails(ctx context.Context, db *sql.DB, id int64, values ...any) error {
	args := append(append([]any(nil), values...), id)
	_, err := db.ExecContext(ctx, "UPDATE userdetails SET phone1 = ?, phone2 = ? WHERE id = ?", args...)
	return err
}

func DeleteUserdetails(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM userdetails WHERE id = ?", id)
	return err
}
