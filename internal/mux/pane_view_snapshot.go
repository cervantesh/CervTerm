package mux

import "cervterm/internal/render"

func detachedPaneSnapshot(source render.Snapshot) render.Snapshot {
	return render.DetachedSnapshot(source)
}
