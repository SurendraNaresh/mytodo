package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/SurendraNaresh/mytodo/internal/model"
)
type taskRow struct {
	check *widget.Check
	name  *widget.Label
	date  *widget.Label
	root  *fyne.Container
}

func newTaskRow() *taskRow {
	r := &taskRow{
		check: widget.NewCheck("", nil),
		name:  widget.NewLabel(""),
		date:  widget.NewLabel(""),
	}

	r.check.Disable()

	r.root = container.NewBorder(
		nil,
		nil,
		r.check,
		r.date,
		r.name,
	)

	return r
}

func (s *AppState) ShowTodo() {
	if err := s.Session.Validate(); err != nil {
		s.ShowLogin()
		return
	}

	user := s.Session.User

	projects, err := model.ProjectsForUser(user.ID)
	if err != nil {
		dialog.ShowError(err, s.Window)
		return
	}

	var selectedProject *model.Project
	var selectedTask *model.Task

	projectName := widget.NewEntry()
	projectName.SetPlaceHolder("New project")

	taskName := widget.NewEntry()
	taskName.SetPlaceHolder("New task")

	dueDate := widget.NewEntry()
	dueDate.SetText(
		time.Now().Format("2006-01-02"),
	)
	dueDate.SetPlaceHolder("YYYY-MM-DD")

	done := widget.NewCheck("Done", nil)

	status := widget.NewLabel("")

	var tasks []model.Task

	taskList := widget.NewList(
		func() int {
			return len(tasks)
		},

		func() fyne.CanvasObject {
			return newTaskRow().root
		},

		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(tasks) {
				return
			}
			// Fyne gives us the root container, so find children
			// without assuming their order.
			row := obj.(*fyne.Container)

			var check *widget.Check
			var labels []*widget.Label

			for _, child := range row.Objects {
				switch v := child.(type) {
				case *widget.Check:
					check = v
				case *widget.Label:
					labels = append(labels, v)
				}
			}

			if check == nil || len(labels) < 2 {
				return
			}

			t := tasks[id]

			check.SetChecked(t.Done)
			labels[0].SetText(t.Name)
			labels[1].SetText(t.DueDate)
		},
	)
