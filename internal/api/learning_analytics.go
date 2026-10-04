package api

import "github.com/gin-gonic/gin"

// Only released exams contribute; no answer keys or peer identities are exposed.
func (s *Server) learningAnalytics(c *gin.Context, studentID string) (gin.H, error) {
	out := gin.H{}
	queries := map[string]string{
		"chapters": `SELECT jsonb_build_object('subject',q.subject,'chapter',coalesce(nullif(q.chapter,''),'Uncategorized'),
 'total',count(*),'correct',count(*) FILTER(WHERE sub.answers->>q.id=q.answer_index::text),
 'incorrect',count(*) FILTER(WHERE sub.answers->>q.id IS DISTINCT FROM q.answer_index::text),
 'accuracy',round(count(*) FILTER(WHERE sub.answers->>q.id=q.answer_index::text)::numeric*100/nullif(count(*),0),1))
 FROM submissions sub JOIN contests co ON co.id=sub.contest_id JOIN contest_questions cq ON cq.contest_id=co.id JOIN questions q ON q.id=cq.question_id
 WHERE sub.student_id=$1 AND co.end_time<now() GROUP BY q.subject,q.chapter ORDER BY q.subject,q.chapter`,
		"history": `SELECT jsonb_build_object('title',co.title,'subject',co.subject,'submitted_at',sub.submitted_at,'correct',sub.correct,'total',sub.total,
 'accuracy',round(sub.correct::numeric*100/nullif(sub.total,0),1),'time_seconds',sub.time_seconds)
 FROM submissions sub JOIN contests co ON co.id=sub.contest_id WHERE sub.student_id=$1 AND co.end_time<now() ORDER BY sub.submitted_at DESC LIMIT 20`,
		"habits": `WITH daily AS (
 SELECT l.student_id,l.program_id,l.day,cardinality(l.checked) AS checked,cardinality(p.tasks) AS tasks
 FROM habit_logs l JOIN habit_programs p ON p.id=l.program_id WHERE l.day<=(now() AT TIME ZONE 'Africa/Addis_Ababa')::date),
 scores AS (
 SELECT e.student_id,e.program_id,p.title,p.active,
 count(d.day) FILTER(WHERE d.checked>0) AS active_days,
 count(d.day) FILTER(WHERE d.checked=d.tasks) AS complete_days,
 coalesce(sum(d.checked),0) AS checked,
 coalesce(sum(d.checked) FILTER(WHERE d.day>=(now() AT TIME ZONE 'Africa/Addis_Ababa')::date-6),0) AS week_checked,
 coalesce(sum(d.checked) FILTER(WHERE d.day>=(now() AT TIME ZONE 'Africa/Addis_Ababa')::date-29),0) AS month_checked,
 coalesce(sum(d.checked) FILTER(WHERE d.day=(now() AT TIME ZONE 'Africa/Addis_Ababa')::date),0) AS today_checked
 FROM habit_enrollments e JOIN habit_programs p ON p.id=e.program_id LEFT JOIN daily d ON d.student_id=e.student_id AND d.program_id=e.program_id
 GROUP BY e.student_id,e.program_id,p.title,p.active),
 ranked AS (
 SELECT a.*,rank() OVER(PARTITION BY a.program_id,ap.department,ap.stream,ap.school_id ORDER BY a.checked DESC) AS rank,
 count(*) OVER(PARTITION BY a.program_id,ap.department,ap.stream,ap.school_id) AS peers
 FROM scores a LEFT JOIN academic_profiles ap ON ap.student_id=a.student_id)
 SELECT jsonb_build_object('program_id',r.program_id,'title',r.title,'active_days',r.active_days,'complete_days',r.complete_days,
 'today_checked',r.today_checked,'week_checked',r.week_checked,'month_checked',r.month_checked,'rank',r.rank,'peers',r.peers,
 'streak',coalesce((SELECT max(n) FROM (
 SELECT row_number() OVER(ORDER BY d.day DESC) AS n,d.day FROM daily d
 WHERE d.student_id=$1 AND d.program_id=r.program_id AND d.checked=d.tasks) x
 WHERE x.day=((now() AT TIME ZONE 'Africa/Addis_Ababa')::date - CASE WHEN EXISTS(SELECT 1 FROM daily t WHERE t.student_id=$1 AND t.program_id=r.program_id AND t.day=(now() AT TIME ZONE 'Africa/Addis_Ababa')::date AND t.checked=t.tasks) THEN 0 ELSE 1 END)-(x.n::int-1)),0))
 FROM ranked r WHERE r.student_id=$1 ORDER BY r.title`,
	}
	for key, query := range queries {
		rows, err := s.store.ServiceRows(c, query, studentID)
		if err != nil {
			return nil, err
		}
		out[key] = rows
	}
	return out, nil
}
