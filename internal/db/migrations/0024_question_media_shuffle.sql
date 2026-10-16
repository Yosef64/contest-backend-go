-- Pictures on answer options and on the explanation, and per-student
-- shuffling of an exam's questions and options.
ALTER TABLE questions
    -- One entry per option ('' for none); empty array when no option has one.
    ADD COLUMN IF NOT EXISTS option_images text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS explanation_image_url text NOT NULL DEFAULT '';

ALTER TABLE contests
    ADD COLUMN IF NOT EXISTS shuffle_questions boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS shuffle_options boolean NOT NULL DEFAULT true;
