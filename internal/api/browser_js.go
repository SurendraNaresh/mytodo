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
	location := window.Get("location")
	search := location.Get("search").String()
	if search != "" {
		parameters := js.Global().Get("URLSearchParams").New(search)
		apiURL := parameters.Call("get", "api")
		if !apiURL.IsNull() && apiURL.String() != "" {
			return Configure(apiURL.String())
		}
	}
	return Configure(location.Get("origin").String() + "/api/v1")
}