/*:-========= [Original way of creating task, check:done, button]
	taskList := widget.NewList(
		func() int {
			return len(tasks)
		},

		func() fyne.CanvasObject {
			check := widget.NewCheck("", nil)

			name := widget.NewLabel("Task")
			date := widget.NewLabel("Date")

			return container.NewBorder(
				nil,
				nil,
				check,
				date,
				name,
			)
		},

		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(tasks) {
				return
			}

			t := tasks[id]

			row := obj.(*fyne.Container)

			check := row.Objects[0].(*widget.Check)
			date := row.Objects[1].(*widget.Label)
			name := row.Objects[2].(*widget.Label)

			check.SetChecked(t.Done)
			check.Disable()

			name.SetText(t.Name)
			date.SetText(t.DueDate)
		},
	)
*/
	clearTask := func() {
		selectedTask = nil

		taskName.SetText("")
		dueDate.SetText(
			time.Now().Format("2006-01-02"),
		)

		done.SetChecked(false)
		taskList.UnselectAll()
	}

	loadTasks := func() {
		tasks = nil

		if selectedProject == nil {
			taskList.Refresh()
			return
		}

		var err error

		tasks, err = model.TasksForParent(
			selectedProject.ID,
			user.ID,
		)

		if err != nil {
			status.SetText(err.Error())
			return
		}

		taskList.Refresh()
	}

	projectList := widget.NewList(
		func() int {
			return len(projects)
		},

		func() fyne.CanvasObject {
			return widget.NewLabel("Project")
		},

		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(projects) {
				return
			}

			p := projects[id]

			obj.(*widget.Label).SetText(
				fmt.Sprintf(
					"%s (%d)",
					p.Name,
					p.ID,
				),
			)
		},
	)

	projectList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(projects) {
			return
		}

		p := projects[id]
		selectedProject = &p

		projectName.SetText(p.Name)

		clearTask()
		loadTasks()

		status.SetText(
			"Project: " + p.Name,
		)
	}

	taskList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(tasks) {
			return
		}

		t := tasks[id]
		selectedTask = &t

		taskName.SetText(t.Name)
		dueDate.SetText(t.DueDate)
		done.SetChecked(t.Done)
	}

	addProject := widget.NewButton(
		"Add",
		func() {
			if projectName.Text == "" {
				status.SetText(
					"Enter project name",
				)
				return
			}

			_, err := model.CreateProject(
				user.ID,
				projectName.Text,
				"",
			)

			if err != nil {
				status.SetText(err.Error())
				return
			}

			s.ShowTodo()
		},
	)

	updateProject := widget.NewButton(
		"Update",
		func() {
			if selectedProject == nil {
				status.SetText(
					"Select project",
				)
				return
			}

			selectedProject.Name =
				projectName.Text

			if err := model.UpdateProject(
				selectedProject,
			); err != nil {

				status.SetText(err.Error())
				return
			}

			s.ShowTodo()
		},
	)

	deleteProject := widget.NewButton(
		"Delete",
		func() {
			if selectedProject == nil {
				status.SetText(
					"Select project",
				)
				return
			}

			p := *selectedProject

			dialog.ShowConfirm(
				"Delete Project",
				"Delete project '"+p.Name+
					"' and all its tasks?",
				func(ok bool) {
					if !ok {
						return
					}

					err := model.DeleteProject(
						p.ID,
						user.ID,
					)

					if err != nil {
						dialog.ShowError(
							err,
							s.Window,
						)
						return
					}

					s.ShowTodo()
				},
				s.Window,
			)
		},
	)

	deleteProject.Importance =
		widget.DangerImportance

	projectEditor := container.NewVBox(
		projectName,

		container.NewHBox(
			addProject,
			updateProject,
			deleteProject,
		),
	)

	projectPanel := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle(
				"Projects",
				fyne.TextAlignLeading,
				fyne.TextStyle{Bold: true},
			),
			projectEditor,
			widget.NewSeparator(),
		),
		nil,
		nil,
		nil,
		projectList,
	)

	addTask := widget.NewButton(
		"Add",
		func() {
			if selectedProject == nil {
				status.SetText(
					"Select a project first",
				)
				return
			}

			if taskName.Text == "" {
				status.SetText(
					"Enter task name",
				)
				return
			}

			if dueDate.Text != "" {
				if _, err := time.Parse(
					"2006-01-02",
					dueDate.Text,
				); err != nil {

					status.SetText(
						"Date must be YYYY-MM-DD",
					)
					return
				}
			}

			_, err := model.CreateTask(
				selectedProject.ID,
				user.ID,
				taskName.Text,
				dueDate.Text,
			)

			if err != nil {
				status.SetText(err.Error())
				return
			}

			taskName.SetText("")
			loadTasks()
		},
	)

	updateTask := widget.NewButton(
		"Update",
		func() {
			if selectedTask == nil {
				status.SetText(
					"Select task",
				)
				return
			}

			selectedTask.Name =
				taskName.Text

			selectedTask.DueDate =
				dueDate.Text

			selectedTask.Done =
				done.Checked

			if selectedProject != nil {
				selectedTask.ParentID =
					selectedProject.ID
			}

			if err := model.UpdateTask(
				selectedTask,
			); err != nil {

				status.SetText(err.Error())
				return
			}

			clearTask()
			loadTasks()
		},
	)

	deleteTask := widget.NewButton(
		"Delete",
		func() {
			if selectedTask == nil {
				status.SetText(
					"Select task",
				)
				return
			}

			id := selectedTask.ID

			if err := model.DeleteTask(
				id,
				user.ID,
			); err != nil {

				status.SetText(err.Error())
				return
			}

			clearTask()
			loadTasks()
		},
	)

	deleteTask.Importance =
		widget.DangerImportance

	newTask := widget.NewButton(
		"New",
		func() {
			clearTask()
		},
	)

	taskForm := widget.NewForm(
		widget.NewFormItem(
			"Task",
			taskName,
		),

		widget.NewFormItem(
			"Due",
			dueDate,
		),

		widget.NewFormItem(
			"Status",
			done,
		),
	)

	taskToolbar := container.NewHBox(
		newTask,
		addTask,
		updateTask,
		deleteTask,
	)

	taskPanel := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle(
				"Tasks",
				fyne.TextAlignLeading,
				fyne.TextStyle{Bold: true},
			),

			taskForm,
			taskToolbar,
			status,

			widget.NewSeparator(),
		),

		nil,
		nil,
		nil,
		taskList,
	)

	split := container.NewHSplit(
		projectPanel,
		taskPanel,
	)

	split.Offset = 0.35

	s.setMain(split)

	if len(projects) > 0 {
		projectList.Select(0)
	}
}
