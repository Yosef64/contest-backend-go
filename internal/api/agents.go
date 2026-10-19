package api

import (
	"crypto/rand"
	"html/template"
	"math/big"
	"net/http"
	"net/url"
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
	return httpx.OK(c, gin.H{"status": "approved", "code": code, "link": agentLink(c, *code), "rate": rate, "stats": stats, "recent": recent})
}

// GET /admin/agents — every agent with their numbers.
func (s *Server) adminAgents(c *gin.Context) error {
	rows, err := s.store.ServiceRows(c, `SELECT jsonb_build_object(
			'id', f.id, 'name', f.name, 'phone', f.phone, 'city', f.city, 'region', f.region,
			'status', f.status, 'code', f.code, 'rate', f.commission_rate, 'created_at', f.created_at,
			'sign_ups', (SELECT count(*) FROM agent_referrals r WHERE r.agent_id = f.id),
			'paying', (SELECT count(DISTINCT student_id) FROM agent_commissions m WHERE m.agent_id = f.id AND m.status <> 'cancelled'),
			'earned', coalesce((SELECT sum(amount) FROM agent_commissions m WHERE m.agent_id = f.id AND m.status <> 'cancelled'), 0),
			'pending', coalesce((SELECT sum(amount) FROM agent_commissions m WHERE m.agent_id = f.id AND m.status = 'pending'), 0))
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
func (s *Server) adminAgentPayout(c *gin.Context) error {
	var paid float64
	if err := s.pool.QueryRow(c, `WITH p AS (
			UPDATE agent_commissions SET status = 'paid', paid_at = now()
			WHERE agent_id = $1 AND status = 'pending' RETURNING amount)
		SELECT coalesce(sum(amount), 0)::float8 FROM p`, c.Param("id")).Scan(&paid); err != nil {
		return err
	}
	if _, err := s.pool.Exec(c, `INSERT INTO audit_events (admin_id, action, resource) VALUES ($1, 'agents.payout', $2)`,
		currentAdmin(c).ID, c.Param("id")); err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"paid": paid})
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
