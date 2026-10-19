// Package auth issues and verifies tokens, hashes passwords, verifies
// Google and Telegram sign-ins, and provides the Gin middleware.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Role string

const (
	RoleStudent Role = "student"
	RoleAdmin   Role = "admin"
)

// Claims identify the caller. For admins Subject is the admin ID; for
// students it is the student ID.
type Claims struct {
	Role Role   `json:"role"`
	Name string `json:"name,omitempty"`
	// Student tokens also carry the sign-in identity, so the profile can be
	// created on first sign-up without trusting client input.
	Provider string `json:"prv,omitempty"`
	Identity string `json:"idn,omitempty"`
	// Photo is the sign-in picture (Google), the profile's first picture.
	Photo string `json:"pic,omitempty"`
	// Session is the signed-in device (student tokens since device limits;
	// older tokens have none).
	Session string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

type Tokens struct {
	secret []byte
	// SessionCheck, when set, confirms a student token's device session is
	// still signed in (another sign-in may have signed it out).
	SessionCheck func(c *gin.Context, claims *Claims) error
}

func NewTokens(secret []byte) *Tokens { return &Tokens{secret: secret} }

func (t *Tokens) Issue(role Role, subject, name string, ttl time.Duration) (string, error) {
	return t.issue(Claims{Role: role, Name: name}, subject, ttl)
}

// IssueStudent issues an app token for a signed-in identity.
func (t *Tokens) IssueStudent(studentID, name, provider, identity, photo, session string, ttl time.Duration) (string, error) {
	return t.issue(Claims{Role: RoleStudent, Name: name, Provider: provider, Identity: identity, Photo: photo, Session: session}, studentID, ttl)
}

func (t *Tokens) issue(claims Claims, subject string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims = Claims{
		Role:     claims.Role,
		Name:     claims.Name,
		Provider: claims.Provider,
		Identity: claims.Identity,
		Photo:    claims.Photo,
		Session:  claims.Session,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    "victory-api",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

func (t *Tokens) Parse(token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(tok *jwt.Token) (any, error) {
		if tok.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method %v", tok.Header["alg"])
		}
		return t.secret, nil
	}, jwt.WithIssuer("victory-api"), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// HashPassword uses bcrypt with the default cost.
func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

// CheckPassword compares in constant time.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// TelegramUser is the verified identity from a Telegram Login payload.
type TelegramUser struct {
	ID        string
	FirstName string
	LastName  string
	Username  string
	PhotoURL  string
}

var ErrInvalidTelegramLogin = errors.New("invalid telegram login")

// VerifyTelegramLogin checks a Telegram Login Widget payload: the hash is
// HMAC-SHA256 of the sorted "key=value" lines, keyed by SHA256(bot token).
// See https://core.telegram.org/widgets/login#checking-authorization.
func VerifyTelegramLogin(botToken string, data map[string]string, maxAge time.Duration) (*TelegramUser, error) {
	if botToken == "" {
		return nil, errors.New("telegram login is not configured")
	}
	hash := data["hash"]
	if hash == "" {
		return nil, ErrInvalidTelegramLogin
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + data[k]
	}
	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(strings.Join(lines, "\n")))
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(strings.ToLower(hash))) {
		return nil, ErrInvalidTelegramLogin
	}
	authDate, err := strconv.ParseInt(data["auth_date"], 10, 64)
	if err != nil || time.Since(time.Unix(authDate, 0)) > maxAge {
		return nil, ErrInvalidTelegramLogin
	}
	if data["id"] == "" {
		return nil, ErrInvalidTelegramLogin
	}
	return &TelegramUser{
		ID:        data["id"],
		FirstName: data["first_name"],
		LastName:  data["last_name"],
		Username:  data["username"],
		PhotoURL:  data["photo_url"],
	}, nil
}
