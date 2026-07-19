package mux

import "cervterm/internal/pty"

// SessionFactory creates local byte transports without coupling the mux to an OS.
type SessionFactory interface {
	Spawn(rows, cols uint16, options pty.Options) (pty.Session, error)
}

type localSessionFactory struct{}

func (localSessionFactory) Spawn(rows, cols uint16, options pty.Options) (pty.Session, error) {
	return pty.NewLocalWithOptions(rows, cols, options)
}

// LocalSessionFactory returns the production local PTY/ConPTY factory.
func LocalSessionFactory() SessionFactory { return localSessionFactory{} }

// SpawnSpec describes one pane process. Topology and rendering concerns are
// intentionally absent.
type SpawnSpec struct {
	TargetID string
	Options  pty.Options
}
