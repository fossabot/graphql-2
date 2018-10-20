package graphql

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// User is a database object based off of what we get back from Google OAuth.
type User struct {
	ID       string
	Role     string
	Created  time.Time
	Modified time.Time
}

// Save is an upsert based operation for User.
func (u *User) Save(ctx context.Context) error {
	_, err := db.ExecContext(ctx,
		`
    INSERT INTO users (id, role, created_at, modified_at)
    VALUES ($1, $2, $3, $4)
    ON CONFLICT (id) DO UPDATE
    SET (role, modified_at) = ($2, $4)
    WHERE users.id = $1;`,
		sanitize(u.ID),
		sanitize(u.Role),
		u.Created,
		time.Now())

	return err
}

// GetUser returns a user from the database. If the User does not exist, we
// create it.
func GetUser(ctx context.Context, id string) (*User, error) {
	var user User
	id = sanitize(id);
	row := db.QueryRowContext(ctx, "SELECT id, role, created_at, modified_at FROM users WHERE id = $1", id)
	err := row.Scan(&user.ID, &user.Role, &user.Created, &user.Modified)

	switch {
	case err == sql.ErrNoRows:
		user.ID = id
		user.Role = "normal"
		user.Created = time.Now()
		user.Modified = time.Now()
		return &user, (&user).Save(ctx)
	case err != nil:
		return nil, fmt.Errorf("Error running get query: %+v", err)
	default:
		return &user, (&user).Save(ctx)
	}
}
