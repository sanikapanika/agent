package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/uptimy/agent/internal/kube"
	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/scheduler"
	"github.com/uptimy/agent/internal/store"
)

// JobInterval is how often discovered CronJobs' Jobs are read for runs.
const JobInterval = 10 * time.Second

// A CronJob's heartbeat gets its runs from the CronJob's Jobs instead of
// pings: a Job's start time starts a run, and its Complete or Failed
// condition finishes it, at the times Kubernetes recorded. Missed runs come
// from the heartbeat's schedule as usual.

// Report hands a run's start or finish to the scheduler.
type Report func(heartbeatID int64, signal scheduler.Signal, at time.Time, message string)

type cronJobRef struct {
	namespace, name, uid string
	monitor              string // the heartbeat's name
}

type jobTracker struct {
	// seen: a Job's start and finish already handled, by Job UID.
	seen map[string]*seenJob
	// since, per heartbeat: what happened at or before it was handled before
	// the agent started (or is older than the heartbeat), so it isn't
	// reported again.
	since map[int64]time.Time
}

type seenJob struct {
	namespace     string
	start, finish bool
}

type ownerRef struct {
	UID string `json:"uid"`
}

type job struct {
	Metadata struct {
		Name            string     `json:"name"`
		UID             string     `json:"uid"`
		OwnerReferences []ownerRef `json:"ownerReferences"`
	} `json:"metadata"`
	Status struct {
		StartTime  *time.Time `json:"startTime"`
		Conditions []struct {
			Type               string    `json:"type"`
			Status             string    `json:"status"`
			Reason             string    `json:"reason"`
			Message            string    `json:"message"`
			LastTransitionTime time.Time `json:"lastTransitionTime"`
		} `json:"conditions"`
	} `json:"status"`
}

func (j job) ownedBy(uid string) bool {
	return slices.ContainsFunc(j.Metadata.OwnerReferences, func(o ownerRef) bool { return o.UID == uid })
}

// finished returns how the Job ended, if it has: its Complete or Failed
// condition.
func (j job) finished() (ok bool, at time.Time, reason string, done bool) {
	for _, c := range j.Status.Conditions {
		if c.Status != "True" {
			continue
		}
		switch c.Type {
		case "Complete":
			return true, c.LastTransitionTime, "", true
		case "Failed":
			reason := c.Reason
			if c.Message != "" {
				reason += ": " + c.Message
			}
			return false, c.LastTransitionTime, reason, true
		}
	}
	return false, time.Time{}, "", false
}

type jobEvent struct {
	job    job
	signal scheduler.Signal
	at     time.Time
	reason string // a failure's
}

// trackJobs reports the runs of discovered CronJobs that it hasn't yet.
func (d *Discoverer) trackJobs(ctx context.Context, st *store.Store, report Report) error {
	if len(d.cronJobs) == 0 {
		return nil
	}
	all, err := st.ListMonitors(ctx)
	if err != nil {
		return err
	}
	heartbeats := map[string]monitor.Monitor{}
	for _, m := range all {
		if m.Source == monitor.SourceKubernetes && m.Kind == monitor.KindHeartbeat {
			heartbeats[m.Name] = m
		}
	}
	jobsIn := map[string][]job{} // by namespace
	live := map[string]bool{}    // Job UIDs that still exist
	for _, ref := range d.cronJobs {
		m, ok := heartbeats[ref.monitor]
		if !ok {
			continue // not saved yet
		}
		jobs, listed := jobsIn[ref.namespace]
		if !listed {
			var l struct {
				Items []job `json:"items"`
			}
			err := d.kube.Get(ctx, "/apis/batch/v1/namespaces/"+ref.namespace+"/jobs", &l)
			switch {
			case kube.IsForbidden(err):
				d.warn("not allowed to list jobs in " + ref.namespace + ", so CronJob runs can't be read; give the agent's service account list access to jobs")
				continue
			case err != nil:
				return fmt.Errorf("listing jobs in %s: %w", ref.namespace, err)
			}
			jobs = l.Items
			jobsIn[ref.namespace] = jobs
		}
		since, ok := d.jobs.since[m.ID]
		if !ok {
			since = handledUntil(ctx, st, m)
			d.jobs.since[m.ID] = since
		}

		var events []jobEvent
		for _, j := range jobs {
			if !j.ownedBy(ref.uid) {
				continue
			}
			live[j.Metadata.UID] = true
			if j.Status.StartTime != nil {
				events = append(events, jobEvent{job: j, signal: scheduler.SignalStart, at: *j.Status.StartTime})
			}
			if ok, at, reason, done := j.finished(); done {
				e := jobEvent{job: j, signal: scheduler.SignalSuccess, at: at}
				if !ok {
					e.signal, e.reason = scheduler.SignalFailure, reason
				}
				events = append(events, e)
			}
		}
		// In order, and a Job's start before its finish when they share a
		// second.
		slices.SortStableFunc(events, func(a, b jobEvent) int {
			if c := a.at.Compare(b.at); c != 0 {
				return c
			}
			return boolInt(a.signal != scheduler.SignalStart) - boolInt(b.signal != scheduler.SignalStart)
		})
		for _, e := range events {
			s := d.jobs.seen[e.job.Metadata.UID]
			if s == nil {
				s = &seenJob{namespace: ref.namespace}
				d.jobs.seen[e.job.Metadata.UID] = s
			}
			flag := &s.finish
			if e.signal == scheduler.SignalStart {
				flag = &s.start
			}
			if *flag {
				continue
			}
			*flag = true
			// While paused, runs are ignored, as pings are.
			if m.Paused || !e.at.After(since) {
				continue
			}
			report(m.ID, e.signal, e.at, d.jobMessage(ctx, ref.namespace, e))
		}
	}
	// Forget Jobs that are gone (TTL, history limit), in namespaces that
	// were read.
	for uid, s := range d.jobs.seen {
		if _, listed := jobsIn[s.namespace]; listed && !live[uid] {
			delete(d.jobs.seen, uid)
		}
	}
	return nil
}

