package config

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	lua "github.com/yuin/gopher-lua"
)

const MaxLayoutPersistencePathBytes = 4096

type LayoutPersistenceConfig struct {
	Enabled bool
	Path    string
}

func validateLayoutPersistencePath(path string) error {
	if len(path) > MaxLayoutPersistencePathBytes {
		return fmt.Errorf("layout_persistence.path must be at most %d bytes", MaxLayoutPersistencePathBytes)
	}
	if !utf8.ValidString(path) {
		return errors.New("layout_persistence.path must be valid UTF-8")
	}
	for _, r := range path {
		if r == 0 || unicode.IsControl(r) {
			return errors.New("layout_persistence.path must not contain NUL or control characters")
		}
	}
	return nil
}

func validateDocumentStringField(source, path string, value lua.LValue) error {
	text, ok := value.(lua.LString)
	if !ok {
		return typeError(source, path, KindString, value)
	}
	if path != "layout_persistence.path" {
		return nil
	}
	if err := validateLayoutPersistencePath(string(text)); err != nil {
		return documentError(source, path, "%s", strings.TrimPrefix(err.Error(), "layout_persistence.path "))
	}
	return nil
}
