package pomo

import (
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreReopenAndRecovery(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := testTime()
	e := NewEngine("focus", time.Minute, now)
	e.Advance(now.Add(time.Second))
	if err := s.Save(&e.Session); err != nil {
		t.Fatal(err)
	}
	e.Advance(now.Add(2 * time.Second))
	if err := s.Save(&e.Session); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Recover(); err != nil {
		t.Fatal(err)
	}
	spans, err := s.FocusSpans()
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 || spans[0].End.Sub(spans[0].Start) != 2*time.Second {
		t.Fatalf("duplicated or lost checkpoint: %+v", spans)
	}
	var state string
	if err := s.db.QueryRow("SELECT state FROM sessions WHERE id=?", e.Session.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "interrupted" {
		t.Fatalf("recovery state = %s", state)
	}
	report, err := readReport(s, now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 2*time.Second || report.Completed != 0 {
		t.Fatal("recovery counted unconfirmed time")
	}
}

func TestGuidedCyclePersistsExtensionAndCadence(t *testing.T) {
	s := testStore(t)
	cfg := Config{"2s", "1s", "3s", 2, false}
	now := testTime()
	c, err := NewController(s, cfg, "focus", 2*time.Second, now)
	if err != nil {
		t.Fatal(err)
	}
	c.Advance(now.Add(time.Second))
	c.Advance(now.Add(2 * time.Second))
	firstID := c.Engine.Session.ID
	c.Engine.Extend(now.Add(10 * time.Second))
	c.Advance(now.Add(11 * time.Second))
	c.Advance(now.Add(12 * time.Second))
	if err := c.StartBreak(now.Add(12 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if c.LongBreak || c.Engine.Session.Planned != time.Second {
		t.Fatal("wrong first break")
	}
	c.Advance(now.Add(13 * time.Second))
	if err := c.StartFocus(now.Add(20 * time.Second)); err != nil {
		t.Fatal(err)
	}
	c.Advance(now.Add(21 * time.Second))
	c.Advance(now.Add(22 * time.Second))
	if err := c.StartBreak(now.Add(22 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if !c.LongBreak || c.Engine.Session.Planned != 3*time.Second {
		t.Fatal("long break cadence failed")
	}
	if c.Completed != 2 {
		t.Fatal("extension incremented completed count")
	}
	r, err := readReport(s, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 6*time.Second || r.Completed != 2 {
		t.Fatalf("wrong totals: %+v", r)
	}
	var spanCount int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM spans WHERE session_id=?", firstID).Scan(&spanCount); err != nil {
		t.Fatal(err)
	}
	if spanCount != 2 {
		t.Fatal("extension not attached to original focus")
	}
}

func TestEarlyStopCanStartBreakAndDisableLongBreak(t *testing.T) {
	s := testStore(t)
	cfg := Config{"5s", "2s", "3s", 0, false}
	now := testTime()
	c, err := NewController(s, cfg, "focus", 5*time.Second, now)
	if err != nil {
		t.Fatal(err)
	}
	c.Advance(now.Add(time.Second))
	if err := c.StartBreak(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	r, err := readReport(s, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != time.Second || r.Completed != 0 {
		t.Fatal("interrupted session lost or counted complete")
	}
	if duration, long := nextBreak(cfg, 4, true); long || duration != 2*time.Second {
		t.Fatal("disabled long break still scheduled")
	}
}

func TestRecoverCompletedOvertime(t *testing.T) {
	s := testStore(t)
	now := testTime()
	e := NewEngine("focus", time.Second, now)
	e.Advance(now.Add(time.Second))
	e.Extend(now.Add(5 * time.Second))
	e.Advance(now.Add(6 * time.Second))
	if err := s.Save(&e.Session); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover(); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := s.db.QueryRow("SELECT state FROM sessions WHERE id=?", e.Session.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "completed" {
		t.Fatal("lost completed status after overtime crash")
	}
	r, err := readReport(s, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 2*time.Second || r.Completed != 1 {
		t.Fatal("incorrect recovered extension")
	}
}

func TestExclusiveTimerAndConcurrentStats(t *testing.T) {
	dir := t.TempDir()
	unlock, err := acquireTimerLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if second, err := acquireTimerLock(dir); err == nil {
		second()
		t.Fatal("second timer obtained lock")
	}
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	reader, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	e := NewEngine("focus", time.Minute, testTime())
	e.Advance(testTime().Add(time.Second))
	if err := s.Save(&e.Session); err != nil {
		t.Fatal(err)
	}
	r, err := readReport(reader, testTime().Add(time.Minute))
	if err != nil || r.Total != time.Second {
		t.Fatalf("stats while timer active: %+v, %v", r, err)
	}
}
