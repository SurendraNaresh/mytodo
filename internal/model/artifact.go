package model

import (
	"database/sql"
	"fmt"

	"github.com/SurendraNaresh/mytodo/internal/db"
)

type Artifact struct {
	ID          int64  `json:"id"`
	EventID     int64  `json:"event_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	FilePath    string `json:"-"`
	FileName    string `json:"file_name"`
	CreatedAt   string `json:"created_at"`
}

func CreateArtifact(artifact Artifact) (*Artifact, error) {
	result, err := db.DB().Exec(`INSERT INTO artifacts (event_id, title, description, file_path, file_name) VALUES (?, ?, ?, ?, ?)`, artifact.EventID, artifact.Title, artifact.Description, artifact.FilePath, artifact.FileName)
	if err != nil {
		return nil, err
	}
	artifact.ID, err = result.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := db.DB().QueryRow(`SELECT created_at FROM artifacts WHERE id = ?`, artifact.ID).Scan(&artifact.CreatedAt); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func GetArtifact(id int64) (*Artifact, error) {
	var artifact Artifact
	err := db.DB().QueryRow(`SELECT id, event_id, title, description, file_path, file_name, created_at FROM artifacts WHERE id = ?`, id).Scan(
		&artifact.ID, &artifact.EventID, &artifact.Title, &artifact.Description, &artifact.FilePath, &artifact.FileName, &artifact.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get artifact: %w", err)
	}
	return &artifact, nil
}

func ListArtifacts(eventID int64) ([]Artifact, error) {
	query := `SELECT id, event_id, title, description, file_path, file_name, created_at FROM artifacts`
	args := []any{}
	if eventID > 0 {
		query += ` WHERE event_id = ?`
		args = append(args, eventID)
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := db.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	artifacts := make([]Artifact, 0)
	for rows.Next() {
		var artifact Artifact
		if err := rows.Scan(&artifact.ID, &artifact.EventID, &artifact.Title, &artifact.Description, &artifact.FilePath, &artifact.FileName, &artifact.CreatedAt); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

func UpdateArtifact(artifact Artifact) error {
	result, err := db.DB().Exec(`UPDATE artifacts SET event_id = ?, title = ?, description = ?, file_path = ?, file_name = ? WHERE id = ?`, artifact.EventID, artifact.Title, artifact.Description, artifact.FilePath, artifact.FileName, artifact.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func DeleteArtifact(id int64) error {
	result, err := db.DB().Exec(`DELETE FROM artifacts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}
