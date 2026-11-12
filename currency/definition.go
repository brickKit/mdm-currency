// Package currency is mdm/currency (boundary card mdm/currency): currencies (ISO 4217, with their minor unit,
// rounding and cash unit), rate types and daily exchange rates, kept by people over the user surface, read by
// components over the system surface (the source methods of protocol 02 and the effective rate), and published as
// full-state events. Step 6 of the representative deployment experiment: no rate source adapter yet (D75).
package currency

import (
	"embed"
	"io/fs"

	core "github.com/brickKit/mdm-currency/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/component"
	"github.com/brickKit/be-sdk-go/v3/events"
	"github.com/brickKit/be-sdk-go/v3/runtime"
)

// Identity of the component.
const (
	Version  = "3.0.38"
	GRPCPort = 9226
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func migrations() fs.FS {
	sub, _ := fs.Sub(migrationFiles, "migrations")
	return sub
}

// Definition is the component as the launcher and the shells take it.
func Definition() component.Definition {
	return component.Definition{ID: core.ID, Version: Version, Migrations: migrations(), GRPCPort: GRPCPort,
		Streams: []events.Stream{{Name: core.Stream, Partitions: 4}}, Authz: core.Definitions(), Resend: core.OpResend, Recovery: core.OpRecover,
		New: func(rt *runtime.Runtime) (component.Module, error) { return New(rt) }}
}
