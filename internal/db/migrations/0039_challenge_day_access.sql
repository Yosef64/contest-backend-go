-- Challenges for remedial and freshman students too, like exams.
ALTER TABLE challenges DROP CONSTRAINT IF EXISTS challenges_grade_check;
ALTER TABLE challenges ADD CONSTRAINT challenges_grade_check
    CHECK (grade IN ('9', '10', '11', '12', 'remedial', 'freshman'));

-- A day's own access, over the challenge's general rules: '' follows them;
-- 'free' opens the day to everyone; 'referral' opens it with two invites;
-- 'premium' keeps it for paid students.
ALTER TABLE challenge_days ADD COLUMN IF NOT EXISTS access text NOT NULL DEFAULT ''
    CHECK (access IN ('', 'free', 'referral', 'premium'));

-- Invites for one day set to 'referral' (unit = the day).
ALTER TABLE challenge_referrals DROP CONSTRAINT IF EXISTS challenge_referrals_kind_check;
ALTER TABLE challenge_referrals ADD CONSTRAINT challenge_referrals_kind_check
    CHECK (kind IN ('lesson', 'exam', 'final', 'day'));
