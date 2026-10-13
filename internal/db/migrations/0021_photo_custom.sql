-- Profile pictures start as the sign-in picture (Google) and follow it on
-- each sign-in, until the student uploads or removes their own.
ALTER TABLE students ADD COLUMN photo_custom boolean NOT NULL DEFAULT false;
