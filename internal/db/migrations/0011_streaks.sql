-- Daily streaks: a day counts when the student opens the app or studies
-- (chapter completed, exam submitted, habit logged, study session done).
-- Opening the app is the only signal not already stored elsewhere.
CREATE TABLE IF NOT EXISTS student_activity (
    student_id text NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    day        date NOT NULL,
    PRIMARY KEY (student_id, day)
);
