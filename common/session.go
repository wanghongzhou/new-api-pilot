package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	SessionCookieName = "session"
	SessionMaxAge     = 30 * 24 * time.Hour
	maxSessionBytes   = 4096
)

var (
	ErrSessionMissing = errors.New("session cookie is missing")
	ErrSessionInvalid = errors.New("session cookie is invalid")
	ErrSessionExpired = errors.New("session cookie has expired")
)

type sessionPayload struct {
	Identity
	ExpiresAt int64 `json:"expires_at"`
}

type SessionStore struct {
	secret []byte
	secure bool
	clock  Clock
}

func NewSessionStore(secret []byte, secure bool, clock Clock) (*SessionStore, error) {
	if len(secret) < 32 {
		return nil, errors.New("session secret must contain at least 32 bytes")
	}
	if clock == nil {
		clock = SystemClock{}
	}
	secretCopy := append([]byte(nil), secret...)
	return &SessionStore{secret: secretCopy, secure: secure, clock: clock}, nil
}

func (store *SessionStore) Write(writer http.ResponseWriter, identity Identity) error {
	payload := sessionPayload{Identity: identity, ExpiresAt: store.clock.Now().Add(SessionMaxAge).Unix()}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}
	value := base64.RawURLEncoding.EncodeToString(encodedPayload)
	signature := base64.RawURLEncoding.EncodeToString(store.sign(value))
	cookieValue := value + "." + signature
	if len(cookieValue) > maxSessionBytes {
		return ErrSessionInvalid
	}
	http.SetCookie(writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    cookieValue,
		Path:     "/",
		MaxAge:   int(SessionMaxAge / time.Second),
		Expires:  store.clock.Now().Add(SessionMaxAge),
		HttpOnly: true,
		Secure:   store.secure,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func (store *SessionStore) Read(request *http.Request) (Identity, error) {
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return Identity{}, ErrSessionMissing
		}
		return Identity{}, ErrSessionInvalid
	}
	if len(cookie.Value) == 0 || len(cookie.Value) > maxSessionBytes {
		return Identity{}, ErrSessionInvalid
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return Identity{}, ErrSessionInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(signature) != sha256.Size {
		return Identity{}, ErrSessionInvalid
	}
	expected := store.sign(parts[0])
	if subtle.ConstantTimeCompare(signature, expected) != 1 {
		return Identity{}, ErrSessionInvalid
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Identity{}, ErrSessionInvalid
	}
	var payload sessionPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return Identity{}, ErrSessionInvalid
	}
	if payload.ID == "" || payload.SessionVersion <= 0 || payload.ExpiresAt <= store.clock.Now().Unix() {
		if payload.ExpiresAt <= store.clock.Now().Unix() {
			return Identity{}, ErrSessionExpired
		}
		return Identity{}, ErrSessionInvalid
	}
	return payload.Identity, nil
}

func (store *SessionStore) Clear(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   store.secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (store *SessionStore) sign(value string) []byte {
	mac := hmac.New(sha256.New, store.secret)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
