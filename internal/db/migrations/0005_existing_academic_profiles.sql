INSERT INTO academic_profiles(student_id) SELECT id FROM students ON CONFLICT DO NOTHING;
