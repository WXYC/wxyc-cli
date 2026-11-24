package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zalando/go-keyring"
)

// ErrNoSession indicates no session token is stored for the profile.
var ErrNoSession = errors.New("no session token; run `wxyc login` first")

// ErrSessionExpired indicates the stored session token was rejected by the
// token-exchange endpoint (HTTP 401) and is no longer valid. Like ErrNoSession
// it maps to ExitAuth: the remedy is to run `wxyc login` again.
var ErrSessionExpired = errors.New("session token rejected; run `wxyc login` again")

// keyringService namespaces the CLI's secrets in the OS keychain.
const keyringService = "wxyc-cli"

// Store persists the long-lived session token per profile. The password is
// never stored — only the session token, which the TokenProvider exchanges for
// short-lived JWTs.
type Store interface {
	// CheckProfile reports whether the store can hold a session under
	// profile, so a caller can refuse a bad name before doing any work
	// (login checks it before asking for a password).
	CheckProfile(profile string) error
	Save(profile, token string) error
	Load(profile string) (string, error)
	Clear(profile string) error
}

// KeyringStore keeps the session token in the OS keychain.
type KeyringStore struct{}

// CheckProfile refuses only an empty name. The keychain keys a session by an
// opaque account name, so slashes and the like are harmless there, and
// sessions saved under such names before file-name rules existed must stay
// usable.
func (KeyringStore) CheckProfile(profile string) error {
	if profile == "" {
		return fmt.Errorf("invalid profile name %q: it can't be empty", profile)
	}
	return nil
}

func (KeyringStore) Save(profile, token string) error {
	return keyring.Set(keyringService, profile, token)
}

func (KeyringStore) Load(profile string) (string, error) {
	tok, err := keyring.Get(keyringService, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNoSession
	}
	if err != nil {
		return "", err
	}
	return tok, nil
}

func (KeyringStore) Clear(profile string) error {
	err := keyring.Delete(keyringService, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// FileStore is the fallback when no OS keychain is available (e.g. a headless
// server). Tokens are written to Dir/<profile>.token with 0600 perms.
type FileStore struct {
	Dir string
}

// CheckProfile refuses a profile name that isn't a safe, portable file name.
// The token lives at Dir/<profile>.token, so the name must stay inside Dir
// on every OS: only letters, digits, '.', '-' and '_' are allowed (no
// separators, no ':' for a Windows drive or alternate data stream), "." and
// ".." are refused, and so are Windows device names (CON, NUL, COM1, ...),
// which Windows reserves even with an extension. Dots elsewhere
// ("stage..2") are fine. A bad name is refused, never cleaned.
func (FileStore) CheckProfile(profile string) error {
	if profile == "" || profile == "." || profile == ".." || !safeName(profile) || windowsDevice(profile) {
		return fmt.Errorf("invalid profile name %q: use only letters, digits, '.', '-' and '_', not \".\" or \"..\" or a Windows device name such as CON", profile)
	}
	return nil
}

func safeName(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// windowsDevice reports whether name, up to its first dot, is a device name
// Windows reserves in every directory.
func windowsDevice(name string) bool {
	stem, _, _ := strings.Cut(name, ".")
	switch stem = strings.ToUpper(stem); stem {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) &&
		stem[3] >= '0' && stem[3] <= '9'
}

// path is the profile's token file, inside Dir.
func (f FileStore) path(profile string) (string, error) {
	if err := f.CheckProfile(profile); err != nil {
		return "", err
	}
	return filepath.Join(f.Dir, profile+".token"), nil
}

func (f FileStore) Save(profile, token string) error {
	p, err := f.path(profile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	// Write a new file and rename it over the token path. The rename
	// replaces a symlink there rather than writing through it, and a save
	// that fails partway leaves the previous token in place.
	tmp, err := os.CreateTemp(f.Dir, ".token-*") // created 0600
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.WriteString(token); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return err
	}
	// Sync the directory too, or a crash can lose the rename and with it
	// the session login just reported saved.
	return syncDir(f.Dir)
}

// syncDir flushes a directory's entries to disk. It is a variable so tests
// can observe it. Windows can't open a directory for syncing, and NTFS
// journals the rename itself, so there it does nothing.
var syncDir = func(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (f FileStore) Load(profile string) (string, error) {
	p, err := f.path(profile)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNoSession
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (f FileStore) Clear(profile string) error {
	p, err := f.path(profile)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// defaultStoreDir returns the per-user config dir for the file fallback.
func defaultStoreDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "wxyc-cli"), nil
}

// NewStore returns the keyring store if the platform keychain is usable, else
// the file fallback. Usability is probed with a round-trip on a sentinel key.
func NewStore() Store {
	if keyringUsable() {
		return KeyringStore{}
	}
	dir, err := defaultStoreDir()
	if err != nil {
		dir = filepath.Join(os.TempDir(), "wxyc-cli")
	}
	return FileStore{Dir: dir}
}

func keyringUsable() bool {
	const probe = "__probe__"
	if err := keyring.Set(keyringService, probe, "1"); err != nil {
		return false
	}
	_ = keyring.Delete(keyringService, probe)
	return true
}
