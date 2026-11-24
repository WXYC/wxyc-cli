package auth

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestFileStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := FileStore{Dir: dir}

	if _, err := s.Load("dj"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("Load on empty = %v, want ErrNoSession", err)
	}
	if err := s.Save("dj", "sess-token"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("dj")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sess-token" {
		t.Errorf("Load = %q, want sess-token", got)
	}
	if err := s.Clear("dj"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("dj"); !errors.Is(err, ErrNoSession) {
		t.Errorf("Load after Clear = %v, want ErrNoSession", err)
	}
}

func TestFileStore_FileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	s := FileStore{Dir: dir}
	if err := s.Save("dj", "secret"); err != nil {
		t.Fatal(err)
	}
	var info fs.FileInfo
	err := filepath.Walk(dir, func(p string, fi fs.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			info = fi
		}
		return err
	})
	if err != nil || info == nil {
		t.Fatalf("could not stat token file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file perms = %o, want 600", perm)
	}
}

func TestFileStore_ClearMissingIsNoError(t *testing.T) {
	s := FileStore{Dir: t.TempDir()}
	if err := s.Clear("never-saved"); err != nil {
		t.Errorf("Clear on missing = %v, want nil", err)
	}
}

func TestKeyringStore_RoundTrip(t *testing.T) {
	keyring.MockInit()
	s := KeyringStore{}

	if _, err := s.Load("dj"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("Load on empty = %v, want ErrNoSession", err)
	}
	if err := s.Save("dj", "kr-token"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("dj")
	if err != nil {
		t.Fatal(err)
	}
	if got != "kr-token" {
		t.Errorf("Load = %q, want kr-token", got)
	}
	if err := s.Clear("dj"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("dj"); !errors.Is(err, ErrNoSession) {
		t.Errorf("Load after Clear = %v, want ErrNoSession", err)
	}
}

// Ensure the file fallback lands somewhere real when Dir is defaulted.
func TestDefaultFileStoreDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := defaultStoreDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "wxyc-cli" {
		t.Errorf("dir = %q, want .../wxyc-cli", dir)
	}
}

// A profile name is a file name inside Dir, never a path: one that could
// escape Dir must be refused by every operation, and nothing written.
func TestFileStore_RejectsPathProfiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "store")
	s := FileStore{Dir: dir}
	for _, p := range []string{"../x", "../../x", "a/b", "..", ".", "", `a\b`,
		// Names Windows would read as a drive, a stream or a device.
		"C:x", "dj:stream", "CON", "nul", "Com1", "lpt9.x", "aux.stage",
		// Anything else outside [A-Za-z0-9._-].
		"a b", "a*b", "a?b", "a|b", `a"b`, "a<b", "a>b", "a\tb", "é"} {
		want := fmt.Sprintf("invalid profile name %q", p)
		if err := s.Save(p, "tok"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Save(%q) = %v, want %q", p, err, want)
		}
		if _, err := s.Load(p); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Load(%q) = %v, want %q", p, err, want)
		}
		if err := s.Clear(p); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Clear(%q) = %v, want %q", p, err, want)
		}
		// Wherever the name would have put its token, nothing is there.
		if _, err := os.Lstat(filepath.Join(dir, p+".token")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Save(%q) left a file at %s (%v)", p, filepath.Join(dir, p+".token"), err)
		}
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("store dir created for rejected profiles (%v)", err)
	}
}

// Dots inside a name are fine: with no separator the name can't leave Dir,
// and refusing it would strand a session already saved under it.
func TestFileStore_AllowsDotsInProfile(t *testing.T) {
	s := FileStore{Dir: t.TempDir()}
	for _, p := range []string{"x..y", "stage..2", "...", ".hidden"} {
		if err := s.Save(p, "tok-"+p); err != nil {
			t.Fatalf("Save(%q) = %v", p, err)
		}
		if got, err := s.Load(p); err != nil || got != "tok-"+p {
			t.Errorf("Load(%q) = %q, %v", p, got, err)
		}
		if err := s.Clear(p); err != nil {
			t.Errorf("Clear(%q) = %v", p, err)
		}
	}
}

// Names made of letters, digits, dots, dashes and underscores are kept,
// including ones that merely start like a Windows device name.
func TestFileStore_AllowsSafeProfiles(t *testing.T) {
	s := FileStore{Dir: t.TempDir()}
	for _, p := range []string{"default", "night_shift", "stage-2", "CONSOLE", "nully", "com10", "Lpt"} {
		if err := s.CheckProfile(p); err != nil {
			t.Errorf("CheckProfile(%q) = %v, want nil", p, err)
		}
		if err := s.Save(p, "tok"); err != nil {
			t.Errorf("Save(%q) = %v", p, err)
		}
	}
}

