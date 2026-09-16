package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testAuth(t *testing.T, password string) (*UIAuth, *UserStore) {
	t.Helper()
	store, err := NewUserStore(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	auth, err := NewUIAuth(store, "liuyuquan", password)
	if err != nil {
		t.Fatal(err)
	}
	return auth, store
}

func TestUIAuthSessionRenewsAndCookieUsesForwardedHTTPS(t *testing.T) {
	a, store := testAuth(t, "secret123")
	_, token, err := a.Login("liuyuquan", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	nearExpiry := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	if _, err := store.db.Exec(`UPDATE ui_sessions SET expires_at=? WHERE token_hash=?`, nearExpiry, credentialStamp(token)); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/accounts", nil)
	req.Header.Set("X-Forwarded-Proto", "https, http")
	req.AddCookie(&http.Cookie{Name: UICookieName, Value: token})
	w := httptest.NewRecorder()
	a.RefreshSession(w, req)
	setCookie := w.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "Secure") {
		t.Fatalf("renewed cookie should be Secure behind HTTPS proxy: %q", setCookie)
	}
	if !strings.Contains(setCookie, "Max-Age=2592000") {
		t.Fatalf("renewed cookie should keep 30-day max age: %q", setCookie)
	}
	if !a.ValidRequest(req) {
		t.Fatal("renewed session should remain valid")
	}
}

func TestUIAuthSuperadminRoundtrip(t *testing.T) {
	a, _ := testAuth(t, "secret123")
	identity, token, err := a.Login("liuyuquan", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	if !identity.IsSuperadmin() {
		t.Fatal("expected superadmin")
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ui/login", nil)
	a.IssueCookie(w, req, token)
	authed := httptest.NewRequest("GET", "/api/accounts", nil)
	authed.AddCookie(w.Result().Cookies()[0])
	if !a.ValidRequest(authed) {
		t.Fatal("valid session rejected")
	}
}

func TestRegularUserPasswordResetRevokesSession(t *testing.T) {
	a, store := testAuth(t, "secret123")
	user, err := store.Create("member", "password-old", true)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := a.Login("member", "password-old")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: UICookieName, Value: token})
	if !a.ValidRequest(req) {
		t.Fatal("user session rejected")
	}
	if err := store.SetPassword(user.ID, "password-new", true); err != nil {
		t.Fatal(err)
	}
	if a.ValidRequest(req) {
		t.Fatal("old session survived password reset")
	}
}

func TestDisabledUserCannotLogin(t *testing.T) {
	a, store := testAuth(t, "secret123")
	user, err := store.Create("member", "password-old", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(user.ID, "disabled"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Login("member", "password-old"); err == nil {
		t.Fatal("disabled user logged in")
	}
}
