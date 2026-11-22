package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	iofs "io/fs"
	"strings"
	"sync"
	"time"

	"github.com/hxkhan/evie"
)

//go:embed scripts
var embeddedScripts embed.FS

type Quote struct {
	Customer string  `json:"customer"`
	Total    float64 `json:"total"`
}

// Sandboxed, parse-once embedding: scripts live inside the Go binary via
// embed.FS, are parsed once into an immutable Bundle, and run concurrently
// across isolated engines with restricted stdlib, captured stdout, and timeouts.
func main() {
	scriptsFS, err := iofs.Sub(embeddedScripts, "scripts")
	if err != nil {
		panic(err)
	}

	// Parse and validate the whole dependency graph once (no VM execution yet).
	bundle, err := evie.ParseProject(scriptsFS, ".")
	if err != nil {
		fmt.Print(evie.Format(err, false))
		panic(err)
	}
	fmt.Printf("loaded bundle: packages=%v files=%v\n", bundle.Packages(), bundle.Files())

	orders := []struct {
		customer string
		subtotal float64
	}{
		{"Alice", 80},
		{"Bob", 150},
		{"Cara", 240},
	}

	type outcome struct {
		quote  Quote
		stdout string
		err    error
	}
	results := make([]outcome, len(orders))

	// Share the same immutable Bundle across goroutines; each gets its own Engine.
	var wg sync.WaitGroup
	for i, order := range orders {
		wg.Add(1)
		go func(idx int, customer string, subtotal float64) {
			defer wg.Done()

			var out bytes.Buffer
			eng := evie.New(evie.Options{
				Stdlib: []string{"io", "math"}, // deny "fs", "time", etc.
				Stdout: &out,
			})
			defer eng.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			if err := eng.LoadBundle(ctx, bundle); err != nil {
				results[idx].err = err
				return
			}
			val, err := eng.CallContext(ctx, "main.quote", customer, subtotal, 0.08)
			if err != nil {
				results[idx].err = err
				return
			}
			var q Quote
			if err := val.Unmarshal(&q); err != nil {
				results[idx].err = err
				return
			}
			results[idx] = outcome{
				quote:  q,
				stdout: strings.TrimSpace(out.String()),
			}
		}(i, order.customer, order.subtotal)
	}
	wg.Wait()

	for _, r := range results {
		if r.err != nil {
			fmt.Print(evie.Format(r.err, false))
			panic(r.err)
		}
		fmt.Printf("[Go] %s => $%.2f (script printed: %q)\n", r.quote.Customer, r.quote.Total, r.stdout)
	}
}
