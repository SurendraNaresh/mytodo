package main

import (
	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/clientdata"
	appconfig "github.com/SurendraNaresh/mytodo/internal/config"
	"github.com/SurendraNaresh/mytodo/internal/model"
	"github.com/SurendraNaresh/mytodo/internal/ui"
)

func main() {
	config, err := appconfig.Load()
	if err != nil {
		ui.ShowStartupError(err)
		return
	}
	if err := api.Configure(config.APIURL); err != nil {
		ui.ShowStartupError(err)
		return
	}
	model.UseRemoteAPI(true)

	application := ui.NewApp()
	localDataPath, err := clientdata.ResolvePath(config.LocalDataFile, application.App.Storage().RootURI().Path())
	if err != nil {
		application.ShowStartupError(err)
		application.Window.ShowAndRun()
		return
	}
	if err := clientdata.Open(localDataPath); err != nil {
		application.ShowStartupError(err)
		application.Window.ShowAndRun()
		return
	}
	defer clientdata.Close()
	if savedAPIURL, err := clientdata.LoadSetting("api_url"); err != nil {
		application.ShowStartupError(err)
		application.Window.ShowAndRun()
		return
	} else if savedAPIURL != "" {
		if err := api.Configure(savedAPIURL); err != nil {
			application.ShowStartupError(err)
			application.Window.ShowAndRun()
			return
		}
	}
	if err := clientdata.SaveSetting("api_url", api.CurrentURL()); err != nil {
		application.ShowStartupError(err)
		application.Window.ShowAndRun()
		return
	}
	if err := clientdata.SaveTheme("app", `{"variant":"light-green"}`); err != nil {
		application.ShowStartupError(err)
		application.Window.ShowAndRun()
		return
	}
	application.ShowLogin()
	application.Window.ShowAndRun()
}
