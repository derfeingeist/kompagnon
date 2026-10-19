package main

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const (
	passwordIterations = 600_000
	passwordKeyLength  = 32
	sessionDuration    = 30 * 24 * time.Hour
	inviteDuration     = 7 * 24 * time.Hour
	maxRequestBytes    = 1 << 20
)

type account struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	HouseholdID string `json:"household_id"`
	Household   string `json:"household"`
}

type accountContextKey struct{}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Household   string `json:"household"`
		InviteToken string `json:"invite_token"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Household = strings.TrimSpace(input.Household)
	if _, err := mail.ParseAddress(input.Email); err != nil || len(input.Email) > 254 {
		writeError(w, http.StatusBadRequest, "Enter a valid email address.")
		return
	}
	if len(input.Password) < 12 || len(input.Password) > 1024 {
		writeError(w, http.StatusBadRequest, "Password must be between 12 and 1024 characters.")
		return
	}
	if len(input.DisplayName) < 1 || len(input.DisplayName) > 80 {
		writeError(w, http.StatusBadRequest, "Name must be between 1 and 80 characters.")
		return
	}
	if input.InviteToken == "" && (len(input.Household) < 1 || len(input.Household) > 100) {
		writeError(w, http.StatusBadRequest, "Household name must be between 1 and 100 characters.")
		return
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create account.")
		return
	}
	hash, err := pbkdf2.Key(sha256.New, input.Password, salt, passwordIterations, passwordKeyLength)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create account.")
		return
	}
	userID, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create account.")
		return
	}
	now := time.Now().UTC().Unix()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create account.")
		return
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(r.Context(), `INSERT INTO users(id,email,display_name,password_salt,password_hash,created_at) VALUES(?,?,?,?,?,?)`,
		userID, input.Email, input.DisplayName, salt, hash, now); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, http.StatusConflict, "An account with that email already exists.")
		} else {
			writeError(w, http.StatusInternalServerError, "Could not create account.")
		}
		return
	}

	var householdID string
	role := "owner"
	if input.InviteToken != "" {
		tokenHash := sha256.Sum256([]byte(input.InviteToken))
		err = tx.QueryRowContext(r.Context(), `SELECT household_id FROM invites WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?`,
			tokenHash[:], now).Scan(&householdID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "This household invitation is invalid or expired.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not join household.")
			return
		}
		role = "member"
		if _, err = tx.ExecContext(r.Context(), `UPDATE invites SET used_at = ? WHERE token_hash = ? AND used_at IS NULL`, now, tokenHash[:]); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not join household.")
			return
		}
	} else {
		householdID, err = newID()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not create household.")
			return
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO households(id,name,created_by,created_at) VALUES(?,?,?,?)`,
			householdID, input.Household, userID, now); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not create household.")
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO memberships(user_id,household_id,role,joined_at) VALUES(?,?,?,?)`,
		userID, householdID, role, now); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not join household.")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create account.")
		return
	}

	s.createSession(w, r, userID)
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.Password) > 1024 {
		writeError(w, http.StatusBadRequest, "Password is too long.")
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))

	var userID string
	var salt, expected []byte
	err := s.db.QueryRowContext(r.Context(), `SELECT id,password_salt,password_hash FROM users WHERE email = ?`, input.Email).
		Scan(&userID, &salt, &expected)
	found := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "Could not sign in.")
		return
	}
	if !found {
		salt = []byte("kompagnon-login-dummy-salt")
		expected = make([]byte, passwordKeyLength)
	}
	actual, err := pbkdf2.Key(sha256.New, input.Password, salt, passwordIterations, passwordKeyLength)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not sign in.")
		return
	}
	if !found || subtle.ConstantTimeCompare(actual, expected) != 1 {
		writeError(w, http.StatusUnauthorized, "Email or password is incorrect.")
		return
	}
	s.createSession(w, r, userID)
}

func (s *server) createSession(w http.ResponseWriter, r *http.Request, userID string) {
	token, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not start session.")
		return
	}
	tokenHash := sha256.Sum256([]byte(token))
	expires := time.Now().UTC().Add(sessionDuration)
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,?,?)`,
		tokenHash[:], userID, expires.Unix()); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not start session.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookie)
	if err == nil {
		tokenHash := sha256.Sum256([]byte(cookie.Value))
		if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token_hash = ?`, tokenHash[:]); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not end session.")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "Sign in to continue.")
			return
		}
		tokenHash := sha256.Sum256([]byte(cookie.Value))
		var user account
		err = s.db.QueryRowContext(r.Context(), `
SELECT u.id,u.email,u.display_name,m.household_id,h.name
FROM sessions s
JOIN users u ON u.id=s.user_id
JOIN memberships m ON m.user_id=u.id
JOIN households h ON h.id=m.household_id
WHERE s.token_hash=? AND s.expires_at>?`, tokenHash[:], time.Now().UTC().Unix()).
			Scan(&user.ID, &user.Email, &user.DisplayName, &user.HouseholdID, &user.Household)
		if err != nil {
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode})
			writeError(w, http.StatusUnauthorized, "Sign in to continue.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), accountContextKey{}, user)))
	})
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, r.Context().Value(accountContextKey{}).(account))
}

func (s *server) createInvite(w http.ResponseWriter, r *http.Request) {
	user := currentAccount(r)
	token, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create invitation.")
		return
	}
	tokenHash := sha256.Sum256([]byte(token))
	expires := time.Now().UTC().Add(inviteDuration)
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO invites(token_hash,household_id,created_by,expires_at) VALUES(?,?,?,?)`,
		tokenHash[:], user.HouseholdID, user.ID, expires.Unix()); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create invitation.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      token,
		"expires_at": expires.UTC().Format(time.RFC3339),
	})
}

func currentAccount(r *http.Request) account {
	return r.Context().Value(accountContextKey{}).(account)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "Request body must be valid JSON.")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Request body must contain one JSON value.")
		return false
	}
	return true
}

func randomToken(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func newID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
