package layoutstate

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func samplePlan(t *testing.T) Plan {
	t.Helper()
	plan, err := NewPlan(sampleDocument())
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestStorePathResolution(t *testing.T) {
	root := t.TempDir()
	ops := defaultStoreOps()
	ops.userConfigDir = func() (string, error) { return root, nil }
	store, err := newStore(StoreOptions{}, ops)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "cervterm", "layout-v1.json")
	if store.Path() != want {
		t.Fatalf("Path() = %q, want %q", store.Path(), want)
	}

	explicit, err := NewStore(StoreOptions{Path: filepath.Join(root, "..", filepath.Base(root), "state.json")})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(explicit.Path()) || explicit.Path() != filepath.Clean(explicit.Path()) {
		t.Fatalf("explicit path not absolute and clean: %q", explicit.Path())
	}
}

func TestStoreMissingAndRoundTrip(t *testing.T) {
	store, err := NewStore(StoreOptions{Path: filepath.Join(t.TempDir(), "private", "layout.json")})
	if err != nil {
		t.Fatal(err)
	}
	if plan, found, err := store.Load(); err != nil || found || plan.Snapshot().Version != 0 {
		t.Fatalf("missing Load() = (%v, %v, %v)", plan, found, err)
	}
	want := samplePlan(t)
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.Load()
	if err != nil || !found {
		t.Fatalf("Load() found=%v err=%v", found, err)
	}
	wantJSON, _ := Marshal(want)
	gotJSON, _ := Marshal(got)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("round trip mismatch\ngot  %s\nwant %s", gotJSON, wantJSON)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(store.Path())
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestStoreInvalidDocumentsAreTypedAndRedacted(t *testing.T) {
	secret := "SECRET-CONTENT-7e22"
	secretDir := filepath.Join(t.TempDir(), "SECRET-PATH-9a31")
	path := filepath.Join(secretDir, "layout.json")
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(StoreOptions{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		[]byte(`{"version":999,"active_workspace":0,"workspaces":[],"secret":"` + secret + `"}`),
		[]byte(strings.Repeat("x", MaxJSONBytes+1)),
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		plan, found, err := store.Load()
		if !found || !errors.Is(err, ErrInvalidState) || plan.Snapshot().Version != 0 {
			t.Fatalf("invalid Load() = (%v, %v, %v)", plan, found, err)
		}
		message := err.Error()
		if strings.Contains(message, secret) || strings.Contains(message, secretDir) || strings.Contains(message, path) {
			t.Fatalf("error leaked private data: %q", message)
		}
	}
}

func TestStoreReplacementFailurePreservesOldFileAndCleansTemp(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "layout.json")
	old := []byte("old bytes")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	ops := defaultStoreOps()
	ops.replace = func(string, string) error { return errors.New("SECRET replace failure") }
	store, err := newStore(StoreOptions{Path: path}, ops)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Save(samplePlan(t))
	if err == nil || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), path) {
		t.Fatalf("replacement error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(old) {
		t.Fatalf("destination changed: %q", got)
	}
	matches, err := filepath.Glob(filepath.Join(directory, ".layout-v1-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("staging files remain: %v", matches)
	}
}

func TestStoreDurabilityUncertainMeansReplacementCompleted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.json")
	ops := defaultStoreOps()
	ops.syncParent = func(string) error { return errors.New("sync failed") }
	store, err := newStore(StoreOptions{Path: path}, ops)
	if err != nil {
		t.Fatal(err)
	}
	want := samplePlan(t)
	err = store.Save(want)
	if !errors.Is(err, ErrDurabilityUncertain) {
		t.Fatalf("Save() error = %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	wantData, _ := Marshal(want)
	if string(data) != string(wantData) {
		t.Fatal("replacement was not complete before durability error")
	}
}

func TestStoreRejectsUnsafeDestinations(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(StoreOptions{Path: directory})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Load(); !found || !errors.Is(err, ErrUnsafeState) {
		t.Fatalf("directory Load() found=%v err=%v", found, err)
	}

	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "link")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err == nil {
		linkStore, _ := NewStore(StoreOptions{Path: link})
		if _, found, err := linkStore.Load(); !found || !errors.Is(err, ErrUnsafeState) {
			t.Fatalf("symlink Load() found=%v err=%v", found, err)
		}
	}
}

type faultStoreFile struct {
	storeFile
	fail string
}

func (f *faultStoreFile) Chmod(mode os.FileMode) error {
	if f.fail == "chmod" {
		return errors.New("SECRET chmod")
	}
	return f.storeFile.Chmod(mode)
}
func (f *faultStoreFile) Write(data []byte) (int, error) {
	if f.fail == "write" {
		return 0, errors.New("SECRET write")
	}
	if f.fail == "short" {
		return 0, nil
	}
	return f.storeFile.Write(data)
}
func (f *faultStoreFile) Sync() error {
	if f.fail == "sync" {
		return errors.New("SECRET sync")
	}
	return f.storeFile.Sync()
}
func (f *faultStoreFile) Close() error {
	if f.fail == "close" {
		_ = f.storeFile.Close()
		return errors.New("SECRET close")
	}
	return f.storeFile.Close()
}

func TestStorePrecommitFaultsPreserveOldFileAndCleanTemp(t *testing.T) {
	for _, stage := range []string{"mkdir", "create", "chmod", "write", "short", "sync", "close"} {
		t.Run(stage, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "layout.json")
			old := []byte("old")
			if err := os.WriteFile(path, old, 0600); err != nil {
				t.Fatal(err)
			}
			ops := defaultStoreOps()
			if stage == "mkdir" {
				ops.mkdirAll = func(string, os.FileMode) error { return errors.New("SECRET mkdir") }
			}
			originalCreate := ops.createTemp
			ops.createTemp = func(directory, pattern string) (storeFile, string, error) {
				if stage == "create" {
					return nil, "", errors.New("SECRET create")
				}
				file, name, err := originalCreate(directory, pattern)
				if err != nil {
					return nil, "", err
				}
				return &faultStoreFile{storeFile: file, fail: stage}, name, nil
			}
			store, err := newStore(StoreOptions{Path: path}, ops)
			if err != nil {
				t.Fatal(err)
			}
			err = store.Save(samplePlan(t))
			if err == nil || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), directory) {
				t.Fatalf("err=%v", err)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil || string(got) != string(old) {
				t.Fatalf("old file got=%q err=%v", got, readErr)
			}
			matches, _ := filepath.Glob(filepath.Join(directory, ".layout-v1-*"))
			if len(matches) != 0 {
				t.Fatalf("temps=%v", matches)
			}
		})
	}
}

