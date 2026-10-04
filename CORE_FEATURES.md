# Credential-free core features

Implemented across the Go backend, React admin and Flutter student app.
AI integrations, automated payment gateways, SMS/email delivery and external authentication configuration are excluded from this implementation.

## Start and configure

1. Start the backend with the existing database/environment configuration. Startup applies migrations 0002 through 0005 automatically. Back up a production database before the normal deployment process.
2. Run the admin with its backend API URL configured and demo mode disabled. Use **Courses** for access/import, **Contests > Contest settings** for exams, **Students > Reports** for reporting, **Admins** for roles, and the existing **Settings** icon for other service/account controls.
3. Run the student app in API mode using its existing API URL configuration. Open **Profile > My learning**. Demo mode explains that the service features need the backend; it does not fake approvals or saved learning records.

## Features

| Area | Available behavior |
| --- | --- |
| Academic structure | Entrance, remedial and freshman departments; natural/social streams; school invitation codes; admin-controlled school transfers |
| Access | Course, chapter and contest rules for group/school and free/academic/training access; server-side enforcement and expiring approvals |
| Enrollment | Student payment reference and optional proof URL; manual admin approval/rejection with separate academic and training modules |
| Student analytics | Released-exam chapter accuracy graphs, correct/incorrect counts (including unanswered), recent exam history and deterministic practice prompts |
| Habits and achievements | Complete-day streaks, daily/7-day/30-day task totals, completion badges and anonymous ranks within school/department/stream |
| Learning | Target scores with recorded performance, targeted announcements, 30-day training checklists and admin habit activity |
| Exams | Pre-exam instructions, question navigator with answered/skipped/marked states, question image URLs and mathematical formulas |
| Makeup exams | Per-contest availability/duration, audited closure of new attempts, persistent server timers and resumable active attempts, hidden answers, one submission, existing scoring/results |
| Exam mode | Different hashed entry/exit passwords, backend question/submission gates, app navigation lock and Android pinning integration |
| Results | Live scores/standings hidden until the scheduled exam ends, private school exam results and department/stream leaderboards |
| Referrals | Two distinct authenticated peers unlock enabled chapters; duplicate/self visits rejected; chapter toolbar copies IDs; peers confirm in My learning |
| Content | Atomic JSON lesson import, sanitized HTML/math, question image uploads, batch question tagging/deletion with referenced-question protection |
| Administration | Owner/editor/analyst/school-report manager roles, current-password-verified password changes, audit history |
| Reporting | School, gender, city, region, subject, chapter, department and stream aggregation; accuracy, pass/fail and timing; grade/subject/UTC date filters, matching CSV export and focused report printing |
| Agents | Student application using profile contact information, visible review status and admin approval/rejection |
| Protection | Repeated student identity watermark, Android screenshot protection, encrypted device storage for authorized lesson text with at most a 24-hour lease |

## Practical limits

- Enrollment verification is manual. Proof is a URL/reference, not a dedicated student receipt upload flow.
- Referral confirmation currently uses copied student/chapter IDs, not public web/app invitation links. School invitations use join codes.
- Offline caching covers lesson text and metadata after successful authorized retrieval. It does not download videos/images or queue progress writes. Access changes are checked on reconnect; cached leases expire locally.
- Android screenshot protection is best effort. Strict device kiosk enforcement requires managed Android devices; ordinary screen pinning can be exited through OS controls. iOS kiosk/screenshot prevention is not implemented.
- Existing public media URLs are not converted to private signed URLs or full DRM.
- Existing external Google/Telegram production login configuration still needs its own credentials. Development login must remain limited to development deployments.
- Existing sessions are not revoked by a password change. There is no new email/SMS password-reset service.
- Login streaks, separate topic tagging, automated school invitation links and richer analytics remain future work; this document describes the implemented core batch, not completion of every optional guideline item.

## Verification

- Backend: `go vet ./...` and `go test ./...`.
- Backend integration: `python scripts/core_services_check.py http://127.0.0.1:<test-port>` against a disposable development database only. The script creates fixtures and uses its documented test owner credentials; never run it against production.
- Admin: TypeScript compilation, ESLint and production build (existing fast-refresh/bundle-size warnings remain).
- Flutter: analyzer, cache/math/widget tests, golden screen tests, and the real API data-layer test. Set API_BASE_URL, ADMIN_EMAIL and ADMIN_PASSWORD for `flutter test test/api_integration_test.dart`.

## Analytics semantics

Chapter reports group by question subject and chapter, and count each exam/chapter assessment separately for pass/fail. Exam duration remains the full exam duration, not time attributed to a chapter. Student charts/history and chapter reports exclude exams until their scheduled end. Habit ranks expose only the current student rank and cohort size; ties share rank. Streaks require all tasks checked and preserve yesterday's streak while today is unfinished. Badge awards reflect saved habit logs, not device activity. No database migration or external credentials are needed for this batch.
