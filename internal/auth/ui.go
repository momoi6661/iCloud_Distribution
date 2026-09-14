package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"
)

const UICookieName = "hme_ui_session"
const uiSessionTTL = 12 * time.Hour

type UIAuth struct {
	store             *UserStore
	superUsername     string
	superPasswordHash [32]byte
	superStamp        string
}

func NewUIAuth(store *UserStore, username, password string) (*UIAuth, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, errors.New("超级管理员用户名和密码不能为空")
	}
	return &UIAuth{store: store, superUsername: username, superPasswordHash: sha256.Sum256([]byte(password)), superStamp: credentialStamp(username + "\n" + password)}, nil
}

func (a *UIAuth) Login(username, password string) (*Identity, string, error) {
	username = strings.TrimSpace(username)
	want := sha256.Sum256([]byte(password))
	if strings.EqualFold(username, a.superUsername) && subtle.ConstantTimeCompare(want[:], a.superPasswordHash[:]) == 1 {
		identity := Identity{ID: SuperadminID, Username: a.superUsername, Role: "superadmin"}
		token, err := a.store.IssueSession(identity, a.superStamp, uiSessionTTL)
		return &identity, token, err
	}
	user, err := a.store.Authenticate(username, password)
	if err != nil {
		return nil, "", err
	}
	identity := Identity{ID: user.ID, Username: user.Username, Role: user.Role, MustChange: user.MustChange}
	token, err := a.store.IssueSession(identity, credentialStamp(user.passwordHash), uiSessionTTL)
	return &identity, token, err
}

func (a *UIAuth) Identity(r *http.Request) (*Identity, bool) {
	cookie, err := r.Cookie(UICookieName)
	if err != nil {
		return nil, false
	}
	return a.store.Session(cookie.Value, a.superStamp)
}

func (a *UIAuth) IssueCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{Name: UICookieName, Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: int(uiSessionTTL.Seconds())})
}

func (a *UIAuth) ClearCookie(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(UICookieName); err == nil {
		_ = a.store.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: UICookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
}

func (a *UIAuth) ValidRequest(r *http.Request) bool { _, ok := a.Identity(r); return ok }
func (a *UIAuth) Store() *UserStore                 { return a.store }
