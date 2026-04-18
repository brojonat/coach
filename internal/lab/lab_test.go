package lab

import (
	"context"
	"errors"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := openTestStore(t)
	// Running migrate a second time should be a no-op.
	if err := s.migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	// Schema version should equal len(migrations) and not duplicate.
	var count, max int
	if err := s.DB().QueryRow(`SELECT COUNT(*), COALESCE(MAX(version), 0) FROM schema_version`).Scan(&count, &max); err != nil {
		t.Fatal(err)
	}
	if count != len(migrations) || max != len(migrations) {
		t.Fatalf("expected %d migrations recorded once (max %d), got count=%d max=%d",
			len(migrations), len(migrations), count, max)
	}
}

func TestSeedTasksIdempotent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	inserted, skipped, err := s.SeedTasks(ctx)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if inserted != len(starterTasks) || skipped != 0 {
		t.Fatalf("first seed: want inserted=%d skipped=0, got inserted=%d skipped=%d",
			len(starterTasks), inserted, skipped)
	}

	inserted2, skipped2, err := s.SeedTasks(ctx)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if inserted2 != 0 || skipped2 != len(starterTasks) {
		t.Fatalf("second seed: want inserted=0 skipped=%d, got inserted=%d skipped=%d",
			len(starterTasks), inserted2, skipped2)
	}
}

func TestListTasksTagFilter(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, _, err := s.SeedTasks(ctx); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListTasks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(starterTasks) {
		t.Fatalf("all: got %d want %d", len(all), len(starterTasks))
	}

	dangerous, err := s.ListTasks(ctx, "dangerous")
	if err != nil {
		t.Fatal(err)
	}
	if len(dangerous) == 0 {
		t.Fatal("expected at least one dangerous task in the seed corpus")
	}
	for _, task := range dangerous {
		if !hasTag(task.Tags, "dangerous") {
			t.Fatalf("task %q returned for tag=dangerous but tags=%v", task.Slug, task.Tags)
		}
	}

	none, err := s.ListTasks(ctx, "nope-this-doesnt-exist")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("unknown tag should return 0, got %d", len(none))
	}
}

func TestGetTask(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, _, err := s.SeedTasks(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTask(ctx, "find-large-files")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Slug != "find-large-files" || got.Difficulty != "intermediate" {
		t.Fatalf("unexpected task: %+v", got)
	}
	if len(got.Tags) == 0 || got.Tags[0] != "files" {
		t.Fatalf("expected files tag, got %v", got.Tags)
	}

	_, err = s.GetTask(ctx, "definitely-not-a-slug")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestListTasksOrderedByDifficulty(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, _, err := s.SeedTasks(ctx); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListTasks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	rank := map[string]int{"beginner": 0, "intermediate": 1, "advanced": 2}
	prev := -1
	for _, task := range all {
		r, ok := rank[task.Difficulty]
		if !ok {
			t.Fatalf("unknown difficulty %q on %s", task.Difficulty, task.Slug)
		}
		if r < prev {
			t.Fatalf("task %s out of order (difficulty=%s)", task.Slug, task.Difficulty)
		}
		prev = r
	}
}

func TestEncodeDecodeTags(t *testing.T) {
	// encode sorts + joins; decode splits + trims. Round-trip should normalize.
	raw := encodeTags([]string{" git ", "files", "", "git"})
	if raw != "files,git,git" {
		t.Fatalf("encode: got %q", raw)
	}
	got := decodeTags("  files , git ,")
	if len(got) != 2 || got[0] != "files" || got[1] != "git" {
		t.Fatalf("decode: got %v", got)
	}
}
