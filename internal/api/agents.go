package api

import (
	"crypto/rand"
	"errors"
	"html/template"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/httpx"
)

// Field agents: approved agents get a code and a link (/a/CODE). Students
// who join with it are theirs; each plan those students take earns the
// agent a commission (the agent_commission trigger records it), which the
// main office pays out.

// referralWindow: a code can be added this long after signing up.
const referralWindow = 14 * 24 * time.Hour

// codeAlphabet leaves out look-alikes (0/O, 1/I/L).
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newAgentCode() (string, error) {
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b), nil
}

// ensureAgentCode gives an approved agent a code (once).
func (s *Server) ensureAgentCode(c *gin.Context, agentID string) error {
	for range 5 {
		code, err := newAgentCode()
		if err != nil {
			return err
		}
		_, err = s.pool.Exec(c, `UPDATE field_agents SET code = $2 WHERE id = $1 AND code IS NULL`, agentID, code)
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "duplicate key") {
			return err
		}
	}
	return httpx.NewError(http.StatusConflict, "code_taken", "Couldn't make a unique code; try again.")
}

// agentLink is the shareable link for a code, on this API's host.
func agentLink(c *gin.Context, code string) string {
	scheme := "https"
	if p := c.GetHeader("X-Forwarded-Proto"); p != "" {
		scheme = p
	} else if c.Request.TLS == nil && strings.HasPrefix(c.Request.Host, "localhost") {
		scheme = "http"
	}
	return scheme + "://" + c.Request.Host + "/a/" + code
}

