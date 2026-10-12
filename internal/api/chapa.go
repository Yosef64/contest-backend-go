package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// Chapa payments ---------------------------------------------------------------
//
// The student starts a payment for an enrollment (academic or training); the
// server asks Chapa for a hosted checkout page and records a pending
// transaction. When the student comes back (or Chapa calls the webhook or
// the callback) the server verifies the transaction with Chapa itself and,
// if it was paid in full, approves the enrollment for the configured number
// of days. Nothing the client or the webhook sends is trusted on its own.

var chapaHTTP = &http.Client{Timeout: 20 * time.Second}

// Prices and enrollment length come from site settings (admin Settings).
const (
	defaultEnrollmentDays = 365
)

func (s *Server) enrollmentPrice(c *gin.Context, module string) (float64, int, error) {
	settings, err := s.siteSettings(c)
	if err != nil {
		return 0, 0, err
	}
	price, _ := strconv.ParseFloat(settings["price_"+module], 64)
	if module == "both" && price <= 0 {
		// No bundle price set: both plans cost the two prices together.
		a, _ := strconv.ParseFloat(settings["price_academic"], 64)
		t, _ := strconv.ParseFloat(settings["price_training"], 64)
		if a > 0 && t > 0 {
			price = a + t
		}
	}
	days, _ := strconv.Atoi(settings["enrollment_days"])
	if days <= 0 {
		days = defaultEnrollmentDays
	}
	return price, days, nil
}

func newTxRef() (string, error) {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 18)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return "vic-" + string(b), nil
}

func (s *Server) chapaRequest(c context.Context, method, path string, body any, out any) error {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(c, method, s.cfg.ChapaBaseURL+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.ChapaSecretKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := chapaHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("chapa: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		var e struct {
			Message any `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		return fmt.Errorf("chapa %d: %v", res.StatusCode, e.Message)
	}
	return json.Unmarshal(raw, out)
}

// POST /payments/chapa {module}
func (s *Server) startChapaPayment(c *gin.Context) error {
	var in struct {
		Module string `json:"module" binding:"required,oneof=academic training both"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if !s.cfg.ChapaEnabled() {
		return httpx.Conflict("payments_unavailable", "Online payment isn't set up yet. Send your payment proof instead.")
	}
	st, err := s.me(c)
	if err != nil {
		return err
	}
	price, _, err := s.enrollmentPrice(c, in.Module)
	if err != nil {
		return err
	}
	if price <= 0 {
		return httpx.Conflict("payments_unavailable", "The academy hasn't set a price for this program yet. Send your payment proof instead.")
	}
	txRef, err := newTxRef()
	if err != nil {
		return err
	}
	first, last := splitName(st.Name)
	body := map[string]any{
		"amount":       strconv.FormatFloat(price, 'f', 2, 64),
		"currency":     "ETB",
		"first_name":   first,
		"last_name":    last,
		"tx_ref":       txRef,
		"callback_url": s.cfg.PublicBaseURL + "/api/v1/payments/chapa/callback",
		"return_url":   s.cfg.ChapaReturnURL + "?tx_ref=" + url.QueryEscape(txRef),
		"customization": map[string]string{
			"title":       "Victory",
			"description": map[string]string{"academic": "Academic plan", "training": "Training plan", "both": "Academic and Training plans"}[in.Module],
		},
	}
	// Chapa wants a valid email and an Ethiopian mobile number when given.
	var login string
	_ = s.pool.QueryRow(c, `SELECT login FROM student_credentials WHERE student_id = $1`, st.ID).Scan(&login)
	if strings.Contains(login, "@") {
		body["email"] = login
	}
	if phone := chapaPhone(st.PhoneNumber); phone != "" {
		body["phone_number"] = phone
	}
	var res struct {
		Status string `json:"status"`
		Data   struct {
			CheckoutURL string `json:"checkout_url"`
		} `json:"data"`
	}
	if err := s.chapaRequest(c, http.MethodPost, "/v1/transaction/initialize", body, &res); err != nil || res.Data.CheckoutURL == "" {
		return httpx.NewError(http.StatusBadGateway, "payment_failed", "Chapa couldn't start the payment. Try again in a moment.")
	}
	if _, err := s.pool.Exec(c, `INSERT INTO chapa_payments (tx_ref, student_id, module, amount, checkout_url) VALUES ($1, $2, $3, $4, $5)`,
		txRef, st.ID, in.Module, price, res.Data.CheckoutURL); err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"tx_ref": txRef, "checkout_url": res.Data.CheckoutURL, "amount": price, "currency": "ETB"})
}

func splitName(name string) (string, string) {
	parts := strings.Fields(name)
	switch len(parts) {
	case 0:
		return "Victory", "Student"
	case 1:
		return parts[0], parts[0]
	}
	return parts[0], strings.Join(parts[1:], " ")
}

// chapaPhone returns 09xxxxxxxx / 07xxxxxxxx, or "" if the number isn't one.
func chapaPhone(raw string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)
	switch {
	case len(digits) == 12 && strings.HasPrefix(digits, "251"):
		digits = "0" + digits[3:]
	case len(digits) == 9:
		digits = "0" + digits
	}
	if len(digits) == 10 && (strings.HasPrefix(digits, "09") || strings.HasPrefix(digits, "07")) {
		return digits
	}
	return ""
}

