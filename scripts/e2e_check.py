"""End-to-end API check against a running server (development mode).

    python scripts/e2e_check.py http://localhost:8080 admin@victory.local <password>

Creates a contest and a dev student, exercises the admin and app APIs, and
deletes the contest afterwards. Prints PASS/FAIL per check.
"""
import json
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone
from http.cookiejar import CookieJar

BASE, EMAIL, PASSWORD = sys.argv[1], sys.argv[2], sys.argv[3]
jar = CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
failures = 0


def call(method, path, body=None, token=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with opener.open(req) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else {})
    except urllib.error.HTTPError as e:
        raw = e.read()
        return e.code, (json.loads(raw) if raw else {})


def check(name, cond, detail=""):
    global failures
    print(("PASS " if cond else "FAIL ") + name + ("" if cond else f"  -> {detail}"))
    if not cond:
        failures += 1


iso = lambda d: d.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")
now = datetime.now(timezone.utc)

# --- Admin -------------------------------------------------------------------
s, r = call("POST", "/api/v1/admin/auth/login", {"email": EMAIL, "password": PASSWORD})
check("admin login", s == 200 and r["admin"]["is_owner"], r)
s, r = call("GET", "/api/v1/admin/auth/me")
check("admin session cookie works", s == 200 and r["admin"]["email"] == EMAIL, r)
s, r = call("GET", "/api/v1/admin/admins")
check("admin list hides password hashes", s == 200 and all("password" not in json.dumps(a) for a in r["admins"]), r)

s, r = call("GET", "/api/v1/admin/courses?grade=9")
check("grade 9 has 5 seeded courses", s == 200 and len(r["courses"]) == 5, r)
math9 = next(c for c in r["courses"] if c["subject"] == "Mathematics")
s, r = call("GET", f"/api/v1/admin/courses/{math9['id']}")
ch = r["chapters"][0]
check("course detail lists chapters in order", s == 200 and [c["number"] for c in r["chapters"]] == [1, 2, 3], r)

s, r = call("GET", f"/api/v1/admin/chapters/{ch['id']}")
chapter = r["chapter"]
check("chapter keeps SVG diagram", "<svg" in chapter["html"] and "viewBox" in chapter["html"], chapter["html"][:200])
s, r = call("POST", f"/api/v1/admin/chapters/{ch['id']}/lock")
check("acquire edit lock", s == 200 and r["lock"]["admin_name"], r)
s, r = call("PUT", f"/api/v1/admin/chapters/{ch['id']}", {
    "html": chapter["html"] + '<p onclick="x()">E2E edit<script>alert(1)</script></p>',
    "base_version": chapter["version"], "snapshot": True})
check("save chapter (sanitized, new version)", s == 200 and "script" not in r["chapter"]["html"] and "onclick" not in r["chapter"]["html"]
      and r["chapter"]["version"] == chapter["version"] + 1, r)
s, r = call("PUT", f"/api/v1/admin/chapters/{ch['id']}", {"html": "<p>stale</p>", "base_version": chapter["version"]})
check("stale save rejected (409 version_conflict)", s == 409 and r["error"]["code"] == "version_conflict", r)
s, r = call("GET", f"/api/v1/admin/chapters/{ch['id']}/versions")
check("version history recorded", s == 200 and len(r["versions"]) >= 2, r)
s, r = call("POST", f"/api/v1/admin/chapters/{ch['id']}/versions/1/restore")
check("restore version 1", s == 200 and "E2E edit" not in r["chapter"]["html"], r)
s, r = call("POST", f"/api/v1/admin/chapters/{ch['id']}/comments", {"body": "E2E comment", "quote": "linear"})
check("add comment", s == 201 and r["comment"]["body"] == "E2E comment", r)
call("PATCH", f"/api/v1/admin/comments/{r['comment']['id']}", {"resolved": True})
call("DELETE", f"/api/v1/admin/chapters/{ch['id']}/lock")

s, r = call("GET", "/api/v1/admin/questions?subject=Mathematics")
qids = [q["id"] for q in r["questions"][:3]]
answers = {q["id"]: q["answer_index"] for q in r["questions"][:3]}
check("question bank filter", s == 200 and len(qids) == 3, r)

s, r = call("POST", "/api/v1/admin/contests", {"title": "E2E", "subject": "Mathematics", "grade": "9",
            "start_time": iso(now), "end_time": iso(now - timedelta(minutes=5)), "question_ids": qids})
check("contest validation: end before start", s == 422 and "end_time" in r["error"].get("fields", {}), r)
s, r = call("POST", "/api/v1/admin/contests", {"title": "E2E Live Contest", "subject": "Mathematics", "grade": "9",
            "start_time": iso(now - timedelta(minutes=1)), "end_time": iso(now + timedelta(minutes=30)),
            "prize": "Bragging rights", "question_ids": qids})
check("create live contest", s == 201 and r["contest"]["phase"] == "active", r)
contest = r["contest"]
cid = contest["id"]

# --- Student app -------------------------------------------------------------
RUN = str(int(time.time()))
s, r = call("POST", "/api/v1/auth/dev", {"id": "e2e-" + RUN, "name": "E2E Student " + RUN})
token = r.get("token")
check("dev sign-in issues token", s == 200 and token, r)
s, r = call("GET", "/api/v1/contests", token=token)
check("app requires profile first", s == 404 and r["error"]["code"] == "profile_required", r)
s, r = call("PUT", "/api/v1/me", {"name": "E2E Student " + RUN, "phone_number": "0911000000", "gender": "female", "age": 15,
            "school": "Test School", "grade": "1", "city": "Adama", "region": "Oromia",
            "department": "entrance", "stream": "natural"}, token=token)
