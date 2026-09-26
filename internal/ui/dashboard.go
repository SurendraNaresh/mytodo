package ui

import (
	"fmt"
	"os"
	"os/exec"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/SurendraNaresh/mytodo/generated"
)

func (s *AppState) ShowDashboard() {
	if err := s.Session.Validate(); err != nil {
		s.ShowLogin()
		return
	}

	s.Main = container.NewStack()

	user := s.Session.User

	title := widget.NewLabelWithStyle(
		"Todo Master",
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)

	admin := widget.NewButton(
		"Admin / Users",
		func() {
			s.ShowUsers()
		},
	)

	var generateUI *widget.Button
	generateUI = widget.NewButton("Generate UI", func() {
		if !s.Session.IsAdmin() {
			dialog.ShowError(fmt.Errorf("administrator access required"), s.Window)
			return
		}

		dialog.ShowConfirm("Generate UI", "Run blueprintgen from the current project directory? Generated source files may be replaced. The running application will need to be rebuilt and restarted to load them.", func(confirmed bool) {
			if !confirmed {
				return
			}

			root, err := os.Getwd()
			if err != nil {
				dialog.ShowError(err, s.Window)
				return
			}
			if _, err := os.Stat("tools/blueprintgen/main.go"); err != nil {
				dialog.ShowError(fmt.Errorf("run the application from the project root: %w", err), s.Window)
				return
			}

			generateUI.Disable()
			go func() {
				command := exec.Command("go", "run", "./tools/blueprintgen", "-blueprint", "./blueprint.json", "-out", "./generated")
				command.Dir = root
				output, err := command.CombinedOutput()
				fyne.Do(func() {
					generateUI.Enable()
					if err != nil {
						dialog.ShowError(fmt.Errorf("blueprintgen failed: %w\n%s", err, output), s.Window)
						return
					}
					dialog.ShowInformation("UI source generated", "Review generated files and SQL, then run `go test ./...` and rebuild/restart the application. Database migrations are not applied automatically.", s.Window)
				})
			}()
		}, s.Window)
	})

	if !s.Session.IsAdmin() {
		admin.Disable()
		generateUI.Disable()
	}

	profile := widget.NewButton(
		"User / Profile",
		func() {
			s.ShowCurrentUser()
		},
	)

	todo := widget.NewButton(
		fmt.Sprintf("Todo: %s", user.Name),
		func() {
			s.ShowTodo()
		},
	)

	voting := widget.NewButton(
		"Voting events",
		func() {
			s.ShowVoting()
		},
	)

	logout := widget.NewButton(
		"Logout",
		func() {
			s.Session.Logout()
			s.ShowLogin()
		},
	)

	sidebar := container.NewVBox(
		title,
		widget.NewSeparator(),

		widget.NewLabel(
			fmt.Sprintf(
				"%s\n[%s]",
				user.Name,
				user.Role,
			),
		),

		widget.NewSeparator(),

		admin,
		generateUI,
		profile,
		todo,
		voting,

		widget.NewSeparator(),
		logout,
	)

	sidebar.Hide()
	body := container.NewStack(s.Main)
	updateBody := func() {
		if sidebar.Visible() {
			body.Objects = []fyne.CanvasObject{container.NewBorder(nil, nil, sidebar, nil, s.Main)}
		} else {
			body.Objects = []fyne.CanvasObject{s.Main}
		}
		body.Refresh()
	}

	menu := widget.NewButtonWithIcon("", theme.MenuIcon(), func() {
		if sidebar.Visible() {
			sidebar.Hide()
		} else {
			sidebar.Show()
		}
		updateBody()
	})
	menu.Importance = widget.LowImportance

	root := container.NewBorder(
		container.NewHBox(menu),
		nil,
		nil,
		nil,
		body,
	)

	tabs := container.NewAppTabs(container.NewTabItem("mytodo-", root))
	for _, tab := range generated.ModuleTabs() {
		tabs.Append(tab)
	}
	s.setContent(tabs)

	s.ShowTodo()
}
