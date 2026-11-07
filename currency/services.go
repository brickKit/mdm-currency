package currency

import (
	"context"

	currencyv3 "github.com/brickKit/mdm-currency/contract/v3/gen/go/be/mdm/currency/v3"
	source "github.com/brickKit/mdm-currency/v3/internal/source"
)

// The gRPC methods of the system surface answer through internal/source.

// Snapshot is SourceService.Snapshot.
func (m *Module) Snapshot(ctx context.Context, in *currencyv3.SnapshotRequest) (*currencyv3.SnapshotResponse, error) {
	return source.Snapshot(m.b, ctx, in)
}

// Read is SourceService.Read.
func (m *Module) Read(ctx context.Context, in *currencyv3.ReadRequest) (*currencyv3.ReadResponse, error) {
	return source.Read(m.b, ctx, in)
}

// EffectiveRate is RateService.EffectiveRate.
func (m *Module) EffectiveRate(ctx context.Context, in *currencyv3.EffectiveRateRequest) (*currencyv3.EffectiveRateResponse, error) {
	return source.EffectiveRate(m.b, ctx, in)
}
