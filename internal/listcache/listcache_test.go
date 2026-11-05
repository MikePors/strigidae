package listcache

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestSnapshots(test *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	directory := filepath.Join(test.TempDir(), "cache")
	store := New(directory, "example.test", "alice", func() time.Time { return now })
	key := Key{Kind: "search", Query: "author:@me", Limit: 30}
	items := []github.PullRequest{{Number: 1, Title: "private list row"}}
	if err := store.Save(key, items, now); err != nil {
		test.Fatal(err)
	}
	for _, testCase := range []struct {
		name string
		age  time.Duration
		hit  bool
	}{
		{"fresh", 0, true}, {"boundary", 24 * time.Hour, true}, {"expired", 24*time.Hour + time.Second, false}, {"future", -time.Second, false},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			reader := New(directory, "example.test", "alice", func() time.Time { return now.Add(testCase.age) })
			got, hit := reader.Load(key)
			if hit != testCase.hit {
				test.Fatalf("hit=%v", hit)
			}
			if hit {
				if difference := cmp.Diff(items, got); difference != "" {
					test.Fatal(difference)
				}
			}
		})
	}
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{directory: 0o700, store.path(key): 0o600} {
			info, err := os.Stat(path)
			if err != nil {
				test.Fatal(err)
			}
			if info.Mode().Perm() != want {
				test.Errorf("permissions %s: %v", path, info.Mode().Perm())
			}
		}
	}
	if err := store.Save(key, []github.PullRequest{{Number: 2}}, now.Add(-time.Minute)); err != nil {
		test.Fatal(err)
	}
	got, _ := store.Load(key)
	if difference := cmp.Diff(items, got); difference != "" {
		test.Fatal(difference)
	}
	if err := store.Save(key, nil, now); err != nil {
		test.Fatal(err)
	}
	got, hit := store.Load(key)
	if !hit || len(got) != 0 {
		test.Fatal("empty success did not replace snapshot")
	}
	if err := store.Remove(key); err != nil {
		test.Fatal(err)
	}
	if _, hit := store.Load(key); hit {
		test.Fatal("removed entry still loaded")
	}
}

func TestIsolation(test *testing.T) {
	directory := test.TempDir()
	now := time.Now()
	key := Key{Kind: "search", Query: "author:@me", Limit: 30}
	store := New(directory, "example.test", "alice", nil)
	if err := store.Save(key, []github.PullRequest{{Number: 1}}, now); err != nil {
		test.Fatal(err)
	}
	for _, testCase := range []struct {
		name, host, account string
		key                 Key
	}{
		{"account", "example.test", "bob", key},
		{"host", "other.test", "alice", key},
		{"query", "example.test", "alice", Key{Kind: "search", Query: "author:bob", Limit: 30}},
		{"limit", "example.test", "alice", Key{Kind: "search", Query: "author:@me", Limit: 5}},
		{"kind", "example.test", "alice", Key{Kind: "repository", Query: "author:@me", Limit: 30}},
		{"repository", "example.test", "alice", Key{Kind: "search", Query: "author:@me", Limit: 30, Repository: github.Repository{Owner: "a", Name: "b"}}},
		{"unknown account", "example.test", "", key},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			if _, hit := New(directory, testCase.host, testCase.account, nil).Load(testCase.key); hit {
				test.Fatal("crossed cache boundary")
			}
		})
	}
}

func TestUnusableCache(test *testing.T) {
	store := New(test.TempDir(), "example.test", "alice", nil)
	key := Key{Kind: "search"}
	for _, contents := range []string{"broken", `{"Version":2}`, `{"Version":1}`, `{"Version":1,"FetchedAt":"bad"}`} {
		if err := os.WriteFile(store.path(key), []byte(contents), 0o600); err != nil {
			test.Fatal(err)
		}
		if _, hit := store.Load(key); hit {
			test.Fatalf("accepted %s", contents)
		}
	}
	blocked := filepath.Join(test.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		test.Fatal(err)
	}
	store = New(blocked, "example.test", "alice", nil)
	if _, hit := store.Load(key); hit {
		test.Fatal("file directory accepted")
	}
	if err := store.Save(key, nil, time.Now()); err == nil {
		test.Fatal("write failure missing")
	}
	var disabled *Store
	if err := disabled.Save(key, nil, time.Now()); err != nil {
		test.Fatal(err)
	}
}

func TestFutureSnapshotCanBeReplaced(test *testing.T) {
	now := time.Now()
	store := New(test.TempDir(), "example.test", "alice", func() time.Time { return now })
	key := Key{Kind: "search"}
	if err := store.Save(key, nil, now.Add(time.Hour)); err != nil {
		test.Fatal(err)
	}
	if err := store.Save(key, []github.PullRequest{{Number: 42}}, now); err != nil {
		test.Fatal(err)
	}
	items, hit := store.Load(key)
	if !hit || len(items) != 1 || items[0].Number != 42 {
		test.Fatal("future timestamp prevented recovery")
	}
}
