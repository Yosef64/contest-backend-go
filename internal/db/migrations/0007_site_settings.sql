-- Official channels and the About text shown in the app, edited by admins.
CREATE TABLE site_settings (
    key        text PRIMARY KEY,
    value      text        NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO site_settings (key, value) VALUES
    ('telegram_bot', 'https://t.me/VictoryLearningBot'),
    ('about', 'Victory Academy prepares students for the Grade 12 national entrance exam and supports remedial and freshman students across the Natural and Social Science streams. Read structured notes, take timed contests and exams, follow your analytics and build study habits with our training programs.')
ON CONFLICT (key) DO NOTHING;
