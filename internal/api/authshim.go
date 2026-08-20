package api

import "github.com/hurshnarayan/reyna/internal/auth"

// authGenerate mints the token used as the OAuth state parameter. Validated on
// the way back in the callback, so a callback cannot be replayed against a
// different account.
func authGenerate(s *Server, userID int64) (string, error) {
	return auth.GenerateToken(userID, s.cfg.JWTSecret)
}
