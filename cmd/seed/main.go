// Command seed fills a running mdm/currency with the seed data for people (be-assembly-standard's seed and test data
// conventions): the rate type budget beside spot; spot rates for six pairs on every day of the last 60 (relative to
// today, so a rerun never drifts), one of them corrected (two versions); monthly budget rates; a currency switched off
// (negative data) and one with cash rounding. Rates are keyed by type, date and pair, so a second run changes nothing.
//
//	SEED_IAM_URL (http://localhost:18200)  the identity member's development exchange
//	SEED_USER (test.bob), SEED_PASSWORD     a test user granted mdm/currency's operations
//	MDM_CURRENCY_URL (http://localhost:18226)
//
// Run: go run ./cmd/seed (mdm/currency has no seed dependencies).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"
)

const prefix = "「本地测试」"

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type client struct{ base, token string }

func (c client) do(method, path string, body any) (int, map[string]any) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (c client) must(method, path string, body any) map[string]any {
	code, out := c.do(method, path, body)
	if code != 200 {
		fail("%s %s: %d %v", method, path, code, out)
	}
	return out
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "seed mdm/currency: "+format+"\n", a...)
	os.Exit(1)
}

func token() string {
	pw := os.Getenv("SEED_PASSWORD")
	if pw == "" {
		fail("SEED_PASSWORD is not set (the test users' password in Casdoor)")
	}
	b, _ := json.Marshal(map[string]string{"username": env("SEED_USER", "test.bob"), "password": pw})
	resp, err := http.Post(env("SEED_IAM_URL", "http://localhost:18200")+"/api/iam/dev/token", "application/json", bytes.NewReader(b))
	if err != nil {
		fail("the identity member is not reachable: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&out) != nil {
		fail("the development exchange refused the seed user: %d", resp.StatusCode)
	}
	return out.AccessToken
}

func main() {
	base := env("MDM_CURRENCY_URL", "http://localhost:18226")
	if r, err := http.Get(base + "/readyz"); err != nil || r.StatusCode != 200 { // probe before pouring
		fail("mdm/currency at %s is not ready", base)
	}
	c := client{base, token()}
	c.must("PUT", "/api/currency/rate-types/spot", map[string]any{"name": "Spot", "names": map[string]string{"zh-CN": "即期"}})
	c.must("PUT", "/api/currency/rate-types/budget", map[string]any{"name": prefix + "Budget", "names": map[string]string{"zh-CN": prefix + "预算"}})
	c.must("PATCH", "/api/currency/currencies/CHF", map[string]any{"cash_unit": "0.05"})
	c.must("PATCH", "/api/currency/currencies/VES", map[string]any{"active": false})
	pairs := []struct {
		from, to string
		rate     float64
	}{{"EUR", "CNY", 7.65}, {"USD", "CNY", 7.12}, {"EUR", "USD", 1.074}, {"JPY", "CNY", 0.0476}, {"HKD", "CNY", 0.9105},
		{"GBP", "EUR", 1.172}}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	first := today.AddDate(0, 0, -59)
	for first.Weekday() == time.Saturday || first.Weekday() == time.Sunday {
		first = first.AddDate(0, 0, 1)
	}
	// the corrected rate exists after the first run: then neither it nor its original is written again
	_, eff := c.do("GET", "/api/currency/rates/effective?rate_type=spot&date="+first.Format("2006-01-02")+
		"&from_currency=EUR&to_currency=CNY", nil)
	corrected := eff["rate_date"] == first.Format("2006-01-02")
	n := 0
	for d := 59; d >= 0; d-- {
		day := today.AddDate(0, 0, -d)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue // no fixing at weekends: the effective rate falls back to Friday's
		}
		for i, p := range pairs {
			if corrected && day.Equal(first) && p.from == "EUR" && p.to == "CNY" {
				n++ // there, corrected by an earlier run
				continue
			}
			// a gentle, repeatable wave around the base rate (the same date always gives the same rate)
			r := p.rate * (1 + 0.01*math.Sin(float64(day.YearDay()+7*i)/9))
			c.must("PUT", fmt.Sprintf("/api/currency/rates/spot/%s/%s/%s", day.Format("2006-01-02"), p.from, p.to),
				map[string]any{"rate": fmt.Sprintf("%.6f", r), "source": "seed"})
			n++
		}
	}
	// a correction: the first rate of the period entered again with another value (version 2), once
	if !corrected {
		c.must("PUT", fmt.Sprintf("/api/currency/rates/spot/%s/EUR/CNY", first.Format("2006-01-02")),
			map[string]any{"rate": "7.7001", "source": "seed-correction"})
	}
	for m := 0; m < 3; m++ {
		month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -m, 0)
		c.must("PUT", fmt.Sprintf("/api/currency/rates/budget/%s/EUR/CNY", month.Format("2006-01-02")),
			map[string]any{"rate": "7.60", "source": "seed"})
		n++
	}
	fmt.Printf("seed mdm/currency: 2 rate types, %d rates, CHF cash 0.05, VES off\n", n)
}