func TestStoreDetectsParentAndStagingReplacement(t *testing.T) {
	t.Run("parent", func(t *testing.T) {
		directory := t.TempDir()
		other := t.TempDir()
		path := filepath.Join(directory, "layout.json")
		ops := defaultStoreOps()
		original := ops.lstat
		calls := 0
		ops.lstat = func(candidate string) (os.FileInfo, error) {
			if candidate == directory {
				calls++
				if calls > 1 {
					return os.Stat(other)
				}
			}
			return original(candidate)
		}
		store, _ := newStore(StoreOptions{Path: path}, ops)
		err := store.Save(samplePlan(t))
		if !errors.Is(err, ErrUnsafeState) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("staging", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "layout.json")
		other := filepath.Join(directory, "other")
		if err := os.WriteFile(other, []byte("other"), 0600); err != nil {
			t.Fatal(err)
		}
		ops := defaultStoreOps()
		original := ops.lstat
		ops.lstat = func(candidate string) (os.FileInfo, error) {
			if strings.Contains(filepath.Base(candidate), ".layout-v1-") {
				return os.Stat(other)
			}
			return original(candidate)
		}
		store, _ := newStore(StoreOptions{Path: path}, ops)
		err := store.Save(samplePlan(t))
		if !errors.Is(err, ErrUnsafeState) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("destination", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "layout.json")
		other := filepath.Join(directory, "other")
		if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(other, []byte("other"), 0600); err != nil {
			t.Fatal(err)
		}
		ops := defaultStoreOps()
		original := ops.lstat
		calls := 0
		ops.lstat = func(candidate string) (os.FileInfo, error) {
			if candidate == path {
				calls++
				if calls > 1 {
					return os.Stat(other)
				}
			}
			return original(candidate)
		}
		store, _ := newStore(StoreOptions{Path: path}, ops)
		err := store.Save(samplePlan(t))
		if !errors.Is(err, ErrUnsafeState) {
			t.Fatalf("err=%v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "old" {
			t.Fatalf("destination=%q", data)
		}
	})
}

func TestStoreRejectsHardLinkedAndSaveUnsafeDestinations(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "layout.json")
	if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, link); err == nil {
		store, _ := NewStore(StoreOptions{Path: link})
		if _, _, err := store.Load(); !errors.Is(err, ErrUnsafeState) {
			t.Fatalf("hardlink load err=%v", err)
		}
		if err := store.Save(samplePlan(t)); !errors.Is(err, ErrUnsafeState) {
			t.Fatalf("hardlink save err=%v", err)
		}
	}
	dirStore, _ := NewStore(StoreOptions{Path: directory})
	if err := dirStore.Save(samplePlan(t)); !errors.Is(err, ErrUnsafeState) {
		t.Fatalf("directory save err=%v", err)
	}
}

func TestStoreSerializesConcurrentSaves(t *testing.T) {
	store, err := NewStore(StoreOptions{Path: filepath.Join(t.TempDir(), "layout.json")})
	if err != nil {
		t.Fatal(err)
	}
	docA, docB := sampleDocument(), sampleDocument()
	docA.Workspaces[0].Windows[0].Title = "A"
	docB.Workspaces[0].Windows[0].Title = "B"
	planA, err := NewPlan(docA)
	if err != nil {
		t.Fatal(err)
	}
	planB, err := NewPlan(docB)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			if index%2 == 0 {
				errs <- store.Save(planA)
			} else {
				errs <- store.Save(planB)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	loaded, found, err := store.Load()
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	title := loaded.Snapshot().Workspaces[0].Windows[0].Title
	if title != "A" && title != "B" {
		t.Fatalf("title=%q", title)
	}
}
