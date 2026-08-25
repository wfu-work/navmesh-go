package services

import "sync"

var httpRouteReloadHook struct {
	sync.RWMutex
	fn func()
}

func RegisterHTTPRouteReloader(reloader func()) {
	httpRouteReloadHook.Lock()
	httpRouteReloadHook.fn = reloader
	httpRouteReloadHook.Unlock()
}

func triggerHTTPRouteReload() {
	httpRouteReloadHook.RLock()
	reloader := httpRouteReloadHook.fn
	httpRouteReloadHook.RUnlock()
	if reloader != nil {
		reloader()
	}
}
