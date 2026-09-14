package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	_ "modernc.org/sqlite"
)

const SuperadminID = "superadmin"

type Identity struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	MustChange bool   `json:"must_change_password,omitempty"`
}

func (i Identity) IsSuperadmin() bool { return i.Role == "superadmin" }

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	MustChange   bool   `json:"must_change_password"`
	CreatedAt    string `json:"created_at"`
	LastLoginAt  string `json:"last_login_at,omitempty"`
	passwordHash string
}

type UserStore struct{ db *sql.DB }

func NewUserStore(path string) (*UserStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'user',
			status TEXT NOT NULL DEFAULT 'active', must_change_password INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL, last_login_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS ui_sessions (
			token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL, username TEXT NOT NULL,
			role TEXT NOT NULL, credential_stamp TEXT NOT NULL,
			created_at TEXT NOT NULL, expires_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ui_sessions_expiry ON ui_sessions(expires_at)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &UserStore{db: db}, nil
}

func (s *UserStore) Close() error { return s.db.Close() }

func normalizeUsername(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 3 || len(value) > 40 {
		return "", errors.New("用户名长度必须为 3 到 40 个字符")
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' && r != '.' {
			return "", errors.New("用户名只能包含字母、数字、点、下划线和短横线")
		}
	}
	return value, nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("密码至少需要 8 个字符")
	}
	if len(password) > 256 {
		return errors.New("密码过长")
	}
	return nil
}

func hashPassword(password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[3])
	expected, err2 := base64.RawStdEncoding.DecodeString(parts[4])
	if err1 != nil || err2 != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, uint32(len(expected)))
	return subtleEqual(actual, expected)
}

func subtleEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func (s *UserStore) Create(username, password string, mustChange bool) (*User, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return nil, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	user := &User{ID: "usr_" + uuid.NewString(), Username: username, Role: "user", Status: "active", MustChange: mustChange, CreatedAt: now}
	_, err = s.db.Exec(`INSERT INTO users(id,username,password_hash,role,status,must_change_password,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, user.ID, user.Username, hash, user.Role, user.Status, boolInt(mustChange), now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errors.New("用户名已存在")
		}
		return nil, err
	}
	return user, nil
}

func (s *UserStore) List() ([]User, error) {
	rows, err := s.db.Query(`SELECT id,username,role,status,must_change_password,created_at,last_login_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		var must int
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.Status, &must, &u.CreatedAt, &u.LastLoginAt); err != nil {
			return nil, err
		}
		u.MustChange = must != 0
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *UserStore) getByUsername(username string) (*User, error) {
	var u User
	var must int
	err := s.db.QueryRow(`SELECT id,username,password_hash,role,status,must_change_password,created_at,last_login_at FROM users WHERE username=?`, strings.TrimSpace(username)).Scan(&u.ID, &u.Username, &u.passwordHash, &u.Role, &u.Status, &must, &u.CreatedAt, &u.LastLoginAt)
	if err != nil {
		return nil, err
	}
	u.MustChange = must != 0
	return &u, nil
}

func (s *UserStore) getByID(id string) (*User, error) {
	var u User
	var must int
	err := s.db.QueryRow(`SELECT id,username,password_hash,role,status,must_change_password,created_at,last_login_at FROM users WHERE id=?`, id).Scan(&u.ID, &u.Username, &u.passwordHash, &u.Role, &u.Status, &must, &u.CreatedAt, &u.LastLoginAt)
	if err != nil {
		return nil, err
	}
	u.MustChange = must != 0
	return &u, nil
}

func (s *UserStore) Authenticate(username, password string) (*User, error) {
	u, err := s.getByUsername(username)
	if err != nil || u.Status != "active" || !verifyPassword(u.passwordHash, password) {
		return nil, errors.New("用户名或密码错误")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(`UPDATE users SET last_login_at=? WHERE id=?`, now, u.ID)
	u.LastLoginAt = now
	return u, nil
}

func (s *UserStore) SetPassword(id, password string, mustChange bool) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE users SET password_hash=?,must_change_password=?,updated_at=? WHERE id=?`, hash, boolInt(mustChange), time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	_, _ = s.db.Exec(`DELETE FROM ui_sessions WHERE user_id=?`, id)
	return nil
}

func (s *UserStore) ChangeOwnPassword(id, current, next string) error {
	u, err := s.getByID(id)
	if err != nil {
		return err
	}
	if !verifyPassword(u.passwordHash, current) {
		return errors.New("当前密码错误")
	}
	return s.SetPassword(id, next, false)
}

func (s *UserStore) SetStatus(id, status string) error {
	if status != "active" && status != "disabled" {
		return errors.New("无效状态")
	}
	result, err := s.db.Exec(`UPDATE users SET status=?,updated_at=? WHERE id=?`, status, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	if status == "disabled" {
		_, _ = s.db.Exec(`DELETE FROM ui_sessions WHERE user_id=?`, id)
	}
	return nil
}

func (s *UserStore) Delete(id string) error {
	result, err := s.db.Exec(`DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	_, _ = s.db.Exec(`DELETE FROM ui_sessions WHERE user_id=?`, id)
	return nil
}

func credentialStamp(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *UserStore) IssueSession(identity Identity, stamp string, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC()
	expires := now.Add(ttl)
	_, err := s.db.Exec(`INSERT INTO ui_sessions(token_hash,user_id,username,role,credential_stamp,created_at,expires_at) VALUES(?,?,?,?,?,?,?)`, credentialStamp(token), identity.ID, identity.Username, identity.Role, stamp, now.Format(time.RFC3339), expires.Format(time.RFC3339))
	return token, err
}

func (s *UserStore) Session(token, superStamp string) (*Identity, bool) {
	if token == "" {
		return nil, false
	}
	var identity Identity
	var stamp, expires string
	err := s.db.QueryRow(`SELECT user_id,username,role,credential_stamp,expires_at FROM ui_sessions WHERE token_hash=?`, credentialStamp(token)).Scan(&identity.ID, &identity.Username, &identity.Role, &stamp, &expires)
	if err != nil {
		return nil, false
	}
	expiry, err := time.Parse(time.RFC3339, expires)
	if err != nil || time.Now().After(expiry) {
		_ = s.DeleteSession(token)
		return nil, false
	}
	if identity.IsSuperadmin() {
		if stamp != superStamp {
			return nil, false
		}
		return &identity, true
	}
	u, err := s.getByID(identity.ID)
	if err != nil || u.Status != "active" || stamp != credentialStamp(u.passwordHash) {
		return nil, false
	}
	return &identity, true
}

func (s *UserStore) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM ui_sessions WHERE token_hash=?`, credentialStamp(token))
	return err
}
