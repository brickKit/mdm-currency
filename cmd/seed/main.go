// Command seed fills a running mdm/currency with the seed data for people (be-assembly-standard's seed and test data
// conventions):
//
//   - access: the roles of mdm/currency's operations granted to the seed accounts of infra/iam-casdoor's seed by the
//     authorization admin: maintain (the seed user, test.mdm.admin), rates (test.rates: maintains rates, cannot change
//     a currency), read (the other readers). The seed user waits until mdm/currency takes its grant;
//   - the rate type budget beside spot;
//   - spot rates on every weekday of the last 60 days (relative to today, so a rerun never drifts) for the pairs the
//     seeded legal entities need, each in both directions (the component does not derive an inverse: CNY/USD is
//     1/USD/CNY of the same day), one of them corrected (two versions); monthly budget rates;
//   - a currency switched off (negative data) and one with cash rounding.
//
// Rates are keyed by type, date and pair and change keys are fixed, so a second run changes nothing.
//
//	SEED_IAM_URL (http://localhost:18200)    the identity member's development exchange
//	SEED_AUTHZ_URL (http://localhost:18223)  the authorization member's effect point
//	SEED_ADMIN (test.alice)                  the authorization admin (the install claim)
//	SEED_USER (test.bob), SEED_PASSWORD      the user that writes the data; the test accounts' password
//	MDM_CURRENCY_URL (http://localhost:18226)
//
// Run: go run ./cmd/seed (the component and the security members up; mdm/currency has no seed dependencies).
package main

import (
	"fmt"
	"math"
	"os"
	"time"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/seed"
)

const prefix = "「本地测试」"

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "seed mdm/currency: "+format+"\n", a...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fail("%v", err)
	}
}

type client struct{ seed.Client }

func (c client) do(method, path string, body any) (int, map[string]any) {
	code, out, err := c.Do(method, path, body)
	check(err)
	return code, out
}

func (c client) must(method, path string, body any) map[string]any {
	out, err := c.Must(method, path, body)
	check(err)
	return out
}

// readers are the seed accounts that read currencies and rates.
var readers = []string{"test.mdm.viewer", "test.sales.cn", "test.sales.es", "test.sales.multi", "test.unit.admin.cn"}

// access grants mdm/currency's roles and answers the seed user's client once its grant has reached mdm/currency.
func access(base string) client {
	pw := os.Getenv("SEED_PASSWORD")
	if pw == "" {
		fail("SEED_PASSWORD is not set (the test accounts' password in Casdoor)")
	}
	iam, user, admin := env("SEED_IAM_URL", "http://localhost:18200"), env("SEED_USER", "test.bob"), env("SEED_ADMIN", "test.alice")
	accounts, err := seed.Accounts(iam, pw, append([]string{admin, user, "test.mdm.admin", "test.rates"}, readers...)...)
	check(err)
	a, err := seed.NewAuthz(env("SEED_AUTHZ_URL", "http://localhost:18223"), accounts[admin].Token, "mdm/currency")
	check(err)
	read := []sdkauthz.Segment{seed.Segment("mdm.currency", "mdm.currency.read"), seed.Segment("mdm.exchange_rate", "mdm.exchange_rate.read")}
	_, err = a.GrantRoles([]seed.RoleSpec{
		{Key: "maintain", Name: prefix + "币种与汇率维护", To: []string{user, "test.mdm.admin"}, Segments: append(read,
			seed.Segment("mdm.currency", "mdm.currency.manage"), seed.Segment("mdm.exchange_rate", "mdm.exchange_rate.maintain"))},
		{Key: "rates", Name: prefix + "汇率维护", To: []string{"test.rates"}, Segments: append(read,
			seed.Segment("mdm.exchange_rate", "mdm.exchange_rate.maintain"))},
		{Key: "read", Name: prefix + "币种与汇率只读", To: readers, Segments: read},
	}, accounts)
	check(err)
	c := client{seed.NewClient(base, accounts[user].Token)}
	check(c.WaitGranted("PUT", "/api/currency/rate-types/x", 30*time.Second))
	return c
}

func main() {
	base := env("MDM_CURRENCY_URL", "http://localhost:18226")
	if !seed.Ready(base) { // probe before pouring
		fail("mdm/currency at %s is not ready", base)
	}
	c := access(base)
	c.must("PUT", "/api/currency/rate-types/spot", map[string]any{"name": "Spot", "names": map[string]string{"zh-CN": "即期"}})
	c.must("PUT", "/api/currency/rate-types/budget", map[string]any{"name": prefix + "Budget", "names": map[string]string{"zh-CN": prefix + "预算"}})
	c.must("PATCH", "/api/currency/currencies/CHF", map[string]any{"cash_unit": "0.05"})
	c.must("PATCH", "/api/currency/currencies/VES", map[string]any{"active": false})
	// each pair is entered in both directions; its index shifts the pair's wave
	pairs := []struct {
		from, to string
		rate     float64
	}{{"EUR", "CNY", 7.65}, {"USD", "CNY", 7.12}, {"EUR", "USD", 1.074}, {"JPY", "CNY", 0.0476}, {"HKD", "CNY", 0.9105},
		{"GBP", "EUR", 1.172}, {"USD", "HKD", 7.82}, {"GBP", "USD", 1.258}}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	first := today.AddDate(0, 0, -59)
	for first.Weekday() == time.Saturday || first.Weekday() == time.Sunday {
		first = first.AddDate(0, 0, 1)
	}
	// the corrected rate exists after the first run: then neither it nor its original is written again
	_, eff := c.do("GET", "/api/currency/rates/effective?rate_type=spot&date="+first.Format("2006-01-02")+
		"&from_currency=EUR&to_currency=CNY", nil)
	corrected := eff["rate_date"] == first.Format("2006-01-02")
	put := func(typ string, day time.Time, from, to, rate, source string) {
		c.must("PUT", fmt.Sprintf("/api/currency/rates/%s/%s/%s/%s", typ, day.Format("2006-01-02"), from, to),
			map[string]any{"rate": rate, "source": source})
	}
	n := 0
	for d := 59; d >= 0; d-- {
		day := today.AddDate(0, 0, -d)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue // no fixing at weekends: the effective rate falls back to Friday's
		}
		for i, p := range pairs {
			// a gentle, repeatable wave around the base rate (the same date always gives the same rate)
			r := p.rate * (1 + 0.01*math.Sin(float64(day.YearDay()+7*i)/9))
			if !(corrected && day.Equal(first) && p.from == "EUR" && p.to == "CNY") { // there, corrected by an earlier run
				put("spot", day, p.from, p.to, fmt.Sprintf("%.6f", r), "seed")
			}
			put("spot", day, p.to, p.from, fmt.Sprintf("%.8f", 1/r), "seed")
			n += 2
		}
	}
	// a correction: the first rate of the period entered again with another value (version 2), once
	if !corrected {
		put("spot", first, "EUR", "CNY", "7.7001", "seed-correction")
	}
	for m := 0; m < 3; m++ {
		month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -m, 0)
		for _, b := range [][3]string{{"EUR", "CNY", "7.60"}, {"USD", "CNY", "7.10"}, {"CNY", "USD", "0.14084507"}} {
			put("budget", month, b[0], b[1], b[2], "seed")
			n++
		}
	}
	fmt.Printf("seed mdm/currency: 2 rate types, %d rates (%d pairs both ways), CHF cash 0.05, VES off; roles maintain, rates, read\n",
		n, len(pairs))
}