// POST /me/agent-code {code} — the student joined through an agent.
func (s *Server) appUseAgentCode(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in struct {
		Code string `json:"code" binding:"required,max=12"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	var agentID string
	var agentStudent *string
	// Whether the code is the student's own: their account, or the phone
	// number the agent applied with or has now.
	var ownPhone, paidBefore bool
	err = s.pool.QueryRow(c, `SELECT f.id, f.student_id,
			coalesce(phone_key($2) IN (phone_key(f.phone),
				(SELECT phone_key(phone_number) FROM students WHERE id = f.student_id)), false),
			EXISTS (SELECT 1 FROM enrollment_requests WHERE student_id = $3)
		FROM field_agents f WHERE f.code = $1 AND f.status = 'approved'`, code, st.PhoneNumber, st.ID).Scan(&agentID, &agentStudent, &ownPhone, &paidBefore)
	if err == pgx.ErrNoRows {
		return httpx.Field("code", "No agent has this code. Check it and try again.")
	}
	if err != nil {
		return err
	}
	switch {
	case agentStudent != nil && *agentStudent == st.ID, ownPhone:
		return httpx.Field("code", "That's your own code.")
	case paidBefore:
		// The agent earns on students they bring in, before their first plan.
		return httpx.Field("code", "Agent codes can be added only before your first payment.")
	case s.now().Sub(st.CreatedAt) > referralWindow:
		return httpx.Field("code", "Agent codes can be added in the first 14 days after joining.")
	}
	tag, err := s.pool.Exec(c, `INSERT INTO agent_referrals (student_id, agent_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, st.ID, agentID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("already_referred", "You already joined through an agent.")
	}
	return httpx.OK(c, gin.H{"ok": true})
}

type agentStats struct {
	SignUps   int     `json:"sign_ups"`
	Paying    int     `json:"paying"`
	Earned    float64 `json:"earned"`
	Pending   float64 `json:"pending"`
	Paid      float64 `json:"paid"`
	ThisMonth float64 `json:"this_month"`
}

func (s *Server) agentStatsFor(c *gin.Context, agentID string) (agentStats, error) {
	var a agentStats
	err := s.pool.QueryRow(c, `SELECT
			(SELECT count(*) FROM agent_referrals WHERE agent_id = $1)::int,
			(SELECT count(DISTINCT student_id) FROM agent_commissions WHERE agent_id = $1 AND status <> 'cancelled')::int,
			coalesce((SELECT sum(amount) FROM agent_commissions WHERE agent_id = $1 AND status <> 'cancelled'), 0)::float8,
			coalesce((SELECT sum(amount) FROM agent_commissions WHERE agent_id = $1 AND status = 'pending'), 0)::float8,
			coalesce((SELECT sum(amount) FROM agent_commissions WHERE agent_id = $1 AND status = 'paid'), 0)::float8,
			coalesce((SELECT sum(amount) FROM agent_commissions WHERE agent_id = $1 AND status <> 'cancelled'
				AND created_at >= date_trunc('month', now() AT TIME ZONE 'Africa/Addis_Ababa') AT TIME ZONE 'Africa/Addis_Ababa'), 0)::float8`,
		agentID).Scan(&a.SignUps, &a.Paying, &a.Earned, &a.Pending, &a.Paid, &a.ThisMonth)
	return a, err
}

// GET /me/agent — the agent's dashboard (or their application's state).
func (s *Server) appAgent(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var id, status string
	var code *string
	var rate float64
	err = s.pool.QueryRow(c, `SELECT id, status, code, commission_rate::float8 FROM field_agents WHERE student_id = $1`, st.ID).Scan(&id, &status, &code, &rate)
	if err == pgx.ErrNoRows {
		var referred, paid bool
		_ = s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM agent_referrals WHERE student_id = $1),
			EXISTS (SELECT 1 FROM enrollment_requests WHERE student_id = $1)`, st.ID).Scan(&referred, &paid)
		return httpx.OK(c, gin.H{"status": "none", "referred": referred, "can_add_code": !referred && !paid && s.now().Sub(st.CreatedAt) <= referralWindow})
	}
	if err != nil {
		return err
	}
	if status != "approved" {
		return httpx.OK(c, gin.H{"status": status})
	}
	if code == nil {
		if err := s.ensureAgentCode(c, id); err != nil {
			return err
		}
		if err := s.pool.QueryRow(c, `SELECT code FROM field_agents WHERE id = $1`, id).Scan(&code); err != nil {
			return err
		}
	}
	stats, err := s.agentStatsFor(c, id)
	if err != nil {
		return err
	}
	// Recent students: first name only, and whether they're paying.
	recent, err := s.store.ServiceRows(c, `SELECT jsonb_build_object(
			'name', split_part(st.name, ' ', 1), 'joined', r.created_at,
			'earned', coalesce((SELECT sum(amount) FROM agent_commissions m WHERE m.agent_id = r.agent_id AND m.student_id = r.student_id AND m.status <> 'cancelled'), 0))
		FROM agent_referrals r JOIN students st ON st.id = r.student_id
		WHERE r.agent_id = $1 ORDER BY r.created_at DESC LIMIT 30`, id)
	if err != nil {
		return err
	}
	payout, err := s.agentPayout(c, id)
	if err != nil {
		return err
	}
	payouts, err := s.agentPayouts(c, id, 20)
	if err != nil {
		return err
	}
	for i := range payouts {
		payouts[i].PaidBy = "" // the agent sees the academy, not the admin
	}
	return httpx.OK(c, gin.H{"status": "approved", "code": code, "link": agentLink(c, *code), "rate": rate, "stats": stats,
		"recent": recent, "payout": payout, "payouts": payouts})
}

// agentPayoutDetails is where an agent's commission is sent.
type agentPayoutDetails struct {
	Method    string     `json:"method"`
	Bank      string     `json:"bank"`
	Account   string     `json:"account"`
	Name      string     `json:"name"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func (s *Server) agentPayout(c *gin.Context, agentID string) (agentPayoutDetails, error) {
	var p agentPayoutDetails
	err := s.pool.QueryRow(c, `SELECT payout_method, payout_bank, payout_account, payout_name, payout_updated_at
		FROM field_agents WHERE id = $1`, agentID).Scan(&p.Method, &p.Bank, &p.Account, &p.Name, &p.UpdatedAt)
	return p, err
}

var (
	digitsOnly  = regexp.MustCompile(`^[0-9]+$`)
	payoutBanks = map[string]bool{
		"Commercial Bank of Ethiopia": true, "Awash Bank": true, "Bank of Abyssinia": true, "Dashen Bank": true,
		"Hibret Bank": true, "Wegagen Bank": true, "Nib International Bank": true, "Cooperative Bank of Oromia": true,
		"Lion International Bank": true, "Oromia Bank": true, "Zemen Bank": true, "Bunna Bank": true,
		"Berhan Bank": true, "Abay Bank": true, "Addis International Bank": true, "Enat Bank": true,
		"Global Bank Ethiopia": true, "Amhara Bank": true, "Siinqee Bank": true, "Tsehay Bank": true,
		"Ahadu Bank": true, "Gadaa Bank": true, "Hijra Bank": true, "ZamZam Bank": true, "Goh Betoch Bank": true,
		"Shabelle Bank": true, "Rammis Bank": true, "Tsedey Bank": true, "Sidama Bank": true, "Omo Bank": true,
		"Development Bank of Ethiopia": true,
	}
)

// PUT /me/agent/payout {method, bank, account, name} — where the agent is
// paid. Recorded with its time, and in the audit log, so the academy sees
// details changed shortly before a payout.
func (s *Server) appAgentPayout(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in struct {
		Method  string `json:"method" binding:"required,oneof=bank telebirr cbe_birr"`
		Bank    string `json:"bank" binding:"max=80"`
		Account string `json:"account" binding:"required,max=40"`
		Name    string `json:"name" binding:"required,max=120"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	in.Account = strings.Join(strings.Fields(strings.ReplaceAll(in.Account, "-", "")), "")
	in.Name = strings.Join(strings.Fields(in.Name), " ")
	in.Bank = strings.TrimSpace(in.Bank)
	fields := map[string]string{}
	switch in.Method {
	case "bank":
		if !payoutBanks[in.Bank] {
			fields["bank"] = "Choose your bank."
		}
		if !digitsOnly.MatchString(in.Account) || len(in.Account) < 8 || len(in.Account) > 20 {
			fields["account"] = "Enter the account number (8 to 20 digits)."
		}
	default:
		// telebirr and CBE Birr pay to a phone number.
		in.Bank = ""
		if p, ok := ethiopianMobile(in.Account); ok {
			in.Account = p
		} else {
			fields["account"] = "Enter the phone number of the wallet (09… or 07…)."
		}
	}
	if len([]rune(in.Name)) < 3 {
		fields["name"] = "Enter the full name on the account."
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}
	var agentID string
	err = s.pool.QueryRow(c, `UPDATE field_agents SET payout_method = $2, payout_bank = $3, payout_account = $4,
			payout_name = $5, payout_updated_at = now()
		WHERE student_id = $1 AND status = 'approved' RETURNING id`, st.ID, in.Method, in.Bank, in.Account, in.Name).Scan(&agentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.Forbidden("Only approved agents can add payout details.")
	}
	if err != nil {
		return err
	}
	slog.Info("agent payout details changed", "agent", agentID)
	payout, err := s.agentPayout(c, agentID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"payout": payout})
}

// GET /admin/agents — every agent with their numbers.
func (s *Server) adminAgents(c *gin.Context) error {
	rows, err := s.store.ServiceRows(c, `SELECT jsonb_build_object(
			'id', f.id, 'name', f.name, 'phone', f.phone, 'city', f.city, 'region', f.region,
			'status', f.status, 'code', f.code, 'rate', f.commission_rate, 'created_at', f.created_at,
			'sign_ups', (SELECT count(*) FROM agent_referrals r WHERE r.agent_id = f.id),
			'paying', (SELECT count(DISTINCT student_id) FROM agent_commissions m WHERE m.agent_id = f.id AND m.status <> 'cancelled'),
			'earned', coalesce((SELECT sum(amount) FROM agent_commissions m WHERE m.agent_id = f.id AND m.status <> 'cancelled'), 0),
			'pending', coalesce((SELECT sum(amount) FROM agent_commissions m WHERE m.agent_id = f.id AND m.status = 'pending'), 0),
			'payout', jsonb_build_object('method', f.payout_method, 'bank', f.payout_bank, 'account', f.payout_account,
				'name', f.payout_name, 'updated_at', f.payout_updated_at))
		FROM field_agents f ORDER BY (f.status = 'pending') DESC, f.created_at DESC`)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"agents": rows})
}

// PATCH /admin/agents/:id {status?, rate?} — approve (gives the code),
// reject, or change the commission rate.
func (s *Server) adminUpdateAgent(c *gin.Context) error {
	var in struct {
		Status *string  `json:"status" binding:"omitempty,oneof=approved rejected"`
		Rate   *float64 `json:"rate" binding:"omitempty,min=0,max=100"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	tag, err := s.pool.Exec(c, `UPDATE field_agents SET status = coalesce($2, status), commission_rate = coalesce($3, commission_rate) WHERE id = $1`,
		c.Param("id"), in.Status, in.Rate)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("Agent")
	}
	if _, err := s.pool.Exec(c, `INSERT INTO audit_events (admin_id, action, resource) VALUES ($1, 'agents.update', $2)`,
		currentAdmin(c).ID, c.Param("id")); err != nil {
		return err
	}
	if in.Status != nil && *in.Status == "approved" {
		if err := s.ensureAgentCode(c, c.Param("id")); err != nil {
			return err
		}
	}
	return s.adminAgents(c)
}

// POST /admin/agents/:id/payout — marks everything owed as paid.
// POST /admin/agents/:id/payout {reference?} — records a payout of
// everything owed: the amount, where it was sent (the agent's details as
// they are now) and the transfer reference.
func (s *Server) adminAgentPayout(c *gin.Context) error {
	var in struct {
		Reference string `json:"reference" binding:"max=120"`
	}
	if err := httpx.Bind(c, &in); err != nil && c.Request.ContentLength > 0 {
		return err
	}
	admin := currentAdmin(c)
	var payout agentPayoutRecord
	err := pgx.BeginFunc(c, s.pool, func(tx pgx.Tx) error {
		// Lock the agent so two payouts can't settle the same commissions.
		var p agentPayoutDetails
		err := tx.QueryRow(c, `SELECT payout_method, payout_bank, payout_account, payout_name FROM field_agents
			WHERE id = $1 FOR UPDATE`, c.Param("id")).Scan(&p.Method, &p.Bank, &p.Account, &p.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Agent")
		}
		if err != nil {
			return err
		}
		err = tx.QueryRow(c, `WITH owed AS (
				SELECT id, amount FROM agent_commissions WHERE agent_id = $1 AND status = 'pending'),
			payout AS (
				INSERT INTO agent_payouts (agent_id, amount, commissions, method, bank, account, account_name, reference, paid_by)
				SELECT $1, sum(amount), count(*), $2, $3, $4, $5, $6, $7 FROM owed HAVING count(*) > 0
				RETURNING id, amount::float8, commissions, method, bank, account, account_name, reference, created_at),
			settled AS (
				UPDATE agent_commissions m SET status = 'paid', paid_at = now(), payout_id = (SELECT id FROM payout)
				WHERE m.id IN (SELECT id FROM owed) AND EXISTS (SELECT 1 FROM payout))
			SELECT id, amount, commissions, method, bank, account, account_name, reference, created_at FROM payout`,
			c.Param("id"), p.Method, p.Bank, p.Account, p.Name, strings.TrimSpace(in.Reference), admin.ID).
			Scan(&payout.ID, &payout.Amount, &payout.Commissions, &payout.Method, &payout.Bank, &payout.Account,
				&payout.AccountName, &payout.Reference, &payout.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.Conflict("nothing_owed", "Nothing is owed to this agent.")
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(c, `INSERT INTO audit_events (admin_id, action, resource) VALUES ($1, 'agents.payout', $2)`,
			admin.ID, c.Param("id"))
		return err
	})
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"paid": payout.Amount, "payout": payout})
}

// agentPayoutRecord is one payout to an agent.
type agentPayoutRecord struct {
	ID          int64     `json:"id"`
	Amount      float64   `json:"amount"`
	Commissions int       `json:"commissions"`
	Method      string    `json:"method"`
	Bank        string    `json:"bank"`
	Account     string    `json:"account"`
	AccountName string    `json:"account_name"`
	Reference   string    `json:"reference"`
	PaidBy      string    `json:"paid_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Server) agentPayouts(c *gin.Context, agentID string, limit int) ([]agentPayoutRecord, error) {
	rows, err := s.pool.Query(c, `SELECT p.id, p.amount::float8, p.commissions, p.method, p.bank, p.account, p.account_name,
			p.reference, coalesce(a.name, ''), p.created_at
		FROM agent_payouts p LEFT JOIN admins a ON a.id = p.paid_by
		WHERE p.agent_id = $1 ORDER BY p.created_at DESC LIMIT $2`, agentID, limit)
	if err != nil {
		return nil, err
	}
	out := []agentPayoutRecord{}
	for rows.Next() {
		var p agentPayoutRecord
		if err := rows.Scan(&p.ID, &p.Amount, &p.Commissions, &p.Method, &p.Bank, &p.Account, &p.AccountName,
			&p.Reference, &p.PaidBy, &p.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GET /admin/agents/:id — the students an agent brought in (with what each
// earned them) and the agent's payouts.
func (s *Server) adminAgentDetail(c *gin.Context) error {
	id := c.Param("id")
	var exists bool
	if err := s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM field_agents WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return httpx.NotFound("Agent")
	}
	students, err := s.store.ServiceRows(c, `SELECT jsonb_build_object(
			'id', st.id, 'name', st.name, 'phone', st.phone_number, 'grade', st.grade, 'joined', r.created_at,
			'plans', (SELECT count(*) FROM agent_commissions m WHERE m.agent_id = r.agent_id AND m.student_id = r.student_id
				AND m.status <> 'cancelled'),
			'earned', coalesce((SELECT sum(amount) FROM agent_commissions m WHERE m.agent_id = r.agent_id
				AND m.student_id = r.student_id AND m.status <> 'cancelled'), 0),
			'paying', EXISTS (SELECT 1 FROM enrollment_requests e WHERE e.student_id = r.student_id AND e.status = 'approved'
				AND e.expires_at > now()))
		FROM agent_referrals r JOIN students st ON st.id = r.student_id
		WHERE r.agent_id = $1 ORDER BY r.created_at DESC`, id)
	if err != nil {
		return err
	}
	payouts, err := s.agentPayouts(c, id, 100)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"students": students, "payouts": payouts})
}

var agentPage = template.Must(template.New("a").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Join Victory</title>
<style>
:root{--g:#13814f;--ink:#121715;--muted:#71777f;--canvas:#eceeef}
*{box-sizing:border-box}body{margin:0;background:var(--canvas);color:var(--ink);font:16px/1.5 Urbanist,system-ui,-apple-system,Segoe UI,Roboto,sans-serif;display:grid;min-height:100vh;place-items:center;padding:16px}
main{background:#fff;border-radius:28px;padding:28px;max-width:420px;width:100%}
h1{font-size:28px;line-height:1.2;letter-spacing:-.7px;margin:0 0 8px;font-weight:600}
p{color:var(--muted);margin:0 0 16px}
.code{font:600 28px/1 ui-monospace,Menlo,monospace;letter-spacing:.2em;text-align:center;background:#f1f8f4;color:var(--g);border-radius:16px;padding:16px;margin:8px 0 16px}
.btn{display:block;text-align:center;text-decoration:none;border-radius:999px;padding:14px 18px;font-weight:600;margin-top:10px}
.primary{background:var(--g);color:#fff}
small{display:block;color:var(--muted);margin-top:18px;font-size:13px}
</style></head><body><main>
<h1>{{if .Agent}}{{.Agent}} invited you to Victory{{else}}Join Victory{{end}}</h1>
<p>Courses, live exams, past national exams and leaderboards for Grade 9–12, remedial and freshman students.</p>
{{if .Code}}<div class="code">{{.Code}}</div>{{end}}
<a class="btn primary" href="{{.AppLink}}">Open in the Victory app</a>
<small>New to Victory? Install the app, create your account, then enter this code under Profile › Agent code within 14 days of joining, before your first payment.</small>
</main></body></html>`))

// GET /a/:code — the public page behind an agent's link.
func (s *Server) agentLanding(c *gin.Context) {
	code := strings.ToUpper(strings.TrimSpace(c.Param("code")))
	data := struct {
		Agent, Code string
		AppLink     template.URL
	}{AppLink: "victory://open"}
	var name string
	if err := s.pool.QueryRow(c, `SELECT name FROM field_agents WHERE code = $1 AND status = 'approved'`, code).Scan(&name); err == nil {
		data.Agent = strings.Fields(name + " ")[0]
		data.Code = code
		data.AppLink = template.URL("victory://agent?" + url.Values{"code": {code}}.Encode())
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Status(http.StatusOK)
	_ = agentPage.Execute(c.Writer, data)
}

// ethiopianMobile returns a wallet phone number as 09… or 07… (how telebirr
// and CBE Birr show it), from any common way of writing it.
func ethiopianMobile(raw string) (string, bool) {
	m := ethioPhone.FindStringSubmatch(phoneSeparators.ReplaceAllString(strings.TrimSpace(raw), ""))
	if m == nil {
		return "", false
	}
	return "0" + m[1], true
}
