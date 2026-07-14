package core

func (t *Terminal) Cwd() string { return t.cwd }
func (t *Terminal) CwdSeq() int { return t.cwdSeq }

func (t *Terminal) SetCwd(s string) {
	if s == t.cwd {
		return
	}
	t.cwd = s
	t.cwdSeq++
}
