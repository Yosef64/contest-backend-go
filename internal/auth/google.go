package auth

import (
	"context"
	"errors"

	"google.golang.org/api/idtoken"
)

// GoogleUser is the verified identity from a Google ID token.
type GoogleUser struct {
	Subject  string
	Email    string
	Name     string
	Picture  string
	Verified bool
}

var ErrInvalidGoogleToken = errors.New("invalid google token")

// VerifyGoogleIDToken validates signature, expiry, issuer and audience
// (one of the app's OAuth client IDs).
func VerifyGoogleIDToken(ctx context.Context, token string, clientIDs []string) (*GoogleUser, error) {
	if len(clientIDs) == 0 {
		return nil, errors.New("google sign-in is not configured")
	}
	var lastErr error
	for _, aud := range clientIDs {
		p, err := idtoken.Validate(ctx, token, aud)
		if err != nil {
			lastErr = err
			continue
		}
		str := func(k string) string { s, _ := p.Claims[k].(string); return s }
		verified, _ := p.Claims["email_verified"].(bool)
		return &GoogleUser{Subject: p.Subject, Email: str("email"), Name: str("name"), Picture: str("picture"), Verified: verified}, nil
	}
	return nil, errors.Join(ErrInvalidGoogleToken, lastErr)
}
