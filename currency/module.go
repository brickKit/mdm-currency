package currency

import (
	"context"
	"time"

	"github.com/brickKit/be-sdk-go/v3/grpcx"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/runtime"
	"github.com/brickKit/be-sdk-go/v3/work"
	currencyv3 "github.com/brickKit/mdm-currency/contract/v3/gen/go/be/mdm/currency/v3"
	core "github.com/brickKit/mdm-currency/v3/internal/core"
	currencies "github.com/brickKit/mdm-currency/v3/internal/currencies"
	rates "github.com/brickKit/mdm-currency/v3/internal/rates"
)

// Module is the currency component as the launcher and the shells run it: it only assembles the business modules
// under internal/ (their map is internal/AGENTS.md); the gRPC services answer through them (services.go).
type Module struct {
	currencyv3.UnimplementedSourceServiceServer
	currencyv3.UnimplementedRateServiceServer
	b *core.Base
}

// New builds the module from its runtime.
func New(rt *runtime.Runtime) (*Module, error) {
	return &Module{b: &core.Base{RT: rt, Deployment: rt.Config.String("DEPLOYMENT_ID"),
		Instance: rt.Config.String("IMPLEMENTATION_INSTANCE")}}, nil
}

// Routes registers the user surface (contract/openapi.yaml).
func (m *Module) Routes(r *httpx.Routes) error {
	h := m.handler
	r.Add(httpx.Route{Method: "GET", Path: "/api/currency/currencies", Operation: core.OpRead, Handle: h(currencies.ListCurrencies)})
	r.Add(httpx.Route{Method: "GET", Path: "/api/currency/currencies/:code", Operation: core.OpRead, Handle: h(currencies.GetCurrency)})
	r.Add(httpx.Route{Method: "PATCH", Path: "/api/currency/currencies/:code", Operation: core.OpManage, Handle: h(currencies.UpdateCurrency)})
	r.Add(httpx.Route{Method: "GET", Path: "/api/currency/rate-types", Operation: core.OpRead, Handle: h(rates.ListRateTypes)})
	r.Add(httpx.Route{Method: "PUT", Path: "/api/currency/rate-types/:code", Operation: core.OpManage, Handle: h(rates.PutRateType)})
	r.Add(httpx.Route{Method: "GET", Path: "/api/currency/rates", Operation: core.OpRateRead, Handle: h(rates.ListRates)})
	r.Add(httpx.Route{Method: "GET", Path: "/api/currency/rates/effective", Operation: core.OpRateRead, Handle: h(rates.EffectiveHTTP)})
	r.Add(httpx.Route{Method: "PUT", Path: "/api/currency/rates/:rate_type/:date/:from_currency/:to_currency", Operation: core.OpMaintain, Handle: h(rates.PutRate)})
	return nil
}

// Services registers the system surface: every component of the deployment reads master data (boundary cards: master
// data is read by all business components).
func (m *Module) Services(s *grpcx.Services) error {
	s.Register(&currencyv3.SourceService_ServiceDesc, m, map[string]grpcx.Method{
		"Snapshot": {Operation: core.OpSystem, Callers: []string{grpcx.AnyComponent}},
		"Read":     {Operation: core.OpSystem, Callers: []string{grpcx.AnyComponent}}})
	s.Register(&currencyv3.RateService_ServiceDesc, m, map[string]grpcx.Method{
		"EffectiveRate": {Operation: core.OpSystem, Callers: []string{grpcx.AnyComponent}}})
	return nil
}

// Jobs declares the search backfill: currencies written by a migration (the ISO table) have no search column yet,
// and a newer normalization version of the SDK rewrites the old ones (protocol 14).
func (m *Module) Jobs(j *work.Jobs) error {
	j.Add(work.Job{Name: "mdm.currency.search_backfill", Kind: work.Singleton, Effect: work.Local, Interval: 10 * time.Minute,
		MaxAttempts: work.Unlimited, Handle: func(ctx context.Context, _ *work.Run) error { return currencies.Backfill(m.b, ctx) }})
	return nil
}

// handler adapts a module's handler, which takes the shared base first, to what the SDK registers.
func (m *Module) handler(f func(*core.Base, *httpx.Context) error) httpx.Handler {
	return func(c *httpx.Context) error { return f(m.b, c) }
}
