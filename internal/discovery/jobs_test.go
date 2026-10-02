package discovery

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uptimy/agent/internal/managed"
	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/scheduler"
	"github.com/uptimy/agent/internal/store"
)

const cronJobs = `{"items":[
 {"metadata":{"labels":{"upti.my/monitor":"true"},"name":"backup","namespace":"shop","uid":"cj-1"},
  "spec":{"schedule":"0 3 * * *","timeZone":"Europe/Berlin","jobTemplate":{"spec":{"activeDeadlineSeconds":600}}}},
 {"metadata":{"labels":{"upti.my/monitor":"true"},"name":"sync","namespace":"shop","uid":"cj-2","annotations":{"upti.my/name":"Product sync"}},
  "spec":{"schedule":"*/5 * * * *"}},
 {"metadata":{"labels":{"upti.my/monitor":"true"},"name":"report","namespace":"shop","uid":"cj-3"},
  "spec":{"schedule":"0 9 * * 1","suspend":true}}
]}`

type reported struct {
	id      int64
	signal  scheduler.Signal
	at      time.Time
	message string
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// jobsJSON: an old run from before the heartbeat existed, one that
// completed, and one that failed.
func jobsJSON(old, start, done, failStart, failed time.Time) string {
	return fmt.Sprintf(`{"items":[
 {"metadata":{"labels":{"upti.my/monitor":"true"},"name":"backup-1","uid":"j-1","ownerReferences":[{"uid":"cj-1"}]},
  "status":{"startTime":%q,"conditions":[{"type":"Complete","status":"True","lastTransitionTime":%q}]}},
 {"metadata":{"labels":{"upti.my/monitor":"true"},"name":"backup-2","uid":"j-2","ownerReferences":[{"uid":"cj-1"}]},
  "status":{"startTime":%q,"conditions":[{"type":"SuccessCriteriaMet","status":"True","lastTransitionTime":%q},
                                         {"type":"Complete","status":"True","lastTransitionTime":%q}]}},
 {"metadata":{"labels":{"upti.my/monitor":"true"},"name":"backup-3","uid":"j-3","ownerReferences":[{"uid":"cj-1"}]},
  "status":{"startTime":%q,"conditions":[{"type":"Failed","status":"True","reason":"BackoffLimitExceeded","message":"Job has reached the specified backoff limit","lastTransitionTime":%q}]}},
 {"metadata":{"labels":{"upti.my/monitor":"true"},"name":"other","uid":"j-9","ownerReferences":[{"uid":"someone-else"}]},
  "status":{"startTime":%q}}
]}`, ts(old), ts(old.Add(time.Minute)), ts(start), ts(done), ts(done), ts(failStart), ts(failed), ts(start))
}

const failedPods = `{"items":[
 {"metadata":{"creationTimestamp":"2026-01-01T00:00:00Z"},"status":{"containerStatuses":[{"name":"backup","state":{"terminated":{"exitCode":1,"reason":"Error"}}}]}},
 {"metadata":{"creationTimestamp":"2026-01-01T00:01:00Z"},"status":{"containerStatuses":[{"name":"backup","state":{"terminated":{"exitCode":137,"reason":"OOMKilled"}}}]}}
]}`

func TestCronJobs(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	f, d := newFake(t)
	f.set("/apis/batch/v1/cronjobs", cronJobs)

	ds, err := d.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beats := map[string]managed.Desired{}
	for _, x := range ds {
		if x.Kind == monitor.KindHeartbeat {
			beats[x.Name] = x
		}
	}
	backup, sync, report := beats["shop/backup (cronjob)"], beats["Product sync"], beats["shop/report (cronjob)"]
	if backup.Heartbeat == nil || backup.Heartbeat.Cron != "0 3 * * *" || backup.Heartbeat.Timezone != "Europe/Berlin" || backup.Heartbeat.GraceSeconds != 660 {
		t.Fatalf("backup: %+v", backup.Heartbeat)
	}
	if !backup.GeneratedToken {
		t.Error("a discovered heartbeat should keep its ping token")
	}
	// No deadline: until the next run, here five minutes.
	if sync.Heartbeat == nil || sync.Heartbeat.Timezone != "UTC" || sync.Heartbeat.GraceSeconds != 300 {
		t.Fatalf("sync: %+v", sync.Heartbeat)
	}
	if !report.Paused || report.SetPaused == nil || !*report.SetPaused {
		t.Fatalf("a suspended CronJob should pause its heartbeat: %+v", report)
	}
	if _, err := managed.Sync(ctx, st, monitor.SourceKubernetes, ds, managed.Options{KeepPaused: true}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	start, done := now.Add(time.Minute), now.Add(3*time.Minute)
	failStart, failed := now.Add(4*time.Minute), now.Add(5*time.Minute)
	f.set("/apis/batch/v1/namespaces/shop/jobs", jobsJSON(now.Add(-time.Hour), start, done, failStart, failed))
	f.set("/api/v1/namespaces/shop/pods?job-name=backup-3", failedPods)

	var got []reported
	collect := func(id int64, s scheduler.Signal, at time.Time, msg string) {
		got = append(got, reported{id, s, at, msg})
	}
	if err := d.trackJobs(ctx, st, collect); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		signal scheduler.Signal
		at     time.Time
		msg    string
	}{
		{scheduler.SignalStart, start, ""},
		{scheduler.SignalSuccess, done, "Job backup-2 completed"},
		{scheduler.SignalStart, failStart, ""},
		{scheduler.SignalFailure, failed, "Job backup-3 failed: BackoffLimitExceeded: Job has reached the specified backoff limit; container backup exited with code 137 (OOMKilled)"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d reports: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].signal != w.signal || !got[i].at.Equal(w.at) || got[i].message != w.msg {
			t.Errorf("report %d: got %+v, want %+v", i, got[i], w)
		}
	}

	// Nothing twice.
	got = nil
	if err := d.trackJobs(ctx, st, collect); err != nil || len(got) != 0 {
		t.Fatalf("second read reported %+v (%v)", got, err)
	}

	// After a restart, what the store already has isn't reported again.
	id := got0(t, st, "shop/backup (cronjob)").ID
	if _, err := st.InsertRun(ctx, monitor.Run{MonitorID: id, StartedAt: &failStart, FinishedAt: &failed, Outcome: monitor.OutcomeFailure}); err != nil {
		t.Fatal(err)
	}
	_, d2 := newFake(t)
	d2.kube, d2.cronJobs = d.kube, d.cronJobs
	if err := d2.trackJobs(ctx, st, collect); err != nil || len(got) != 0 {
		t.Fatalf("after a restart reported %+v (%v)", got, err)
	}

	// Resuming the CronJob resumes its heartbeat.
	f.set("/apis/batch/v1/cronjobs", strings.Replace(cronJobs, `"suspend":true`, `"suspend":false`, 1))
	ds, _ = d.Discover(ctx)
	for _, x := range ds {
		if x.Name == "shop/report (cronjob)" && (x.SetPaused == nil || *x.SetPaused) {
			t.Fatalf("resume: %+v", x)
		}
	}
}

func got0(t *testing.T, st *store.Store, name string) monitor.Monitor {
	t.Helper()
	all, _ := st.ListMonitors(context.Background())
	for _, m := range all {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("no monitor %q", name)
	return monitor.Monitor{}
}