check("profile created; legacy grade '1' normalised to '9'", s == 200 and r["student"]["grade"] == "9", r)
s, r = call("PUT", "/api/v1/me", {"name": "E"}, token=token)
check("profile validation errors are per field", s == 422 and "phone_number" in r["error"]["fields"], r)

s, r = call("GET", "/api/v1/courses", token=token)
check("app courses for my grade include chapters", s == 200 and len(r["courses"]) == 5 and r["courses"][0]["chapters"], r)
s, r = call("PUT", f"/api/v1/me/progress/{ch['id']}", {"completed": True}, token=token)
s2, r2 = call("GET", "/api/v1/me/progress", token=token)
check("chapter progress saved", s == 204 and ch["id"] in r2["completed_chapter_ids"], (s, r, r2))

s, r = call("GET", f"/api/v1/contests/{cid}/questions", token=token)
check("questions blocked before registering", s == 403, r)
s, r = call("POST", f"/api/v1/contests/{cid}/register", {"subjects": ["Mathematics"], "experience": "beginner"}, token=token)
check("register", s == 204, r)
s, r = call("GET", f"/api/v1/contests/{cid}/questions", token=token)
check("questions served without answers", s == 200 and len(r["questions"]) == 3 and all("answer_index" not in q and "explanation" not in q for q in r["questions"]), r)

my = {qids[0]: answers[qids[0]], qids[1]: (answers[qids[1]] + 1) % 4, "not-a-question": 0}
s, r = call("POST", f"/api/v1/contests/{cid}/submit", {"answers": my, "time_seconds": 99999}, token=token)
check("submit: score hidden until contest ends", s == 201 and r["submission"]["answered"] == 2 and "correct" not in r["submission"], r)
s, r = call("POST", f"/api/v1/contests/{cid}/submit", {"answers": my, "time_seconds": 10}, token=token)
check("second submission rejected", s == 409 and r["error"]["code"] == "already_submitted", r)
s, r = call("GET", f"/api/v1/contests/{cid}/editorial", token=token)
check("editorial locked while live", s == 409 and r["error"]["code"] == "not_ended", r)

# End the contest (admin), then check results.
s, r = call("PUT", f"/api/v1/admin/contests/{cid}", {"title": contest["title"], "subject": "Mathematics", "grade": "9",
            "start_time": iso(now - timedelta(hours=2)), "end_time": iso(now - timedelta(hours=1)), "prize": "", "question_ids": qids})
check("admin can reschedule (same questions after submissions)", s == 200 and r["contest"]["phase"] == "ended", r)
s, r = call("PUT", f"/api/v1/admin/contests/{cid}", {"title": contest["title"], "subject": "Mathematics", "grade": "9",
            "start_time": iso(now - timedelta(hours=2)), "end_time": iso(now - timedelta(hours=1)), "prize": "", "question_ids": qids[:2]})
check("questions locked once students submitted", s == 422, r)
s, r = call("GET", f"/api/v1/contests/{cid}/editorial", token=token)
check("editorial shows answers after end", s == 200 and r["answers"].get(qids[0]) == answers[qids[0]] and "answer_index" in r["questions"][0], r)
s, r = call("GET", f"/api/v1/contests/{cid}", token=token)
check("server-side score: 1 of 3 correct", s == 200 and r["contest"]["submission"]["correct"] == 1 and r["contest"]["submission"]["total"] == 3, r)
check("time_seconds clamped to contest length", r["contest"]["submission"]["time_seconds"] <= 3600, r)
s, r = call("GET", f"/api/v1/contests/{cid}/standings", token=token)
check("standings include the student", s == 200 and r["standings"][0]["name"] == "E2E Student " + RUN, r)
s, r = call("GET", "/api/v1/leaderboard", token=token)
check("all-time leaderboard includes me", s == 200 and r["me"] and r["me"]["correct"] == 1, r)
s, r = call("GET", "/api/v1/me/stats", token=token)
check("my stats", s == 200 and r["stats"]["contests"] == 1 and r["stats"]["chapters_completed"] == 1, r)

s, r = call("GET", "/api/v1/admin/overview")
check("admin overview", s == 200 and r["totals"]["courses"] == 20 and r["totals"]["students"] >= 1, r)
s, r = call("GET", "/api/v1/admin/students?q=E2E%20Student%20" + RUN)
sid = r["items"][0]["id"] if r.get("items") else None
check("admin student search", s == 200 and sid == "dev_e2e-" + RUN, r)
s, r = call("GET", f"/api/v1/admin/students/{sid}")
check("admin student detail", s == 200 and r["stats"]["contests"] == 1, r)
call("PATCH", f"/api/v1/admin/students/{sid}", {"is_suspended": True})
s, r = call("GET", "/api/v1/contests", token=token)
check("suspended student blocked", s == 403, r)
call("PATCH", f"/api/v1/admin/students/{sid}", {"is_suspended": False})

s, r = call("DELETE", f"/api/v1/admin/questions/{qids[0]}")
check("question used by a contest can't be deleted", s == 409 and r["error"]["code"] == "in_use", r)

# --- Cleanup -------------------------------------------------------------------
s, _ = call("DELETE", f"/api/v1/admin/contests/{cid}")
check("cleanup: delete e2e contest", s == 204)
s, _ = call("DELETE", f"/api/v1/admin/students/{sid}")
check("cleanup: delete e2e student", s == 204)
s, _ = call("POST", "/api/v1/admin/auth/logout")
s, r = call("GET", "/api/v1/admin/overview")
check("logout ends the session", s == 401, r)

print(f"\n{'ALL CHECKS PASSED' if failures == 0 else str(failures) + ' CHECK(S) FAILED'}")
sys.exit(1 if failures else 0)
