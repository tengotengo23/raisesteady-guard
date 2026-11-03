package core

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// PinHash is an argon2id hash of the parent's PIN with its own random salt.
type PinHash struct {
	Alg     string `json:"alg"`
	Salt    string `json:"salt"`
	Hash    string `json:"hash"`
	Time    uint32 `json:"t"`
	MemKiB  uint32 `json:"m"`
	Threads uint8  `json:"p"`
}

// Argon2id parameters: OWASP's minimum (19 MiB, 2 passes) raised to 3 passes.
const (
	pinTime    = 3
	pinMemKiB  = 19 * 1024
	pinThreads = 1
	pinKeyLen  = 32
)

// HashPin returns a new salted argon2id hash of pin.
func HashPin(pin string) (PinHash, error) {
	if !ValidPin(pin) {
		return PinHash{}, errors.New("the PIN must be 4 to 8 digits")
	}
	saltB := make([]byte, 16)
	if _, err := rand.Read(saltB); err != nil {
		return PinHash{}, err
	}
	h := argon2.IDKey([]byte(pin), saltB, pinTime, pinMemKiB, pinThreads, pinKeyLen)
	return PinHash{
		Alg: "argon2id", Salt: base64.RawStdEncoding.EncodeToString(saltB),
		Hash: base64.RawStdEncoding.EncodeToString(h), Time: pinTime, MemKiB: pinMemKiB, Threads: pinThreads,
	}, nil
}

// Check compares pin with the stored hash in constant time.
func (p PinHash) Check(pin string) bool {
	if p.Alg != "argon2id" || !ValidPin(pin) || p.Time == 0 || p.Time > 10 || p.MemKiB < 8*1024 || p.MemKiB > 256*1024 || p.Threads == 0 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(p.Salt)
	want, err2 := base64.RawStdEncoding.DecodeString(p.Hash)
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(pin), salt, p.Time, p.MemKiB, p.Threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ConstantTimeEqual compares two strings without leaking where they differ.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Lockout limits PIN guesses: after MaxTries wrong PINs from one source, that
// source's checks are refused for LockFor. Sources are counted apart ("parent"
// for the parent page, "api:<SID>" for an extension token, "local" for the
// installer), so a child who locks the API cannot lock the parent page. The
// state is kept in a file so the service and the uninstaller share it. It also
// counts all wrong PINs of the day for the parent page.
type Lockout struct {
	mu   sync.Mutex
	Path string
	Now  func() time.Time
	mem  lockFile
}

const (
	MaxTries = 5
	LockFor  = 15 * time.Minute
	maxKeys  = 64
)

type lockState struct {
	Failures    int       `json:"failures"`
	LockedUntil time.Time `json:"lockedUntil"`
}

type lockFile struct {
	Keys       map[string]*lockState `json:"keys"`
	Day        string                `json:"day"`
	TodayFails int                   `json:"todayFails"`
	LastFail   time.Time             `json:"lastFail"`
	damaged    bool
}

// ErrLocked is returned while PIN checks are locked.
var ErrLocked = errors.New("too many wrong PINs; try again later")

func (l *Lockout) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l *Lockout) load() lockFile {
	f := l.mem
	if l.Path != "" {
		var disk lockFile
		if err := LoadJSON(l.Path, &disk); err == nil {
			f = disk
		} else if !errors.Is(err, os.ErrNotExist) {
			f = lockFile{damaged: true}
		}
	}
	if f.Keys == nil {
		f.Keys = map[string]*lockState{}
	}
	return f
}

func (l *Lockout) save(f lockFile) error {
	l.mem = f
	if l.Path == "" {
		return nil
	}
	return SaveJSON(l.Path, f)
}

// Verify checks pin for the "local" source (installer, uninstaller).
func (l *Lockout) Verify(hash PinHash, pin string) (bool, int, time.Duration, error) {
	return l.VerifyKey("local", hash, pin)
}

// VerifyKey checks pin against hash for one source. It returns
// (ok, triesLeft, retryAfter, err).
func (l *Lockout) VerifyKey(key string, hash PinHash, pin string) (bool, int, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.load()
	now := l.now()
	s := f.Keys[key]
	if s == nil {
		s = &lockState{}
		if f.damaged {
			// A damaged file must not open the lock: one try left.
			s.Failures = MaxTries - 1
		}
	}
	if now.Before(s.LockedUntil) {
		return false, 0, s.LockedUntil.Sub(now), ErrLocked
	}
	if !s.LockedUntil.IsZero() {
		s = &lockState{}
	}
	if hash.Check(pin) {
		if s.Failures != 0 || f.Keys[key] != nil {
			delete(f.Keys, key)
			if err := l.save(f); err != nil {
				return true, MaxTries, 0, nil
			}
		}
		return true, MaxTries, 0, nil
	}
	s.Failures++
	day := now.Format("2006-01-02")
	if f.Day != day {
		f.Day, f.TodayFails = day, 0
	}
	f.TodayFails++
	f.LastFail = now
	left := MaxTries - s.Failures
	if s.Failures >= MaxTries {
		s.LockedUntil = now.Add(LockFor)
		s.Failures = 0
		left = 0
	}
	if len(f.Keys) >= maxKeys {
		for k, v := range f.Keys {
			if !now.Before(v.LockedUntil) && k != key {
				delete(f.Keys, k)
			}
		}
	}
	f.Keys[key] = s
	if err := l.save(f); err != nil {
		return false, left, 0, err
	}
	if left == 0 {
		return false, 0, LockFor, ErrLocked
	}
	return false, left, 0, nil
}

// Today returns how many wrong PINs were entered today, from every source,
// and when the last one was.
func (l *Lockout) Today() (int, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.load()
	if f.Day != l.now().Format("2006-01-02") {
		return 0, time.Time{}
	}
	return f.TodayFails, f.LastFail
}
