package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Save(profile, token string) error
	Load(profile string) (string, error)
	Clear(profile string) error
}

// KeyringStore keeps the session token in the OS keychain.
type KeyringStore struct{}

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

// ValidateProfile refuses a profile name that isn't a plain file name. The
// file store keeps a profile's token at Dir/<profile>.token, so a name with a
// separator or ".." could reach outside Dir; it is refused, never cleaned.
func ValidateProfile(profile string) error {
	if profile == "" || strings.ContainsAny(profile, `/\`) || strings.Contains(profile, "..") {
		return fmt.Errorf("invalid profile name %q: it can't be empty or contain a slash, backslash or \"..\"", profile)
	}
	return nil
}

// path is the profile's token file, inside Dir.
func (f FileStore) path(profile string) (string, error) {
	if err := ValidateProfile(profile); err != nil {
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
	file, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	// The create mode only applies to a new file, so tighten an existing one
	// left looser before the token goes into it.
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.WriteString(token); err != nil {
		file.Close()
		return err
	}
	return file.Close()
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
