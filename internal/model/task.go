package model

import (
	"errors"
	"strings"

	"github.com/SurendraNaresh/mytodo/internal/db"
)

// Parent/master record.
type Project struct {
	ID          int64
	OwnerID     int64
	Name        string
	Description string
}

// Child/detail record.
type Task struct {
	ID       int64
	ParentID int64
	UserID   int64

	Name    string
	DueDate string
	Done    bool
}

// -------------------------------------------------------
// Parent CRUD
// -------------------------------------------------------

func CreateProject(
	ownerID int64,
	name string,
	description string,
) (*Project, error) {

	name = strings.TrimSpace(name)

	if ownerID == 0 {
		return nil, errors.New("owner required")
	}

	if name == "" {
		return nil, errors.New("project name required")
	}

	res, err := db.DB().Exec(`
		INSERT INTO projects
		    (owner_id, name, description)
		VALUES (?, ?, ?)
	`,
		ownerID,
		name,
		description,
	)

	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetProject(id)
}

func GetProject(id int64) (*Project, error) {
	var p Project

	err := db.DB().QueryRow(`
		SELECT
		    id,
		    owner_id,
		    name,
		    description
		  FROM projects
		 WHERE id = ?
	`, id).Scan(
		&p.ID,
		&p.OwnerID,
		&p.Name,
		&p.Description,
	)

	if err != nil {
		return nil, err
	}

	return &p, nil
}

func ProjectsForUser(userID int64) ([]Project, error) {
	rows, err := db.DB().Query(`
		SELECT
		    id,
		    owner_id,
		    name,
		    description
		  FROM projects
		 WHERE owner_id = ?
		 ORDER BY name
	`, userID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project

	for rows.Next() {
		var p Project

		if err := rows.Scan(
			&p.ID,
			&p.OwnerID,
			&p.Name,
			&p.Description,
		); err != nil {
			return nil, err
		}

		projects = append(projects, p)
	}

	return projects, rows.Err()
}

func UpdateProject(p *Project) error {
	if p == nil {
		return errors.New("nil project")
	}

	p.Name = strings.TrimSpace(p.Name)

	if p.Name == "" {
		return errors.New("project name required")
	}

	_, err := db.DB().Exec(`
		UPDATE projects
		   SET name        = ?,
		       description = ?
		 WHERE id = ?
		   AND owner_id = ?
	`,
		p.Name,
		p.Description,
		p.ID,
		p.OwnerID,
	)

	return err
}

func DeleteProject(id, ownerID int64) error {
	// Child tasks disappear automatically because of ON DELETE CASCADE.
	_, err := db.DB().Exec(`
		DELETE FROM projects
		 WHERE id = ?
		   AND owner_id = ?
	`,
		id,
		ownerID,
	)

	return err
}

// -------------------------------------------------------
// Child CRUD
// -------------------------------------------------------

func CreateTask(
	parentID int64,
	userID int64,
	name string,
	dueDate string,
) (*Task, error) {

	name = strings.TrimSpace(name)

	if parentID == 0 {
		return nil, errors.New("parent/project required")
	}

	if userID == 0 {
		return nil, errors.New("user required")
	}

	if name == "" {
		return nil, errors.New("task name required")
	}

	res, err := db.DB().Exec(`
		INSERT INTO tasks
		    (parent_id, user_id, name, due_date, done)
		VALUES (?, ?, ?, ?, 0)
	`,
		parentID,
		userID,
		name,
		dueDate,
	)

	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetTask(id)
}

func GetTask(id int64) (*Task, error) {
	var t Task
	var done int

	err := db.DB().QueryRow(`
		SELECT
		    id,
		    parent_id,
		    user_id,
		    name,
		    due_date,
		    done
		  FROM tasks
		 WHERE id = ?
	`, id).Scan(
		&t.ID,
		&t.ParentID,
		&t.UserID,
		&t.Name,
		&t.DueDate,
		&done,
	)

	if err != nil {
		return nil, err
	}

	t.Done = done != 0

	return &t, nil
}

func TasksForParent(parentID, userID int64) ([]Task, error) {
	rows, err := db.DB().Query(`
		SELECT
		    id,
		    parent_id,
		    user_id,
		    name,
		    due_date,
		    done
		  FROM tasks
		 WHERE parent_id = ?
		   AND user_id = ?
		 ORDER BY
		    done,
		    due_date,
		    id
	`,
		parentID,
		userID,
	)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task

	for rows.Next() {
		var t Task
		var done int

		if err := rows.Scan(
			&t.ID,
			&t.ParentID,
			&t.UserID,
			&t.Name,
			&t.DueDate,
			&done,
		); err != nil {
			return nil, err
		}

		t.Done = done != 0
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func UpdateTask(t *Task) error {
	if t == nil {
		return errors.New("nil task")
	}

	t.Name = strings.TrimSpace(t.Name)

	if t.Name == "" {
		return errors.New("task name required")
	}

	done := 0
	if t.Done {
		done = 1
	}

	_, err := db.DB().Exec(`
		UPDATE tasks
		   SET parent_id = ?,
		       name      = ?,
		       due_date  = ?,
		       done      = ?
		 WHERE id = ?
		   AND user_id = ?
	`,
		t.ParentID,
		t.Name,
		t.DueDate,
		done,
		t.ID,
		t.UserID,
	)

	return err
}

func DeleteTask(id, userID int64) error {
	_, err := db.DB().Exec(`
		DELETE FROM tasks
		 WHERE id = ?
		   AND user_id = ?
	`,
		id,
		userID,
	)

	return err
}
