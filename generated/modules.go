package generated

import (
	"fyne.io/fyne/v2/container"
	module1 "github.com/SurendraNaresh/mytodo/generated/user_management"
	module0 "github.com/SurendraNaresh/mytodo/generated/voting"
)

func ModuleTabs() []*container.TabItem {
	return []*container.TabItem{
		container.NewTabItem("vote", module0.NewVotingEventView()),
		container.NewTabItem("userdetails", module1.NewUserView()),
	}
}
