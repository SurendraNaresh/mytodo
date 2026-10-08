//go:build js

package clientdata

import (
	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/model"
)

func Open(string) error                  { return nil }
func Close() error                       { return nil }
func SaveUser(*model.User) error         { return nil }
func SaveSetting(string, string) error   { return nil }
func LoadSetting(string) (string, error) { return "", nil }
func SaveTheme(string, string) error     { return nil }
func SaveEvents([]api.Event) error       { return nil }
