package checks

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uptimy/agent/internal/monitor"
)

func dbMonitor(t *testing.T, typ monitor.Type, target string, cfg monitor.Config) monitor.Check {
	t.Helper()
	return normalized(t, monitor.Check{Type: typ, Target: target, Config: cfg})
}

func runCheck(m monitor.Check) Outcome {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New(nil).Run(ctx, m)
}

// fakeRedis answers AUTH, SELECT and PING like a Redis server with the given
// password. It records the commands it received.
func fakeRedis(t *testing.T, password string) (addr string, commands *[]string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var (
		mu  sync.Mutex
		got []string
	)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				r := bufio.NewReader(conn)
				authed := password == ""
				for {
					args, err := readCommand(r)
					if err != nil {
						return
					}
					mu.Lock()
					got = append(got, strings.Join(args, " "))
					mu.Unlock()
					switch strings.ToUpper(args[0]) {
					case "AUTH":
						if args[len(args)-1] != password {
							fmt.Fprint(conn, "-WRONGPASS invalid username-password pair or user is disabled.\r\n")
							continue
						}
						authed = true
						fmt.Fprint(conn, "+OK\r\n")
					case "SELECT":
						fmt.Fprint(conn, "+OK\r\n")
					case "PING":
						if !authed {
							fmt.Fprint(conn, "-NOAUTH Authentication required.\r\n")
							continue
						}
						fmt.Fprint(conn, "+PONG\r\n")
					}
				}
			}()
		}
	}()
	return ln.Addr().String(), &got
}

func readCommand(r *bufio.Reader) ([]string, error) {
	var n int
	if _, err := fmt.Fscanf(r, "*%d\r\n", &n); err != nil {
		return nil, err
	}
	args := make([]string, n)
	for i := range args {
		var size int
		if _, err := fmt.Fscanf(r, "$%d\r\n", &size); err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := r.Read(buf); err != nil {
			return nil, err
		}
		args[i] = string(buf[:size])
	}
	return args, nil
}

func TestRedis(t *testing.T) {
	addr, commands := fakeRedis(t, "s3cret")

	out := runCheck(dbMonitor(t, monitor.TypeRedis, "redis://:s3cret@"+addr+"/2", monitor.Config{}))
	if !out.OK || out.Message != "PONG" {
		t.Fatalf("expected PONG, got %+v", out)
	}
	if want := "AUTH s3cret,SELECT 2,PING"; strings.Join(*commands, ",") != want {
		t.Fatalf("commands = %v, want %s", *commands, want)
	}

	out = runCheck(dbMonitor(t, monitor.TypeRedis, "redis://:wrong@"+addr, monitor.Config{}))
	if out.OK || !strings.Contains(out.Message, "WRONGPASS") || strings.Contains(out.Message, "wrong@") {
		t.Fatalf("expected auth failure, got %+v", out)
	}

	out = runCheck(dbMonitor(t, monitor.TypeRedis, "redis://"+addr, monitor.Config{}))
	if out.OK || !strings.Contains(out.Message, "NOAUTH") {
		t.Fatalf("expected NOAUTH, got %+v", out)
	}
}

func TestRedisNotRedis(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			fmt.Fprint(conn, "HTTP/1.1 400 Bad Request\r\n\r\n")
			conn.Close()
		}
	}()
	out := runCheck(dbMonitor(t, monitor.TypeRedis, "redis://"+ln.Addr().String(), monitor.Config{}))
	if out.OK || !strings.Contains(out.Message, "not a Redis server") {
		t.Fatalf("expected protocol failure, got %+v", out)
	}
}

func TestDatabaseEnvTarget(t *testing.T) {
	addr, _ := fakeRedis(t, "")
	t.Setenv("CACHE_URL", "redis://"+addr)
	m := dbMonitor(t, monitor.TypeRedis, "${CACHE_URL}", monitor.Config{})
	if out := runCheck(m); !out.OK {
		t.Fatalf("expected up, got %+v", out)
	}
	os.Unsetenv("CACHE_URL")
	if out := runCheck(m); out.OK || !strings.Contains(out.Message, "CACHE_URL is not set") {
		t.Fatalf("expected unset variable, got %+v", out)
	}
}

// Postgres and MySQL run against real servers when these are set, e.g.
//
//	AGENT_TEST_POSTGRES_URL=postgres://postgres:pw@localhost:5432/postgres?sslmode=disable
//	AGENT_TEST_MYSQL_URL=mysql://root:pw@localhost:3306/mysql?tls=false
func TestPostgres(t *testing.T) {
	url := os.Getenv("AGENT_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("AGENT_TEST_POSTGRES_URL not set")
	}
	testSQL(t, monitor.TypePostgres, url, "PostgreSQL")
}

// AGENT_TEST_REDIS_URL=redis://:pw@localhost:6379 runs against a real server.
func TestRedisServer(t *testing.T) {
	url := os.Getenv("AGENT_TEST_REDIS_URL")
	if url == "" {
		t.Skip("AGENT_TEST_REDIS_URL not set")
	}
	if out := runCheck(dbMonitor(t, monitor.TypeRedis, url, monitor.Config{})); !out.OK || out.Message != "PONG" {
		t.Fatalf("PING: %+v", out)
	}
	bad := strings.Replace(url, "://", "://:wrong-password-x@", 1)
	if i := strings.Index(url, "@"); i >= 0 {
		bad = url[:strings.Index(url, "://")+3] + ":wrong-password-x" + url[i:]
	}
	if out := runCheck(dbMonitor(t, monitor.TypeRedis, bad, monitor.Config{})); out.OK || strings.Contains(out.Message, "wrong-password-x") {
		t.Fatalf("bad password: %+v", out)
	}
}

func TestMySQL(t *testing.T) {
	url := os.Getenv("AGENT_TEST_MYSQL_URL")
	if url == "" {
		t.Skip("AGENT_TEST_MYSQL_URL not set")
	}
	testSQL(t, monitor.TypeMySQL, url, "MySQL")
}

func testSQL(t *testing.T, typ monitor.Type, url, server string) {
	out := runCheck(dbMonitor(t, typ, url, monitor.Config{}))
	if !out.OK || !strings.Contains(out.Message, server) {
		t.Fatalf("default query: %+v", out)
	}
	out = runCheck(dbMonitor(t, typ, url, monitor.Config{Query: "SELECT 1 + 1", Expected: "2"}))
	if !out.OK {
		t.Fatalf("expected value: %+v", out)
	}
	out = runCheck(dbMonitor(t, typ, url, monitor.Config{Query: "SELECT 1", Expected: "0"}))
	if out.OK || !strings.Contains(out.Message, `got "1"`) {
		t.Fatalf("wrong value: %+v", out)
	}
	for _, q := range []string{
		"CREATE TABLE uptimy_agent_should_not_exist (id int)",
		"SELECT 1; COMMIT; CREATE TABLE uptimy_agent_should_not_exist (id int)",
	} {
		if out = runCheck(dbMonitor(t, typ, url, monitor.Config{Query: q})); out.OK {
			t.Fatalf("write succeeded (%s): %+v", q, out)
		}
	}
	bad := strings.Replace(url, "://", "://nobody:wrongpass@", 1)
	if i := strings.Index(url, "@"); i >= 0 {
		bad = url[:strings.Index(url, "://")+3] + "nobody:wrongpass" + url[i:]
	}
	out = runCheck(dbMonitor(t, typ, bad, monitor.Config{}))
	if out.OK || strings.Contains(out.Message, "wrongpass") {
		t.Fatalf("bad login: %+v", out)
	}
	t.Logf("bad login message: %s", out.Message)
}
