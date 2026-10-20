package store

import (
	"context"
	"os"
	"testing"

	"forking-paths/internal/db"
)

func TestPlazaHonorsPrivacy(t *testing.T) {
	pool, err := db.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var id int
	if err := pool.QueryRow(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Skip(err)
	}
	own := WithUser(ctx, id)
	st := New(pool)
	if err := st.SetGardenPublic(own, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.SetGardenPublic(own, false) })

	people, err := st.Plaza(own)
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range people {
		if person.ID == id {
			t.Fatal("private garden appeared in the plaza")
		}
		if len(person.Entries) > 4 {
			t.Fatalf("preview has %d entries", len(person.Entries))
		}
	}
	if _, err := st.PlazaOne(own, id); err != ErrNotFound {
		t.Fatalf("private garden: %v", err)
	}

	if err := st.SetGardenPublic(own, true); err != nil {
		t.Fatal(err)
	}
	people, err = st.Plaza(own)
	if err != nil {
		t.Fatal(err)
	}
	var found *PlazaPerson
	for i := range people {
		if people[i].ID == id {
			found = &people[i]
		}
		if len(people[i].Entries) > 4 {
			t.Fatalf("preview has %d entries", len(people[i].Entries))
		}
		for _, entry := range people[i].Entries {
			if entry.Notes != "" {
				t.Fatal("notes leaked")
			}
		}
		prev := int(^uint(0) >> 1)
		for _, entry := range people[i].Entries {
			if entry.SpendMinutes > prev {
				t.Fatal("preview is not ordered by time")
			}
			prev = entry.SpendMinutes
		}
	}
	if found == nil {
		t.Fatal("public garden missing")
	}
	full, err := st.PlazaOne(own, id)
	if err != nil {
		t.Fatal(err)
	}
	if full.Total < len(found.Entries) || (full.Total > 4 && len(found.Entries) != 4) {
		t.Fatalf("preview %d of %d", len(found.Entries), full.Total)
	}
	if len(full.Entries) != full.Total {
		t.Fatalf("full list %d of %d", len(full.Entries), full.Total)
	}
}