// verifyChapa asks Chapa about tx_ref and, when it was paid in full,
// approves the enrollment. Safe to call many times.
func (s *Server) verifyChapa(c *gin.Context, txRef string) (string, error) {
	var p struct {
		studentID, module, status string
		amount                    float64
	}
	err := s.pool.QueryRow(c, `SELECT student_id, module, status, amount::float8 FROM chapa_payments WHERE tx_ref = $1`, txRef).
		Scan(&p.studentID, &p.module, &p.status, &p.amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", store.ErrNotFound
	}
	if err != nil || p.status != "pending" {
		return p.status, err
	}
	var res struct {
		Status string `json:"status"`
		Data   struct {
			Status    string  `json:"status"`
			Amount    float64 `json:"amount"`
			Currency  string  `json:"currency"`
			Reference string  `json:"reference"`
			TxRef     string  `json:"tx_ref"`
		} `json:"data"`
	}
	if err := s.chapaRequest(c, http.MethodGet, "/v1/transaction/verify/"+url.PathEscape(txRef), nil, &res); err != nil {
		return "pending", nil // not paid yet (or Chapa unreachable): try again later
	}
	d := res.Data
	switch strings.ToLower(d.Status) {
	case "success":
	case "failed", "cancelled":
		_, err := s.pool.Exec(c, `UPDATE chapa_payments SET status = 'failed', verified_at = now() WHERE tx_ref = $1 AND status = 'pending'`, txRef)
		return "failed", err
	default:
		return "pending", nil
	}
	if d.TxRef != txRef || strings.ToUpper(d.Currency) != "ETB" || d.Amount+0.001 < p.amount {
		return "pending", nil // doesn't match what was asked for; an admin can check
	}
	_, days, err := s.enrollmentPrice(c, p.module)
	if err != nil {
		return "", err
	}
	err = pgx.BeginFunc(c, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(c, `UPDATE chapa_payments SET status = 'success', chapa_ref = $2, verified_at = now() WHERE tx_ref = $1 AND status = 'pending'`, txRef, d.Reference)
		if err != nil || tag.RowsAffected() == 0 {
			return err // already approved by another request
		}
		// A renewal extends from the current end, never shortens it. The
		// "both" plan approves the academic and the training plan.
		for _, module := range planModules(p.module) {
			if _, err := tx.Exec(c, `INSERT INTO enrollment_requests (student_id, module, reference, status, note, expires_at)
				VALUES ($1, $2, $3, 'approved', 'Paid online with Chapa',
					greatest(now(), coalesce((SELECT max(expires_at) FROM enrollment_requests
						WHERE student_id = $1 AND module = $2 AND status = 'approved'), now())) + make_interval(days => $4))`,
				p.studentID, module, "Chapa "+txRef, days); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return "success", nil
}

// GET /payments/chapa/:tx_ref — the app checks after the student pays.
func (s *Server) checkChapaPayment(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	txRef := c.Param("tx_ref")
	var owner string
	if err := s.pool.QueryRow(c, `SELECT student_id FROM chapa_payments WHERE tx_ref = $1`, txRef).Scan(&owner); err != nil || owner != st.ID {
		return httpx.NotFound("Payment")
	}
	status, err := s.verifyChapa(c, txRef)
	if err != nil {
		return storeErr(err, "Payment")
	}
	return httpx.OK(c, gin.H{"status": status})
}

// GET /payments/chapa/callback?trx_ref=… — Chapa's server-to-server call.
func (s *Server) chapaCallback(c *gin.Context) {
	ref := c.Query("trx_ref")
	if ref == "" {
		ref = c.Query("tx_ref")
	}
	if ref != "" && s.cfg.ChapaEnabled() {
		_, _ = s.verifyChapa(c, ref)
	}
	c.Status(http.StatusOK)
}

// POST /payments/chapa/webhook — signed by Chapa. The body only tells us
// which transaction to verify; the verification itself goes to Chapa.
func (s *Server) chapaWebhook(c *gin.Context) {
	raw, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if secret := s.cfg.ChapaWebhookSecret; secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(raw)
		want := hex.EncodeToString(mac.Sum(nil))
		got := c.GetHeader("x-chapa-signature")
		if got == "" {
			got = c.GetHeader("Chapa-Signature")
		}
		if !hmac.Equal([]byte(strings.ToLower(got)), []byte(want)) {
			c.Status(http.StatusUnauthorized)
			return
		}
	}
	var in struct {
		TxRef string `json:"tx_ref"`
	}
	if json.Unmarshal(raw, &in) == nil && in.TxRef != "" && s.cfg.ChapaEnabled() {
		_, _ = s.verifyChapa(c, in.TxRef)
	}
	c.Status(http.StatusOK)
}

// GET /payments/options — whether online payment is on, and the prices.
func (s *Server) paymentOptions(c *gin.Context) error {
	academic, days, err := s.enrollmentPrice(c, "academic")
	if err != nil {
		return err
	}
	training, _, err := s.enrollmentPrice(c, "training")
	if err != nil {
		return err
	}
	both, _, err := s.enrollmentPrice(c, "both")
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{
		"chapa":           s.cfg.ChapaEnabled(),
		"currency":        "ETB",
		"enrollment_days": days,
		"prices":          gin.H{"academic": academic, "training": training, "both": both},
	})
}

// planModules lists the enrollment modules a plan grants.
func planModules(plan string) []string {
	if plan == "both" {
		return []string{"academic", "training"}
	}
	return []string{plan}
}
