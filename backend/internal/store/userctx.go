package store

import "context"

type userKey struct{}

func WithUser(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, userKey{}, id)
}

func UserID(ctx context.Context) (int, error) {
	id, _ := ctx.Value(userKey{}).(int)
	if id <= 0 {
		return 0, ErrUnauthorized
	}
	return id, nil
}
