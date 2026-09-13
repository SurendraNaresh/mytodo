package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

type Blueprint struct {
	Types []BlueprintType `json:"types"`
}

type BlueprintType struct {
	TypeKey      string            `json:"type_key"`
	TypeID       int64             `json:"type_id"`
	ParentTypeID *int64            `json:"parent_type_id"`
	Header       map[string]string `json:"header"`
	LogicRule    map[string]string `json:"logic_rule"`
	UI           BlueprintUI       `json:"ui"`
}

type BlueprintUI struct {
	ListCols  []string `json:"list_cols"`
	FormOrder []string `json:"form_order"`
}

type Store struct {
	db         *sql.DB
	mu         sync.RWMutex
	blueprints map[string]BlueprintType
}

func NewStore(database *sql.DB, blueprintPath string) (*Store, error) {
	content, err := os.ReadFile(blueprintPath)
	if err != nil {
		return nil, fmt.Errorf("read blueprint: %w", err)
	}
	var blueprint Blueprint
	if err := json.Unmarshal(content, &blueprint); err != nil {
		return nil, fmt.Errorf("parse blueprint: %w", err)
	}
	store := &Store{db: database, blueprints: make(map[string]BlueprintType)}
	for _, definition := range blueprint.Types {
		if definition.TypeKey == "" || definition.TypeID == 0 {
			return nil, errors.New("blueprint types require type_key and type_id")
		}
		store.blueprints[definition.TypeKey] = definition
		_, err := database.Exec(`INSERT INTO object_types(type_id, type_key, parent_type_id)
			VALUES (?, ?, ?) ON CONFLICT(type_id) DO UPDATE SET type_key=excluded.type_key,
			parent_type_id=excluded.parent_type_id`, definition.TypeID, definition.TypeKey, definition.ParentTypeID)
		if err != nil {
			return nil, fmt.Errorf("register blueprint type %s: %w", definition.TypeKey, err)
		}
	}
	return store, nil
}

func (s *Store) Create(ctx context.Context, userID int64, typeKey string, parentID *int64, payload map[string]any) (int64, error) {
	s.mu.RLock()
	definition, ok := s.blueprints[typeKey]
	s.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("unknown blueprint type %q", typeKey)
	}
	if definition.ParentTypeID == nil && parentID != nil {
		return 0, errors.New("root object cannot have a parent")
	}
	if definition.ParentTypeID != nil && parentID == nil {
		return 0, errors.New("child object requires a parent")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("encode payload: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if parentID != nil {
		var parentType int64
		var ownerID int64
		err = tx.QueryRowContext(ctx, `SELECT type_id, owner_user_id FROM objects WHERE obj_id = ?`, *parentID).Scan(&parentType, &ownerID)
		if err != nil {
			return 0, fmt.Errorf("load parent: %w", err)
		}
		if parentType != *definition.ParentTypeID {
			return 0, errors.New("parent type does not match blueprint")
		}
		if ownerID != userID {
			return 0, errors.New("forbidden: parent is owned by another user")
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO objects(type_id, parent_obj_id, owner_user_id, data) VALUES (?, ?, ?, ?)`, definition.TypeID, parentID, userID, string(data))
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", typeKey, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
