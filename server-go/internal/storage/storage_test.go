package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"stickers-server/internal/config"
	"stickers-server/internal/models"
)

func newStore(t *testing.T) (*Store, config.Settings) {
	t.Helper()
	s := config.Default()
	s.StorageDir = t.TempDir()
	return New(s), s
}

func TestValidID(t *testing.T) {
	if !ValidID(NewID()) {
		t.Fatal("NewID must be valid")
	}
	for _, bad := range []string{"", "../etc/passwd", "ABCDEF0123456789ABCDEF0123456789", "short"} {
		if ValidID(bad) {
			t.Errorf("ValidID(%q) = true", bad)
		}
	}
}

func TestJobRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	id := NewID()
	if got, err := st.ReadJob(id); err != nil || got != nil {
		t.Fatalf("missing job: got %v, %v", got, err)
	}
	if err := st.WriteJob(models.JobState{JobID: id, Status: models.JobPending}); err != nil {
		t.Fatal(err)
	}
	got, err := st.UpdateJob(id, func(j *models.JobState) {
		j.Status = models.JobReady
		j.Progress = 1
		j.Metadata = &models.JobMetadata{Duration: 5, Width: 720, Height: 1280, FPS: 30}
	})
	if err != nil {
		t.Fatal(err)
	}
	read, _ := st.ReadJob(id)
	if read.Status != models.JobReady || read.Metadata.Width != 720 || read.UpdatedAt != got.UpdatedAt {
		t.Fatalf("round trip mismatch: %+v", read)
	}
}

func TestConcurrentUpdatesDoNotLoseWrites(t *testing.T) {
	st, _ := newStore(t)
	id := NewID()
	st.WriteJob(models.JobState{JobID: id})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st.UpdateJob(id, func(j *models.JobState) { j.Progress += 0.01 })
		}()
	}
	wg.Wait()
	got, _ := st.ReadJob(id)
	if got.Progress < 0.499 || got.Progress > 0.501 {
		t.Fatalf("progress = %v, want ~0.5", got.Progress)
	}
}

func TestFindSourceFileIgnoresTmp(t *testing.T) {
	st, cfg := newStore(t)
	id := NewID()
	dir, _ := st.CreateJobDir(id)
	os.WriteFile(filepath.Join(dir, "source.mp4.tmp"), nil, 0o644)
	if st.FindSourceFile(id) != "" {
		t.Fatal("tmp file should be ignored")
	}
	want := filepath.Join(cfg.JobDir(id), "source.mp4")
	os.WriteFile(want, nil, 0o644)
	if got := st.FindSourceFile(id); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSweepExpired(t *testing.T) {
	st, cfg := newStore(t)
	oldID, newID := NewID(), NewID()
	oldDir, _ := st.CreateJobDir(oldID)
	st.CreateJobDir(newID)
	past := time.Now().Add(-cfg.JobTTL - time.Hour)
	os.Chtimes(oldDir, past, past)

	if n := st.SweepExpired(); n != 1 {
		t.Fatalf("removed %d, want 1", n)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatal("old dir should be gone")
	}
	if _, err := os.Stat(cfg.JobDir(newID)); err != nil {
		t.Fatal("fresh dir should remain")
	}
}
