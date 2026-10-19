package currency

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/mergesafety"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTheManifestDeclaresWhatTheSDKReads: component.yaml's configSchema is exactly the keys the SDK reads.
func TestTheManifestDeclaresWhatTheSDKReads(t *testing.T) {
	raw, err := os.ReadFile("../component.yaml")
	require.NoError(t, err)
	if p := launcher.ManifestProblems(raw, Definition(), nil); len(p) > 0 {
		t.Errorf("component.yaml's configSchema differs from what the SDK reads:\n%v\nthe block:\n%s", p,
			launcher.ConfigSchemaYAML(Definition(), nil))
	}
}

// TestTheRoutesAreTheOpenAPIDocument: every route is in contract/openapi.yaml with its operation, and nothing else is.
func TestTheRoutesAreTheOpenAPIDocument(t *testing.T) {
	raw, err := os.ReadFile("../contract/openapi.yaml")
	require.NoError(t, err)
	var r httpx.Routes
	require.NoError(t, (&Module{}).Routes(&r))
	assert.Empty(t, httpx.OpenAPIProblems(raw, r.List()))
}

// TestThePayloadsFitTheirSchemas: what events and source reads carry is what contract/events promises.
func TestThePayloadsFitTheirSchemas(t *testing.T) {
	cash := "0.05"
	for typ, v := range map[string]any{
		CurrencyChanged: Currency{Code: "CHF", NumericCode: "756", MinorUnit: 2, Rounding: "half_away_from_zero",
			CashUnit: &cash, Active: true, Name: "Swiss Franc", Names: map[string]string{"zh-CN": "瑞士法郎"}, Version: 3,
			UpdatedAt: "2026-10-20T08:00:00.000000Z"},
		RateTypeChanged: RateType{Code: "spot", Active: true, Name: "Spot", Names: map[string]string{}, Version: 1,
			UpdatedAt: "2026-10-20T08:00:00.000000Z"},
		RatePublished: Rate{ID: "01a15ab0-0000-7000-8000-000000000001", RateType: "spot", RateDate: "2026-10-20",
			FromCurrency: "EUR", ToCurrency: "CNY", Rate: "7.6543", Source: "manual", Version: 2,
			UpdatedAt: "2026-10-20T08:00:00.000000Z"},
	} {
		c := jsonschema.NewCompiler()
		s, err := c.Compile("../contract/events/" + typ + ".schema.json")
		require.NoError(t, err, typ)
		b, err := json.Marshal(v)
		require.NoError(t, err)
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		require.NoError(t, err)
		assert.NoError(t, s.Validate(doc), typ)
	}
}

func TestPositiveDecimals(t *testing.T) {
	for s, want := range map[string]bool{"7.6543210001": true, "1": true, "0.05": true, "0": false, "-1": false,
		"1.12345678901": false, "1e3": false, "": false, "01": false} {
		assert.Equal(t, want, positive(s, 10), s)
	}
}

// TestTheModuleIsMergeSafe: the module is compiled into the Go infra shell.
func TestTheModuleIsMergeSafe(t *testing.T) {
	findings, err := mergesafety.CheckDir(".")
	require.NoError(t, err)
	for _, f := range findings {
		t.Errorf("%v", f)
	}
}
