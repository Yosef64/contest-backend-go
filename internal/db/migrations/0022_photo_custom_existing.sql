-- Pictures students uploaded before 0021 are their own choice: keep them.
UPDATE students SET photo_custom = true
WHERE photo_url <> '' AND photo_url NOT LIKE 'https://lh3.googleusercontent.com/%';
