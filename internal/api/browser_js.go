//go:build js

package api

import (
	"fmt"

	"syscall/js"
)

func ConfigureFromBrowser() error {
	window := js.Global().Get("window")
	if window.IsUndefined() || window.IsNull() {
		return fmt.Errorf("browser window is unavailable")
	}
	origin := window.Get("location").Get("origin").String()
	return Configure(origin + "/api/v1")
}
