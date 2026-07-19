package layoutstate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrInvalidState        = errors.New("layout state is invalid")
	ErrUnsafeState         = errors.New("layout state is unsafe")
	ErrDurabilityUncertain = errors.New("layout state durability is uncertain")
)

// StoreOptions selects a current-user-controlled state location. On Windows the
// caller is responsible for choosing a parent protected by the current user's ACL;
// the store still rejects reparse destinations and revalidates object identity.
type StoreOptions struct {
	Path string
}

type storeFile interface {
	io.Reader
	io.Writer
	Stat() (os.FileInfo, error)
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

type storeOps struct {
	userConfigDir func() (string, error)
	abs           func(string) (string, error)
	lstat         func(string) (os.FileInfo, error)
	open          func(string) (storeFile, error)
	mkdirAll      func(string, os.FileMode) error
	createTemp    func(string, string) (storeFile, string, error)
	remove        func(string) error
	replace       func(string, string) error
	syncParent    func(string) error
	hasManyLinks  func(string, os.FileInfo) (bool, error)
}

func defaultStoreOps() storeOps {
	return storeOps{
		userConfigDir: os.UserConfigDir,
		abs:           filepath.Abs,
		lstat:         os.Lstat,
		open:          openLayoutFile,
		mkdirAll:      os.MkdirAll,
		createTemp: func(directory, pattern string) (storeFile, string, error) {
			file, err := os.CreateTemp(directory, pattern)
			if err != nil {
				return nil, "", err
			}
			return file, file.Name(), nil
		},
		remove:       os.Remove,
		replace:      atomicReplaceFile,
		syncParent:   syncParentDirectory,
		hasManyLinks: fileHasMultipleLinks,
	}
}

type Store struct {
	path string
	ops  storeOps
	mu   sync.Mutex
}

func NewStore(options StoreOptions) (*Store, error) {
	return newStore(options, defaultStoreOps())
}

func newStore(options StoreOptions, ops storeOps) (*Store, error) {
	path := options.Path
	if path == "" {
		root, err := ops.userConfigDir()
		if err != nil {
			return nil, errors.New("layout state: resolve user configuration directory")
		}
		path = filepath.Join(root, "cervterm", "layout-v1.json")
	}
	absolute, err := ops.abs(path)
	if err != nil {
		return nil, errors.New("layout state: resolve path")
	}
	return &Store{path: filepath.Clean(absolute), ops: ops}, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Load() (Plan, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	before, err := s.ops.lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Plan{}, false, nil
	}
	if err != nil {
		return Plan{}, false, errors.New("layout state: inspect failed")
	}
	directory := filepath.Dir(s.path)
	parentBefore, err := s.ops.lstat(directory)
	if err != nil || !directoryModeSafe(parentBefore) {
		return Plan{}, true, fmt.Errorf("%w: parent directory is unsafe", ErrUnsafeState)
	}
	if err := s.validateDestination(before); err != nil {
		return Plan{}, true, err
	}

	file, err := s.ops.open(s.path)
	if err != nil {
		return Plan{}, true, errors.New("layout state: open failed")
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil {
		return Plan{}, true, errors.New("layout state: inspect opened file failed")
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return Plan{}, true, fmt.Errorf("%w: destination changed or is not regular", ErrUnsafeState)
	}
	parentAfter, err := s.ops.lstat(directory)
	if err != nil || !directoryModeSafe(parentAfter) || !os.SameFile(parentBefore, parentAfter) {
		return Plan{}, true, fmt.Errorf("%w: parent directory changed", ErrUnsafeState)
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(MaxJSONBytes)+1))
	if err != nil {
		return Plan{}, true, errors.New("layout state: read failed")
	}
	if len(data) > MaxJSONBytes {
		return Plan{}, true, fmt.Errorf("%w: document exceeds size limit", ErrInvalidState)
	}
	plan, err := Unmarshal(data)
	if err != nil {
		return Plan{}, true, fmt.Errorf("%w: document failed structural validation", ErrInvalidState)
	}
	return plan, true, nil
}

func (s *Store) Save(plan Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := Marshal(plan)
	if err != nil {
		return fmt.Errorf("%w: document failed structural validation", ErrInvalidState)
	}
	directory := filepath.Dir(s.path)
	if err := s.ops.mkdirAll(directory, 0o700); err != nil {
		return errors.New("layout state: create parent failed")
	}
	parentBefore, err := s.ops.lstat(directory)
	if err != nil || !directoryModeSafe(parentBefore) {
		return fmt.Errorf("%w: parent directory is unsafe", ErrUnsafeState)
	}
	destinationBefore, destinationExisted, err := s.destinationSnapshot()
	if err != nil {
		return err
	}

	file, tempPath, err := s.ops.createTemp(directory, ".layout-v1-*")
	if err != nil {
		return errors.New("layout state: create staging file failed")
	}
	committed := false
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		if !committed {
			_ = s.ops.remove(tempPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return errors.New("layout state: secure staging file failed")
	}
	stagedOriginal, err := file.Stat()
	if err != nil || !stagedOriginal.Mode().IsRegular() {
		return fmt.Errorf("%w: staging file is unsafe", ErrUnsafeState)
	}
	if err := writeAll(file, data); err != nil {
		return errors.New("layout state: write staging file failed")
	}
	if err := file.Sync(); err != nil {
		return errors.New("layout state: flush staging file failed")
	}
	if err := file.Close(); err != nil {
		closed = true
		return errors.New("layout state: close staging file failed")
	}
	closed = true
	if err := s.verifyDestination(destinationBefore, destinationExisted); err != nil {
		return err
	}
	if err := s.validateStaging(tempPath, stagedOriginal); err != nil {
		return err
	}
	parentAfter, err := s.ops.lstat(directory)
	if err != nil || !directoryModeSafe(parentAfter) || !os.SameFile(parentBefore, parentAfter) {
		return fmt.Errorf("%w: parent directory changed", ErrUnsafeState)
	}
	if err := s.ops.replace(tempPath, s.path); err != nil {
		return errors.New("layout state: atomic replacement failed")
	}
	committed = true
	if err := s.ops.syncParent(s.path); err != nil {
		return fmt.Errorf("%w: parent directory flush failed", ErrDurabilityUncertain)
	}
	return nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func (s *Store) destinationSnapshot() (os.FileInfo, bool, error) {
	info, err := s.ops.lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.New("layout state: inspect destination failed")
	}
	if err := s.validateDestination(info); err != nil {
		return nil, true, err
	}
	return info, true, nil
}

func (s *Store) verifyDestination(previous os.FileInfo, existed bool) error {
	current, currentExists, err := s.destinationSnapshot()
	if err != nil {
		return err
	}
	if currentExists != existed || (existed && !os.SameFile(previous, current)) {
		return fmt.Errorf("%w: destination changed", ErrUnsafeState)
	}
	return nil
}

func (s *Store) validateDestination(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: destination is not a regular file", ErrUnsafeState)
	}
	many, err := s.ops.hasManyLinks(s.path, info)
	if err != nil {
		return fmt.Errorf("%w: destination link metadata unavailable", ErrUnsafeState)
	}
	if many {
		return fmt.Errorf("%w: destination has multiple links", ErrUnsafeState)
	}
	return nil
}

func (s *Store) validateStaging(path string, original os.FileInfo) error {
	current, err := s.ops.lstat(path)
	if err != nil || !current.Mode().IsRegular() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(original, current) {
		return fmt.Errorf("%w: staging file changed", ErrUnsafeState)
	}
	many, err := s.ops.hasManyLinks(path, current)
	if err != nil || many {
		return fmt.Errorf("%w: staging link metadata is unsafe", ErrUnsafeState)
	}
	return nil
}
