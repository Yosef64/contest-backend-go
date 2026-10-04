package api

import (
	"crypto/rand"
	"errors"
	"math/big"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// Admins who forget their password ask the owner, who issues a temporary
// one here (requirePermission already limits writes under /admins to the
// owner). The admin signs in with it and sets their own in Settings.

const tempPasswordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

func tempPassword(n int) (string, error) {
	out := make([]byte, n)
	for i := range out {
		k, err := rand.Int(rand.Reader, big.NewInt(int64(len(tempPasswordAlphabet))))
		if err != nil {
			return "", err
		}
		out[i] = tempPasswordAlphabet[k.Int64()]
	}
	return string(out), nil
}

// adminResetAdminPassword: POST /admin/admins/:id/password-reset
func (s *Server) adminResetAdminPassword(c *gin.Context) error {
	owner := currentAdmin(c)
	if !owner.IsOwner {
		return httpx.Forbidden("Only the owner can reset administrator passwords.")
	}
	target, err := s.store.AdminByID(c, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("Admin")
	}
	if err != nil {
		return err
	}
	if target.IsOwner || target.ID == owner.ID {
		return httpx.BadRequest("Change your own password in Settings.")
	}
	password, err := tempPassword(14)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err = s.pool.Exec(c, "UPDATE admins SET password_hash=$1 WHERE id=$2", hash, target.ID); err != nil {
		return err
	}
	if _, err = s.pool.Exec(c, `INSERT INTO audit_events (admin_id, action, resource) VALUES ($1, 'admin_password_reset', $2)`, owner.ID, target.ID); err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"password": password, "email": target.Email})
}
