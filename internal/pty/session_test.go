package pty

import "testing"

func TestLocalSessionImplementsSessionContract(t *testing.T) {
	var _ Session = (*localSession)(nil)
}
