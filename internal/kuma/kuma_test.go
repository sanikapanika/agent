package kuma

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uptimy/agent/internal/monitor"
)

// A Kuma 2 database, cut down to the columns the importer reads.
const kumaSchema = `
CREATE TABLE monitor (
	id INTEGER PRIMARY KEY, name TEXT, active BOOLEAN DEFAULT 1, interval INTEGER DEFAULT 20, url TEXT, type TEXT,
	hostname TEXT, port INTEGER, keyword TEXT, maxretries INTEGER DEFAULT 0, ignore_tls BOOLEAN DEFAULT 0,
	upside_down BOOLEAN DEFAULT 0, accepted_statuscodes_json TEXT DEFAULT '["200-299"]', dns_resolve_type TEXT,
	dns_resolve_server TEXT, retry_interval INTEGER DEFAULT 0, push_token TEXT, method TEXT DEFAULT 'GET', body TEXT,
	headers TEXT, basic_auth_user TEXT, basic_auth_pass TEXT, auth_method TEXT, bearer_token TEXT,
	database_connection_string TEXT, database_query TEXT, timeout DOUBLE, invert_keyword BOOLEAN DEFAULT 0
);
CREATE TABLE notification (id INTEGER PRIMARY KEY, name TEXT, active BOOLEAN DEFAULT 1, is_default BOOLEAN DEFAULT 0, config TEXT);
CREATE TABLE monitor_notification (id INTEGER PRIMARY KEY, monitor_id INTEGER, notification_id INTEGER);
CREATE TABLE monitor_group (id INTEGER PRIMARY KEY, monitor_id INTEGER, group_id INTEGER, weight INTEGER DEFAULT 1000);
CREATE TABLE "group" (id INTEGER PRIMARY KEY, name TEXT, public BOOLEAN DEFAULT 0, active BOOLEAN DEFAULT 1, weight INTEGER DEFAULT 1000, status_page_id INTEGER);

INSERT INTO monitor (id, name, type, url, interval, maxretries, timeout, accepted_statuscodes_json, auth_method, basic_auth_user, basic_auth_pass, headers)
	VALUES (1, 'Website', 'http', 'https://example.com', 60, 2, 48, '["200-299","301"]', 'basic', 'ops', 's3cret', '{"X-Env":"prod"}');
INSERT INTO monitor (id, name, type, url, keyword, active) VALUES (2, 'Login page', 'keyword', 'https://example.com/login', 'Sign in', 0);
INSERT INTO monitor (id, name, type, hostname, port) VALUES (3, 'Postgres port', 'port', 'db.internal', 5432);
INSERT INTO monitor (id, name, type, hostname) VALUES (4, 'Router', 'ping', '10.0.0.1');
INSERT INTO monitor (id, name, type, hostname, dns_resolve_type, dns_resolve_server, port) VALUES (5, 'MX', 'dns', 'example.com', 'MX', '1.1.1.1', 53);
INSERT INTO monitor (id, name, type, interval, maxretries, retry_interval, push_token) VALUES (6, 'Backup job', 'push', 3600, 2, 600, 'abcdefghij');
INSERT INTO monitor (id, name, type, database_connection_string, database_query) VALUES (7, 'Main DB', 'postgres', 'postgres://u:p@db:5432/app', 'SELECT 1');
INSERT INTO monitor (id, name, type) VALUES (8, 'Container', 'docker');
INSERT INTO monitor (id, name, type, url, upside_down) VALUES (9, 'Inverted', 'http', 'https://example.com', 1);
INSERT INTO monitor (id, name, type, url, keyword, invert_keyword) VALUES (10, 'No error', 'keyword', 'https://example.com', 'error', 1);

INSERT INTO notification (id, name, active, config) VALUES
	(1, 'Ops Slack', 1, '{"type":"slack","slackwebhookURL":"https://hooks.slack.com/services/T/B/x"}'),
	(2, 'On-call', 1, '{"type":"PagerDuty","pagerdutyIntegrationKey":"0123456789abcdef0123456789abcdef"}'),
	(3, 'Mail', 0, '{"type":"smtp","smtpHost":"smtp.example.com","smtpPort":465,"smtpSecure":true,"smtpFrom":"kuma@example.com","smtpTo":"ops@example.com","smtpCC":"cto@example.com"}'),
	(4, 'Gotify', 1, '{"type":"gotify","gotifyserverurl":"http://gotify.lan:8080/","gotifyapplicationToken":"AbCdEf","gotifyPriority":8}'),
	(5, 'Pushover', 1, '{"type":"pushover","pushoveruserkey":"uKey123","pushoverapptoken":"aToken456","pushoverdevice":"phone"}'),
	(6, 'Matrix', 1, '{"type":"matrix","homeserverUrl":"https://matrix.org","internalRoomId":"!abc:matrix.org","accessToken":"syt_token"}'),
	(7, 'Mattermost', 1, '{"type":"mattermost","mattermostWebhookUrl":"https://mm.example.com/hooks/xyz789","mattermostchannel":"alerts","mattermostusername":"kuma"}'),
	(8, 'Google Chat', 1, '{"type":"GoogleChat","googleChatWebhookURL":"https://chat.googleapis.com/v1/spaces/AAA/messages?key=k1&token=t1"}'),
	(9, 'Rocket', 1, '{"type":"rocket.chat","rocketwebhookURL":"https://rocket.example.com/hooks/tokenA/tokenB","rocketchannel":"#ops"}'),
	(10, 'Opsgenie EU', 1, '{"type":"Opsgenie","opsgenieApiKey":"og-key","opsgenieRegion":"eu"}'),
	(11, 'Broken Gotify', 1, '{"type":"gotify","gotifyserverurl":"not a url"}');
INSERT INTO monitor_notification (monitor_id, notification_id) VALUES (1,1),(2,1),(3,1),(4,1),(5,1),(6,1),(7,1),(7,2),(8,2);
INSERT INTO "group" (id, name, weight, status_page_id) VALUES (1, 'Core APIs', 2, 1), (2, 'Website', 1, 1), (3, 'core apis', 1, 2);
INSERT INTO monitor_group (monitor_id, group_id, weight) VALUES (1, 2, 1), (7, 1, 2), (3, 1, 1), (3, 3, 1);
`

func kumaDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kuma.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(kumaSchema); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRead(t *testing.T) {
	plan, err := Read(context.Background(), kumaDB(t))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Monitor{}
	for _, m := range plan.Monitors {
		byName[m.Monitor.Name] = m
	}
	if len(plan.Monitors) != 7 {
		t.Fatalf("imported %d monitors: %+v", len(plan.Monitors), plan.Monitors)
	}

	web := byName["Website"].Monitor
	if web.Check.Type != monitor.TypeHTTP || web.Check.Config.ExpectedStatus != "200-299,301" || web.Check.FailureThreshold != 3 ||
		web.Check.TimeoutSeconds != 48 || !web.Public || web.Check.Config.Headers["X-Env"] != "prod" ||
		web.Check.Config.Headers["Authorization"] != "Basic b3BzOnMzY3JldA==" {
		t.Fatalf("website: %+v %+v", web, web.Check)
	}
	if login := byName["Login page"].Monitor; !login.Paused || login.Check.Config.Keyword != "Sign in" || login.Public {
		t.Fatalf("keyword: %+v", login)
	}
	if tcp := byName["Postgres port"].Monitor; tcp.Check.Type != monitor.TypeTCP || tcp.Check.Target != "db.internal:5432" {
		t.Fatalf("port: %+v", tcp.Check)
	}
	if ping := byName["Router"].Monitor; ping.Check.Type != monitor.TypePing || ping.Check.Target != "10.0.0.1" {
		t.Fatalf("ping: %+v", ping.Check)
	}
	if dns := byName["MX"].Monitor; dns.Check.Config.RecordType != "MX" || dns.Check.Config.Resolver != "1.1.1.1" {
		t.Fatalf("dns: %+v", dns.Check.Config)
	}
	backup := byName["Backup job"]
	if hb := backup.Monitor.Heartbeat; backup.Monitor.Kind != monitor.KindHeartbeat || hb.EverySeconds != 3600 || hb.GraceSeconds != 7200 ||
		len(backup.Notes) == 0 || !strings.Contains(backup.Notes[0], "new ping URL") {
		t.Fatalf("push: %+v %v", hb, backup.Notes)
	}
	if db := byName["Main DB"].Monitor; db.Check.Type != monitor.TypePostgres || db.Check.Config.Query != "SELECT 1" {
		t.Fatalf("postgres: %+v", db.Check)
	}

	skipped := map[string]string{}
	for _, s := range plan.Skipped {
		skipped[s.Name] = s.Reason
	}
	for name, want := range map[string]string{
		"Container": "Docker container monitors", "Inverted": "upside-down", "No error": "inverted keyword", "Broken Gotify": "Gotify server URL",
	} {
		if !strings.Contains(skipped[name], want) {
			t.Errorf("%s skipped for %q, want %q", name, skipped[name], want)
		}
	}

	notifiers := map[string]Notifier{}
	for _, n := range plan.Notifiers {
		notifiers[n.Notifier.Name] = n
	}
	// Ops Slack alerted for every imported monitor: it stays that way.
	if slack := notifiers["Ops Slack"]; !slack.Notifier.AllMonitors || slack.Notifier.Type != "slack" {
		t.Fatalf("slack: %+v", slack)
	}
	// On-call only for Main DB (the Docker monitor wasn't imported).
	if pd := notifiers["On-call"]; pd.Notifier.AllMonitors || len(pd.KumaMonitorIDs) != 1 || pd.KumaMonitorIDs[0] != 7 || pd.Notifier.Type != "pagerduty" {
		t.Fatalf("pagerduty: %+v", pd)
	}
	// Services the agent reaches through Shoutrrr become "More services" channels.
	for name, want := range map[string]string{
		"Gotify":      "gotify://gotify.lan:8080/AbCdEf?disabletls=yes&priority=8",
		"Pushover":    "pushover://shoutrrr:aToken456@uKey123/?devices=phone",
		"Matrix":      "matrix://:syt_token@matrix.org/?rooms=%21abc%3Amatrix.org",
		"Mattermost":  "mattermost://kuma@mm.example.com/xyz789/alerts",
		"Google Chat": "googlechat://chat.googleapis.com/v1/spaces/AAA/messages?key=k1&token=t1",
		"Rocket":      "rocketchat://rocket.example.com/tokenA/tokenB/%23ops",
		"Opsgenie EU": "opsgenie://api.eu.opsgenie.com/og-key",
	} {
		n, ok := notifiers[name]
		if !ok || n.Notifier.Type != "shoutrrr" || n.Notifier.Config.URL != want {
			t.Errorf("%s: %+v (skipped: %q), want %s", name, n.Notifier, skipped[name], want)
		}
	}

	// Status page groups become sections, in Kuma's order; same-named groups
	// on two pages are one section, and a monitor goes in its first group.
	if len(plan.Sections) != 2 || plan.Sections[0].Name != "Website" || plan.Sections[0].ID != "website" ||
		plan.Sections[1].Name != "Core APIs" || plan.Sections[1].ID != "core-apis" {
		t.Fatalf("sections: %+v", plan.Sections)
	}
	for name, want := range map[string]struct {
		section string
		order   int
	}{"Website": {"website", 1}, "Postgres port": {"core-apis", 2}, "Main DB": {"core-apis", 3}} {
		m := byName[name].Monitor
		if !m.Public || m.StatusSection != want.section || m.StatusOrder != want.order {
			t.Errorf("%s: public=%v section=%q order=%d, want %+v", name, m.Public, m.StatusSection, m.StatusOrder, want)
		}
	}

	mail := notifiers["Mail"].Notifier
	if mail.Type != "email" || mail.Enabled || mail.Config.SMTPSecurity != "tls" || mail.Config.SMTPTo != "ops@example.com,cto@example.com" {
		t.Fatalf("email: %+v", mail)
	}
}

func TestReadRejectsOtherFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.db")
	db, _ := sql.Open("sqlite", path)
	db.Exec("CREATE TABLE something (id INTEGER)")
	db.Close()
	if _, err := Read(context.Background(), path); err == nil || !strings.Contains(err.Error(), "Uptime Kuma") {
		t.Fatalf("err: %v", err)
	}
}
