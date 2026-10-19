//go:build integration

package currency

// The currency component against PostgreSQL and the bus over its real surfaces (L2): the ISO table from the
// migration, the user surface with tokens of be-sdk-go's sectest deployment, the system surface over mTLS, the
// events in the outbox. Needs BE_SDK_TEST_PG_ADMIN (a postgres superuser URL of the test database) and
// BE_SDK_TEST_NATS_URL, as be-sdk-go's own integration tests; missing, it fails. The member listens on 18226 (user)
// and 9226 (system). Run: go test -tags integration ./currency/

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/money"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	currencyv3 "github.com/brickKit/mdm-currency/contract/v3/gen/go/be/mdm/currency/v3"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	itPort   = "18226"
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

func TestCurrencyEndToEnd(t *testing.T) {
	adminURL, natsURL := os.Getenv("BE_SDK_TEST_PG_ADMIN"), os.Getenv("BE_SDK_TEST_NATS_URL")
	if adminURL == "" || natsURL == "" {
		t.Fatal("BE_SDK_TEST_PG_ADMIN and BE_SDK_TEST_NATS_URL are needed")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminURL)
	require.NoError(t, err)
	defer admin.Close(ctx)
	_, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+itSchema+" CASCADE; CREATE SCHEMA "+itSchema)
	require.NoError(t, err)
	u, err := url.Parse(adminURL)
	require.NoError(t, err)
	pw, _ := u.User.Password()

	w := sectest.New(t)
	w.GrantAll(sectest.Alice, Definitions())
	env := w.Env(t, ID)
	for k, v := range map[string]string{"COMPONENT_ID": ID, "COMPONENT_VERSION": Version, "PORT": itPort,
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
	require.Equal(t, launcher.ExitOK, launcher.Run(ctx, Definition(), migrate))
	sctx, stop := context.WithCancel(ctx)
	done := make(chan int, 1)
	go func() { done <- launcher.Run(sctx, Definition(), opts) }()
	defer func() { stop(); <-done }()
	require.Eventually(t, func() bool {
		r, err := http.Get(itBase + "/readyz")
		if err == nil {
			r.Body.Close()
		}
		return err == nil && r.StatusCode == 200
	}, 20*time.Second, 100*time.Millisecond)
	alice, bob := client{t, w.Bearer(sectest.Alice)}, client{t, w.Bearer(sectest.Bob)}

	// the ISO table comes with the migration; three decimals for KWD, none for JPY
	code, kwd := alice.do("GET", "/api/currency/currencies/KWD", nil)
	require.Equal(t, 200, code, kwd)
	assert.EqualValues(t, 3, kwd["minor_unit"])
	_, jpy := alice.do("GET", "/api/currency/currencies/JPY", nil)
	assert.EqualValues(t, 0, jpy["minor_unit"])
	// the boundary card's story: a three-decimal currency rounds to its minor unit
	d, err := money.Parse("12.34567")
	require.NoError(t, err)
	assert.Equal(t, "12.346", money.Round(d, int32(kwd["minor_unit"].(float64)), money.Mode(kwd["rounding"].(string))).Text('f'))

	code, _ = bob.do("GET", "/api/currency/currencies/KWD", nil)
	assert.Equal(t, 404, code, "Bob holds no grant: not visible (P8.16)")

	// a deployment's decisions: cash rounding, translations; a stale version is refused; the same change twice once
	code, chf := alice.do("PATCH", "/api/currency/currencies/CHF", map[string]any{"cash_unit": "0.10",
		"names": map[string]string{"zh-CN": "瑞士法郎", "de": "Schweizer Franken"}})
	require.Equal(t, 200, code, chf)
	assert.Equal(t, "0.1", chf["cash_unit"], "no trailing zeros")
	assert.EqualValues(t, 2, chf["version"], "the ISO row was version 1")
	v := chf["version"]
	code, again := alice.do("PATCH", "/api/currency/currencies/CHF", map[string]any{"cash_unit": "0.1"})
	assert.Equal(t, 200, code)
	assert.Equal(t, v, again["version"], "nothing changed: no new version")
	code, stale := alice.do("PATCH", "/api/currency/currencies/CHF", map[string]any{"version": 1, "active": false})
	assert.Equal(t, 409, code)
	assert.Equal(t, "VERSION_CONFLICT", stale["reason"])

	// the keyword: the translated name, its pinyin initials; pages by cursor
	require.Eventually(t, func() bool { // the backfill job writes the ISO rows' search column
		_, out := alice.do("GET", "/api/currency/currencies?q=dinar&limit=200", nil)
		l, _ := out["currencies"].([]any)
		return len(l) >= 5
	}, 20*time.Second, 200*time.Millisecond)
	_, out := alice.do("GET", "/api/currency/currencies?q=rsfl", nil) // 瑞士法郎: r s f l
	l, _ := out["currencies"].([]any)
	require.Len(t, l, 1)
	assert.Equal(t, "CHF", l[0].(map[string]any)["code"])
	seen, cursor := map[string]bool{}, ""
	for pages := 0; ; pages++ {
		require.Less(t, pages, 20)
		_, out := alice.do("GET", "/api/currency/currencies?limit=50&cursor="+url.QueryEscape(cursor), nil)
		for _, c := range out["currencies"].([]any) {
			code := c.(map[string]any)["code"].(string)
			assert.False(t, seen[code], "no currency twice across pages")
			seen[code] = true
		}
		if cursor = out["next_cursor"].(string); cursor == "" {
			break
		}
	}
	var total int
	require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".currency").Scan(&total))
	assert.Len(t, seen, total, "every currency once")

	// rates: one per type, date and pair; the same again changes nothing; a correction is a new version
	code, spot := alice.do("PUT", "/api/currency/rate-types/spot", map[string]any{"name": "Spot"})
	require.Equal(t, 200, code, spot)
	code, r1 := alice.do("PUT", "/api/currency/rates/spot/2026-10-15/EUR/CNY", map[string]any{"rate": "7.65"})
	require.Equal(t, 200, code, r1)
	_, r1b := alice.do("PUT", "/api/currency/rates/spot/2026-10-15/EUR/CNY", map[string]any{"rate": "7.6500"})
	assert.Equal(t, r1["version"], r1b["version"], "7.6500 is 7.65")
	_, r2 := alice.do("PUT", "/api/currency/rates/spot/2026-10-15/EUR/CNY", map[string]any{"rate": "7.66"})
	assert.Equal(t, r1["id"], r2["id"], "the same record")
	assert.EqualValues(t, 2, r2["version"])
	code, _ = alice.do("PUT", "/api/currency/rates/spot/2026-10-18/EUR/CNY", map[string]any{"rate": "7.70"})
	require.Equal(t, 200, code)
	code, eff := alice.do("GET", "/api/currency/rates/effective?rate_type=spot&date=2026-10-17&from_currency=EUR&to_currency=CNY", nil)
	require.Equal(t, 200, code, eff)
	assert.Equal(t, "7.66", eff["rate"])
	assert.Equal(t, "2026-10-15", eff["rate_date"], "the latest date not after the requested one")
	code, _ = alice.do("GET", "/api/currency/rates/effective?rate_type=spot&date=2026-10-14&from_currency=EUR&to_currency=CNY", nil)
	assert.Equal(t, 404, code, "no rate that early")
	_, _ = alice.do("PATCH", "/api/currency/currencies/BTN", map[string]any{"active": false})
	code, inactive := alice.do("PUT", "/api/currency/rates/spot/2026-10-15/EUR/BTN", map[string]any{"rate": "30"})
	assert.Equal(t, 409, code)
	assert.Equal(t, "INACTIVE", inactive["reason"])
	_, rates := alice.do("GET", "/api/currency/rates?from_currency=EUR&to_currency=CNY&limit=1", nil)
	first := rates["rates"].([]any)
	require.Len(t, first, 1)
	assert.Equal(t, "2026-10-18", first[0].(map[string]any)["rate_date"], "latest first")
	_, rest := alice.do("GET", "/api/currency/rates?from_currency=EUR&to_currency=CNY&limit=1&cursor="+
		url.QueryEscape(rates["next_cursor"].(string)), nil)
	assert.Equal(t, "2026-10-15", rest["rates"].([]any)[0].(map[string]any)["rate_date"])

	// each change is a full-state event in the outbox, in the same transaction
	var n int
	require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".be_outbox WHERE record_id = $1 AND type = $2",
		r1["id"], RatePublished).Scan(&n))
	assert.Equal(t, 2, n, "entered, corrected; the same rate again published nothing")
	require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".be_audit_log").Scan(&n))
	assert.Positive(t, n, "the changes are audited")

	// the system surface over mTLS
	tls := w.TLS(t, "erp/sales")
	conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(tls.ClientConfig(ID))))
	require.NoError(t, err)
	defer conn.Close()
	src := currencyv3.NewSourceServiceClient(conn)
	read, err := src.Read(ctx, &currencyv3.ReadRequest{Aggregate: currencyv3.Aggregate_AGGREGATE_CURRENCY,
		Ids: []string{"CHF", "XXQ"}})
	require.NoError(t, err)
	require.Len(t, read.Results, 2)
	assert.Equal(t, currencyv3.ReadState_READ_STATE_EXISTS, read.Results[0].State)
	assert.Contains(t, string(read.Results[0].Data), `"cash_unit":"0.1"`)
	assert.Equal(t, currencyv3.ReadState_READ_STATE_PURGED, read.Results[1].State, "never existed: purged, not an error")
	assert.Equal(t, env["IMPLEMENTATION_INSTANCE"], read.Stamp.Instance)
	assert.Len(t, read.Stamp.Positions, 4)
	assert.Regexp(t, `^[0-9]+\.[0-9]+$`, read.Stamp.Epoch)
	snap, err := src.Snapshot(ctx, &currencyv3.SnapshotRequest{Aggregate: currencyv3.Aggregate_AGGREGATE_CURRENCY, Limit: 100})
	require.NoError(t, err)
	assert.Len(t, snap.Records, 100)
	assert.NotEmpty(t, snap.NextCursor)
	_, err = src.Read(ctx, &currencyv3.ReadRequest{Aggregate: currencyv3.Aggregate_AGGREGATE_CURRENCY,
		Ids: make([]string, MaxBatch+1)})
	e, _, ok := errs.FromGRPC(err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, errs.ReasonBatchTooLarge, e.Reason, "more than 500 IDs: refused, not cut")
	rate, err := currencyv3.NewRateServiceClient(conn).EffectiveRate(ctx, &currencyv3.EffectiveRateRequest{
		RateType: "spot", Date: "2026-10-30", FromCurrency: "EUR", ToCurrency: "CNY"})
	require.NoError(t, err)
	assert.Equal(t, "7.7", rate.Rate)
	assert.Equal(t, "2026-10-18", rate.RateDate)
}
