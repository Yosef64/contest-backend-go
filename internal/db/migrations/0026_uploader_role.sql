-- Question uploaders: staff who add questions and pictures only, and can't
-- change their own password (the owner sets and resets it).
ALTER TABLE admins DROP CONSTRAINT IF EXISTS admins_permission_role_check;
ALTER TABLE admins ADD CONSTRAINT admins_permission_role_check
    CHECK (permission_role IN ('editor', 'analyst', 'manager', 'uploader'));
