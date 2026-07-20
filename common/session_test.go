package common_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"new-api-pilot/common"
	testsupport "new-api-pilot/tests/support"
)

func TestSignedSessionRoundTripTamperAndExpiry(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC))
	store, err := common.NewSessionStore([]byte("01234567890123456789012345678901"), true, clock)
	if err != nil {
		t.Fatalf("NewSessionStore() error = %v", err)
	}
	recorder := httptest.NewRecorder()
	identity := common.Identity{ID: "9007199254740993", Username: "admin", Role: "admin", Status: 1, SessionVersion: 7}
	if err := store.Write(recorder, identity); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Domain != "" {
		t.Fatalf("unexpected cookie: %#v", cookies)
	}
	request := httptest.NewRequest("GET", "/", nil)
	request.AddCookie(cookies[0])
	read, err := store.Read(request)
	if err != nil || read.ID != identity.ID || read.SessionVersion != 7 {
		t.Fatalf("Read() = %#v, %v", read, err)
	}

	tampered := *cookies[0]
	tampered.Value = strings.Replace(tampered.Value, "A", "B", 1)
	if tampered.Value == cookies[0].Value {
		tampered.Value += "x"
	}
	request = httptest.NewRequest("GET", "/", nil)
	request.AddCookie(&tampered)
	if _, err := store.Read(request); !errors.Is(err, common.ErrSessionInvalid) {
		t.Fatalf("tampered Read() error = %v", err)
	}

	clock.Advance(common.SessionMaxAge)
	request = httptest.NewRequest("GET", "/", nil)
	request.AddCookie(cookies[0])
	if _, err := store.Read(request); !errors.Is(err, common.ErrSessionExpired) {
		t.Fatalf("expired Read() error = %v", err)
	}
}

func TestSessionClearUsesHostOnlyExpiredCookie(t *testing.T) {
	store, err := common.NewSessionStore([]byte("01234567890123456789012345678901"), false, common.SystemClock{})
	if err != nil {
		t.Fatalf("NewSessionStore() error = %v", err)
	}
	recorder := httptest.NewRecorder()
	store.Clear(recorder)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 || cookies[0].Domain != "" || !cookies[0].HttpOnly {
		t.Fatalf("unexpected cleared cookie: %#v", cookies)
	}
}
