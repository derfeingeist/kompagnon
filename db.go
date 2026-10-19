package main

import "database/sql"

func migrate(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    password_salt BLOB NOT NULL,
    password_hash BLOB NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS households (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS memberships (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    household_id TEXT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'member')),
    joined_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS memberships_household_idx ON memberships(household_id);
CREATE TABLE IF NOT EXISTS invites (
    token_hash BLOB PRIMARY KEY,
    household_id TEXT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    created_by TEXT NOT NULL REFERENCES users(id),
    expires_at INTEGER NOT NULL,
    used_at INTEGER
);
CREATE TABLE IF NOT EXISTS sessions (
    token_hash BLOB PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expires_at);
CREATE TABLE IF NOT EXISTS recipes (
    id TEXT PRIMARY KEY,
    household_id TEXT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    instructions TEXT NOT NULL DEFAULT '',
    servings INTEGER NOT NULL CHECK (servings > 0),
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS recipes_household_idx ON recipes(household_id);
CREATE TABLE IF NOT EXISTS recipe_ingredients (
    id TEXT PRIMARY KEY,
    recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    amount TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS meal_plans (
    id TEXT PRIMARY KEY,
    household_id TEXT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    planned_date TEXT NOT NULL,
    servings INTEGER NOT NULL CHECK (servings > 0),
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS meal_plans_week_idx ON meal_plans(household_id, planned_date);
CREATE TABLE IF NOT EXISTS shopping_items (
    id TEXT PRIMARY KEY,
    household_id TEXT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    week_start TEXT NOT NULL,
    name TEXT NOT NULL,
    amount TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    checked INTEGER NOT NULL DEFAULT 0 CHECK (checked IN (0, 1)),
    source TEXT NOT NULL CHECK (source IN ('manual', 'generated')),
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS shopping_items_week_idx ON shopping_items(household_id, week_start);
`
	_, err := db.Exec(schema)
	return err
}
