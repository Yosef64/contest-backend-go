CREATE TABLE schools (
 id text PRIMARY KEY DEFAULT gen_random_uuid()::text, name text NOT NULL,
 city text NOT NULL DEFAULT '', region text NOT NULL DEFAULT '',
 join_code text NOT NULL UNIQUE DEFAULT replace(gen_random_uuid()::text,'-',''), created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE academic_profiles (
 student_id text PRIMARY KEY REFERENCES students(id) ON DELETE CASCADE,
 department text NOT NULL DEFAULT 'entrance' CHECK(department IN ('entrance','remedial','freshman')),
 stream text NOT NULL DEFAULT 'natural' CHECK(stream IN ('natural','social')),
 school_id text REFERENCES schools(id), updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE learning_policies (
 resource_type text NOT NULL CHECK(resource_type IN ('course','chapter','contest')),
 resource_id text NOT NULL, access text NOT NULL DEFAULT 'free' CHECK(access IN ('free','academic','training')),
 department text NOT NULL DEFAULT '' CHECK(department IN ('','entrance','remedial','freshman')),
 stream text NOT NULL DEFAULT '' CHECK(stream IN ('','natural','social')),
 school_id text REFERENCES schools(id), referral_enabled boolean NOT NULL DEFAULT false,
 PRIMARY KEY(resource_type,resource_id));
CREATE TABLE enrollment_requests (
 id text PRIMARY KEY DEFAULT gen_random_uuid()::text, student_id text NOT NULL REFERENCES students(id) ON DELETE CASCADE,
 module text NOT NULL CHECK(module IN ('academic','training')), reference text NOT NULL, proof_url text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 note text NOT NULL DEFAULT '', expires_at timestamptz, reviewed_by text REFERENCES admins(id), created_at timestamptz NOT NULL DEFAULT now());
CREATE UNIQUE INDEX enrollment_pending ON enrollment_requests(student_id,module) WHERE status='pending';
CREATE TABLE announcements (
 id text PRIMARY KEY DEFAULT gen_random_uuid()::text, title text NOT NULL, body text NOT NULL,
 department text NOT NULL DEFAULT '', school_id text REFERENCES schools(id), active boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(), author_id text REFERENCES admins(id));
CREATE TABLE score_targets (
 student_id text NOT NULL REFERENCES students(id) ON DELETE CASCADE, subject text NOT NULL,
 target integer NOT NULL CHECK(target BETWEEN 1 AND 100), PRIMARY KEY(student_id,subject));
CREATE TABLE habit_programs (
 id text PRIMARY KEY DEFAULT gen_random_uuid()::text, title text NOT NULL, tasks text[] NOT NULL CHECK(cardinality(tasks) BETWEEN 1 AND 20),
 active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE habit_enrollments (
 student_id text REFERENCES students(id) ON DELETE CASCADE, program_id text REFERENCES habit_programs(id) ON DELETE CASCADE,
 started_on date NOT NULL DEFAULT (now() AT TIME ZONE 'Africa/Addis_Ababa')::date, PRIMARY KEY(student_id,program_id));
CREATE TABLE habit_logs (
 student_id text NOT NULL, program_id text NOT NULL, day date NOT NULL, checked integer[] NOT NULL DEFAULT '{}',
 PRIMARY KEY(student_id,program_id,day), FOREIGN KEY(student_id,program_id) REFERENCES habit_enrollments(student_id,program_id) ON DELETE CASCADE);
CREATE TABLE referral_visits (
 owner_id text REFERENCES students(id) ON DELETE CASCADE, visitor_id text REFERENCES students(id) ON DELETE CASCADE,
 chapter_id text REFERENCES chapters(id) ON DELETE CASCADE, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner_id,visitor_id,chapter_id), CHECK(owner_id<>visitor_id));
CREATE TABLE audit_events (
 id bigserial PRIMARY KEY, admin_id text REFERENCES admins(id) ON DELETE SET NULL, action text NOT NULL,
 resource text NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
ALTER TABLE admins ADD COLUMN permission_role text NOT NULL DEFAULT 'editor' CHECK(permission_role IN ('editor','analyst','manager'));
ALTER TABLE admins ADD COLUMN school_id text REFERENCES schools(id);
