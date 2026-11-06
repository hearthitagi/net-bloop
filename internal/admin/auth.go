package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const sessionCookie = "ll_session"

// HashPassword returns an Argon2id hash in the standard PHC string format.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	const (
		memory  = 64 * 1024
		time    = 1
		threads = 2
		keyLen  = 32
	)
	sum := argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, time, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum)), nil
}

// VerifyPassword checks a password against a PHC encoded Argon2id hash.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		// Allow a legacy plain-text value so an empty config can boot.
		return subtle.ConstantTimeCompare([]byte(encoded), []byte(password)) == 1
	}
	var memory, iters uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iters, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iters, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

type attempt struct {
	fails int
	until time.Time
}

type auth struct {
	mu       sync.Mutex
	attempts map[string]*attempt
}

func newAuth() *auth { return &auth{attempts: map[string]*attempt{}} }

func (a *auth) allowed(ip string) (bool, time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	at, ok := a.attempts[ip]
	if !ok {
		return true, 0
	}
	if time.Now().Before(at.until) {
		return false, time.Until(at.until).Round(time.Second)
	}
	return true, 0
}

func (a *auth) fail(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	at, ok := a.attempts[ip]
	if !ok {
		at = &attempt{}
		a.attempts[ip] = at
	}
	at.fails++
	if at.fails >= 6 {
		at.fails = 0
		at.until = time.Now().Add(30 * time.Second)
	}
}

func (a *auth) reset(ip string) {
	a.mu.Lock()
	delete(a.attempts, ip)
	a.mu.Unlock()
}

func token(secret, user string) string {
	exp := strconv.FormatInt(time.Now().Add(7*24*time.Hour).Unix(), 10)
	payload := exp + "|" + user
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

func validToken(secret, tok string) (string, bool) {
	left, right, ok := strings.Cut(tok, ".")
	if !ok {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(left)
	if err != nil {
		return "", false
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return "", false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), []byte(right)) != 1 {
		return "", false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	return parts[1], true
}

func sessionSecret(passwordHash string) string {
	sum := sha256.Sum256([]byte("net-bloop|" + passwordHash))
	return hex.EncodeToString(sum[:])
}
