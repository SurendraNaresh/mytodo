package generated

import (
	"fyne.io/fyne/v2/container"
	module0 "github.com/SurendraNaresh/mytodo/generated/voting"
)

func ModuleTabs() []*container.TabItem {
	return []*container.TabItem{
		container.NewTabItem("Event", module0.NewVotingEventView()),
	}
}