// handledUntil is when the heartbeat's history ends: its last run (started
// or finished), or its creation. A Job event at or before it was already
// recorded, or predates the heartbeat.
func handledUntil(ctx context.Context, st *store.Store, m monitor.Monitor) time.Time {
	t := m.CreatedAt
	later := func(p *time.Time) {
		if p != nil && p.After(t) {
			t = *p
		}
	}
	if last, err := st.LastRun(ctx, m.ID); err == nil {
		later(last.StartedAt)
		later(last.FinishedAt)
	}
	if open, err := st.OpenRun(ctx, m.ID); err == nil {
		later(open.StartedAt)
	}
	return t
}

// maxMessage matches the length kept for a ping's message.
const maxMessage = 500

// jobMessage describes a run for its history: which Job, and for a failure
// why, with the failed container's exit code (OOMKilled, exit code 1, ...).
func (d *Discoverer) jobMessage(ctx context.Context, namespace string, e jobEvent) string {
	name := e.job.Metadata.Name
	switch e.signal {
	case scheduler.SignalStart:
		return ""
	case scheduler.SignalSuccess:
		return "Job " + name + " completed"
	}
	msg := "Job " + name + " failed"
	if e.reason != "" {
		msg += ": " + e.reason
	}
	if c := d.failedContainer(ctx, namespace, name); c != "" {
		msg += "; " + c
	}
	if len(msg) > maxMessage {
		msg = msg[:maxMessage]
	}
	return msg
}

type pod struct {
	Metadata struct {
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Status struct {
		ContainerStatuses []struct {
			Name  string `json:"name"`
			State struct {
				Terminated *struct {
					ExitCode int    `json:"exitCode"`
					Reason   string `json:"reason"`
				} `json:"terminated"`
			} `json:"state"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

// failedContainer finds why the Job's last pod failed: "container backup
// exited with code 3 (Error)". Empty if its pods are gone.
func (d *Discoverer) failedContainer(ctx context.Context, namespace, jobName string) string {
	var l struct {
		Items []pod `json:"items"`
	}
	sel := url.QueryEscape("job-name=" + jobName)
	if err := d.kube.Get(ctx, "/api/v1/namespaces/"+namespace+"/pods?labelSelector="+sel, &l); err != nil {
		if !errors.Is(err, context.Canceled) && !kube.IsForbidden(err) {
			d.log.Debug("reading a failed job's pods", "job", jobName, "err", err)
		}
		return ""
	}
	// The newest pod is the last attempt.
	slices.SortFunc(l.Items, func(a, b pod) int { return b.Metadata.CreationTimestamp.Compare(a.Metadata.CreationTimestamp) })
	for _, p := range l.Items {
		var parts []string
		for _, c := range p.Status.ContainerStatuses {
			t := c.State.Terminated
			if t == nil || t.ExitCode == 0 {
				continue
			}
			s := fmt.Sprintf("container %s exited with code %d", c.Name, t.ExitCode)
			if t.Reason != "" {
				s += " (" + t.Reason + ")"
			}
			parts = append(parts, s)
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	}
	return ""
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
