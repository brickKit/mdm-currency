//go:build integration

package itest

// The currency component against PostgreSQL and the bus over its real surfaces (L2): the ISO table from the
// migration, the user surface with tokens of be-sdk-go's sectest deployment, the system surface over mTLS, the
// events in the outbox. Needs BE_SDK_TEST_PG_ADMIN (a postgres superuser URL of the test database) and
// BE_SDK_TEST_NATS_URL, as be-sdk-go's own integration tests; missing, it fails. The member listens on 28226 (user)
// and 9226 (system). Run: go test -tags integration ./itest/ (make test-integration); the index of the steps is
// itest/AGENTS.md.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	currency "github.com/brickKit/mdm-currency/v3/currency"

	core "github.com/brickKit/mdm-currency/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

const (
	itPort   = "28226"
	itBase   = "http://localhost:" + itPort
	itSystem = "localhost:9226"
	itSchema = "mdm_currency_it"
)

type client struct {
	t     *testing.T
	token string
}

func (c client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, itBase+path, r)
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// fixture is the running member and what one step leaves for the next.
type fixture struct {
	t          *testing.T
	ctx        context.Context
	admin      *pgx.Conn
	w          *sectest.World
	env        map[string]string
	alice, bob client
	r1, r2     map[string]any // the EUR/CNY spot rate of 2026-10-15, entered and corrected (rates)
}

func TestCurrencyEndToEnd(t *testing.T) {
	fx := start(t)
	fx.currencies()
	fx.rates()
	fx.outbox()
	fx.source()
	fx.recoveryChecks()
}

// start makes the world, migrates and runs the member.
func start(t *testing.T) *fixture {
	adminURL, natsURL := os.Getenv("BE_SDK_TEST_PG_ADMIN"), os.Getenv("BE_SDK_TEST_NATS_URL")
	if adminURL == "" || natsURL == "" {
		t.Fatal("BE_SDK_TEST_PG_ADMIN and BE_SDK_TEST_NATS_URL are needed")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminURL)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close(ctx) })
	_, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+itSchema+" CASCADE; CREATE SCHEMA "+itSchema)
	require.NoError(t, err)
	u, err := url.Parse(adminURL)
	require.NoError(t, err)
	pw, _ := u.User.Password()

	w := sectest.New(t)
	w.GrantAll(sectest.Alice, core.Definitions())
	portFree(t, itPort, itSystem)
	env := w.Env(t, core.ID)
	for k, v := range map[string]string{"COMPONENT_ID": core.ID, "COMPONENT_VERSION": currency.Version, "PORT": itPort,
		"STOP_DRAIN_DELAY": "50ms", "PG_HOST": u.Hostname(), "PG_PORT": u.Port(), "PG_DATABASE": strings.TrimPrefix(u.Path, "/"),
		"PG_USER": u.User.Username(), "PG_PASSWORD": pw, "PG_SCHEMA": itSchema, "PG_OWNER_USER": u.User.Username(),
		"PG_OWNER_PASSWORD": pw, "BUS_URL": natsURL, "IMPLEMENTATION_INSTANCE": ids.New().String()} {
		env[k] = v
	}
	var logs bytes.Buffer
	opts := launcher.Options{Stdout: &logs, Stderr: &logs,
		Env: config.Env{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }, Read: os.ReadFile}}
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(logs.String())
		}
	})
	migrate := opts
	migrate.Args = []string{"migrate"}
	require.Equal(t, launcher.ExitOK, launcher.Run(ctx, currency.Definition(), migrate))
	sctx, stop := context.WithCancel(ctx)
	done := make(chan int, 1)
	go func() { done <- launcher.Run(sctx, currency.Definition(), opts) }()
	t.Cleanup(func() { stop(); <-done })
	require.Eventually(t, func() bool {
		r, err := http.Get(itBase + "/readyz")
		if err == nil {
			r.Body.Close()
		}
		return err == nil && r.StatusCode == 200
	}, 20*time.Second, 100*time.Millisecond)
	return &fixture{t: t, ctx: ctx, admin: admin, w: w, env: env, alice: client{t, w.Bearer(sectest.Alice)},
		bob: client{t, w.Bearer(sectest.Bob)}}

}

// portFree fails at once when a port the member needs is taken: a running cell (host ports 18xxx) or another member
// would answer in its place, and the test would assert against another process.
func portFree(t *testing.T, addrs ...string) {
	for _, a := range addrs {
		if !strings.Contains(a, ":") {
			a = "localhost:" + a
		}
		ln, err := net.Listen("tcp", a)
		require.NoError(t, err, "port %s is taken: is a cell or another member running?", a)
		_ = ln.Close()
	}
}