// The keychain keys a session by an opaque account name, so a slash is
// harmless there: a session saved under "team/a" before file-name rules
// existed must stay usable. Only an empty name is refused.
func TestKeyringStore_CheckProfile(t *testing.T) {
	keyring.MockInit()
	s := KeyringStore{}
	for _, p := range []string{"team/a", `a\b`, "..", "dj:stream", "CON"} {
		if err := s.CheckProfile(p); err != nil {
			t.Errorf("CheckProfile(%q) = %v, want nil", p, err)
		}
		if err := s.Save(p, "tok-"+p); err != nil {
			t.Fatalf("Save(%q) = %v", p, err)
		}
		if got, err := s.Load(p); err != nil || got != "tok-"+p {
			t.Errorf("Load(%q) = %q, %v", p, got, err)
		}
		if err := s.Clear(p); err != nil {
			t.Errorf("Clear(%q) = %v", p, err)
		}
	}
	if err := s.CheckProfile(""); err == nil || !strings.Contains(err.Error(), `invalid profile name ""`) {
		t.Errorf("CheckProfile(\"\") = %v, want an invalid profile error", err)
	}
}

// Save syncs the directory after the rename, so the new entry survives a
// crash: at the moment of the sync the token is already at its path.
func TestFileStore_SaveSyncsDirAfterRename(t *testing.T) {
	dir := t.TempDir()
	s := FileStore{Dir: dir}
	var synced []string
	orig := syncDir
	t.Cleanup(func() { syncDir = orig })
	syncDir = func(d string) error {
		b, err := os.ReadFile(filepath.Join(dir, "dj.token"))
		if err != nil || string(b) != "tok" {
			t.Errorf("at dir sync, dj.token = %q, %v; want the renamed token", b, err)
		}
		synced = append(synced, d)
		return nil
	}
	if err := s.Save("dj", "tok"); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 || synced[0] != dir {
		t.Errorf("synced %v, want [%s]", synced, dir)
	}
}

// A failed directory sync fails the save: login must not report a session
// it can't promise is on disk.
func TestFileStore_SaveReportsDirSyncError(t *testing.T) {
	s := FileStore{Dir: t.TempDir()}
	orig := syncDir
	t.Cleanup(func() { syncDir = orig })
	syncDir = func(string) error { return errors.New("disk on fire") }
	if err := s.Save("dj", "tok"); err == nil || !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("Save = %v, want the sync error", err)
	}
}

// The real directory sync works on a real directory.
func TestSyncDir_RealDir(t *testing.T) {
	if err := syncDir(t.TempDir()); err != nil {
		t.Errorf("syncDir = %v", err)
	}
}

// Save replaces a symlink at the token path instead of writing through it:
// the link's target keeps its contents and mode.
func TestFileStore_SaveDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(target, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "dj.token")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := (FileStore{Dir: dir}).Save("dj", "secret"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "precious" {
		t.Errorf("symlink target = %q, want it untouched", b)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o644 {
		t.Errorf("symlink target perms = %o, want 644", fi.Mode().Perm())
	}
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() {
		t.Errorf("token path mode = %v, want a regular file", fi.Mode())
	}
	if got, _ := (FileStore{Dir: dir}).Load("dj"); got != "secret" {
		t.Errorf("Load = %q, want secret", got)
	}
}

// Save writes a new file and renames it over the old one, never truncating
// the old token in place, so a save that fails partway leaves the previous
// session intact.
func TestFileStore_SaveReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	s := FileStore{Dir: dir}
	if err := s.Save("dj", "old"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "dj.token")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("dj", "new"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("Save rewrote the token file in place; want a new file renamed over it")
	}
	if got, _ := s.Load("dj"); got != "new" {
		t.Errorf("Load = %q, want new", got)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*")); len(m) != 1 {
		t.Errorf("store dir holds %v, want only dj.token", m)
	}
}

// os.WriteFile keeps an existing file's mode, so a token file left 0644 by
// something else must be tightened to 0600 when a session is saved over it.
func TestFileStore_TightensExistingFilePerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dj.token")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil { // defeat the umask
		t.Fatal(err)
	}
	if err := (FileStore{Dir: dir}).Save("dj", "secret"); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file perms = %o, want 600", perm)
	}
}
