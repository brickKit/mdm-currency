// Command mdm-currency runs the currency component alone (serve, migrate); the shells compile in currency.Definition().
package main

import (
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/mdm-currency/v3/currency"
)

func main() { launcher.Main(currency.Definition()) }
