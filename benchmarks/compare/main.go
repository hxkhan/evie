// Command compare benchmarks Evie against other scripting languages across a
// shared suite of workloads, verifying that every implementation prints the
// exact expected checksum.
//
//	go run ./benchmarks/compare                         run all installed interpreters
//	go run ./benchmarks/compare -filter "fib|sieve"     only matching benchmarks
//	go run ./benchmarks/compare -langs evie,lua54,qjs   specific runtimes
//	go run ./benchmarks/compare -jit                    also include Node.js V8 JIT
//	go run ./benchmarks/compare -md results.md          write a markdown table
package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

type Runtime struct {
	Name     string   // display name in tables
	Ext      string   // file extension in suites/<bench>/<bench>.<ext>
	Bins     []string // executable candidates on PATH (empty for built evie)
	Build    string   // a package of the runners module to build instead of looking on PATH
	Args     []string // extra flags before the script path
	OptIn    bool     // skipped unless -jit or explicitly requested via -langs
	Resolved string   // resolved binary path
}

type Cell struct {
	Bench    string  `json:"bench"`
	Lang     string  `json:"lang"`
	OK       bool    `json:"ok"`
	Skipped  bool    `json:"skipped,omitempty"`
	Error    string  `json:"error,omitempty"`
	Runs     int     `json:"runs"`
	MedianNs int64   `json:"median_ns"`
	MinNs    int64   `json:"min_ns"`
	StdDev   float64 `json:"stddev_pct"`
}

type Report struct {
	Commit     string          `json:"commit"`
	Date       time.Time       `json:"date"`
	OS         string          `json:"os"`
	CPUs       int             `json:"cpus"`
	Runs       int             `json:"runs"`
	Languages  []string        `json:"languages"`
	Benchmarks []string        `json:"benchmarks"`
	Cells      map[string]Cell `json:"cells"` // key: bench + "/" + lang
}

var defaultRuntimes = []Runtime{
	{Name: "evie", Ext: "ev"},
	{Name: "lua54", Ext: "lua", Bins: []string{"lua54", "lua"}},
	{Name: "qjs", Ext: "js", Bins: []string{"qjs"}},
	{Name: "v8-jitless", Ext: "js", Bins: []string{"node"}, Args: []string{"--jitless"}},
	{Name: "python", Ext: "py", Bins: []string{"python", "python3"}},
	{Name: "wren", Ext: "wren", Bins: []string{"wren"}},
	{Name: "tengo", Ext: "tengo", Bins: []string{"tengo"}},
	// the other languages a Go program can embed, built from the runners module
	{Name: "goja", Ext: "js", Build: "./goja"},
	{Name: "gopher-lua", Ext: "lua", Build: "github.com/yuin/gopher-lua/cmd/glua"},
	{Name: "starlark", Ext: "star", Build: "./starlark"},
	{Name: "yaegi", Ext: "yaegi", Build: "github.com/traefik/yaegi/cmd/yaegi"},
	{Name: "node-jit", Ext: "js", Bins: []string{"node"}, OptIn: true},
}

func main() {
	dir := flag.String("dir", "", "compare directory (default: auto-detect benchmarks/compare)")
	runs := flag.Int("n", 5, "timed runs per benchmark")
	warmup := flag.Int("warmup", 1, "untimed warm-up runs per benchmark")
	filter := flag.String("filter", "", "only run benchmarks whose name matches this regexp")
	langs := flag.String("langs", "", "comma-separated list of runtimes to run (e.g. evie,lua54,qjs)")
	includeJIT := flag.Bool("jit", false, "also include node-jit (V8 TurboFan JIT)")
	timeout := flag.Duration("timeout", 30*time.Second, "per-run timeout")
	md := flag.String("md", "", "write a markdown comparison table to this path")
	outJSON := flag.String("json", "", "write raw JSON results to this path")
	flag.Parse()
	if *runs < 1 {
		fatal(fmt.Errorf("-n must be at least 1"))
	}
	stopOnInterrupt()

	root, err := findCompareDir(*dir)
	if err != nil {
		fatal(err)
	}
	repoRoot := findRepoRoot(root)

	var re *regexp.Regexp
	if *filter != "" {
		if re, err = regexp.Compile(*filter); err != nil {
			fatal(err)
		}
	}

	benches, err := discoverSuites(filepath.Join(root, "suites"), re)
	if err != nil {
		fatal(err)
	}
	if len(benches) == 0 {
		fatal(fmt.Errorf("no benchmark suites found in %s", filepath.Join(root, "suites")))
	}

	// Build a fresh evie binary outside %TEMP% so Windows Smart App Control won't block it.
	evieBin, cleanup, err := buildEvie(repoRoot, root)
	if err != nil {
		fatal(err)
	}
	atExit(cleanup)
	defer cleanup()

	active := selectRuntimes(evieBin, root, *langs, *includeJIT)
	if len(active) == 0 {
		fatal(fmt.Errorf("no requested runtimes are available on PATH"))
	}

	langNames := make([]string, len(active))
	for i, rt := range active {
		langNames[i] = rt.Name
	}

	report := Report{
		Commit:     git(repoRoot, "rev-parse", "--short", "HEAD"),
		Date:       time.Now().UTC(),
		OS:         goruntime.GOOS + "/" + goruntime.GOARCH,
		CPUs:       goruntime.NumCPU(),
		Runs:       *runs,
		Languages:  langNames,
		Benchmarks: benches,
		Cells:      make(map[string]Cell, len(benches)*len(active)),
	}

	fmt.Printf("evie cross-language benchmarks @ %s  runs=%d  warmup=%d  cpus=%d\n", report.Commit, *runs, *warmup, report.CPUs)
	fmt.Printf("runtimes: %s\n\n", strings.Join(langNames, ", "))

	failed := 0
	for _, bench := range benches {
		suiteDir := filepath.Join(root, "suites", bench)
		expectBytes, err := os.ReadFile(filepath.Join(suiteDir, "expect.txt"))
		if err != nil {
			fatal(fmt.Errorf("%s: missing expect.txt: %w", bench, err))
		}
		expect := strings.TrimSpace(string(expectBytes))
		fmt.Printf("=== %s (expect: %s) ===\n", bench, expect)

		for _, rt := range active {
			script := filepath.Join(suiteDir, bench+"."+rt.Ext)
			var cell Cell
			if _, statErr := os.Stat(script); statErr != nil {
				cell = Cell{Bench: bench, Lang: rt.Name, Skipped: true}
				fmt.Printf("  %-12s skipped (no %s)\n", rt.Name, filepath.Base(script))
			} else {
				cell = runBench(bench, rt, script, expect, *warmup, *runs, *timeout)
				if cell.OK {
					fmt.Printf("  %-12s %8s  (min %8s, ±%4.1f%%)\n", rt.Name, dur(cell.MedianNs), dur(cell.MinNs), cell.StdDev)
				} else {
					failed++
					fmt.Printf("  %-12s FAIL: %s\n", rt.Name, cell.Error)
				}
			}
			report.Cells[bench+"/"+rt.Name] = cell
		}
		fmt.Println()
	}

	printTable(report)

	if *md != "" {
		if err := os.WriteFile(*md, []byte(renderMarkdown(report)), 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("\nwrote markdown report to %s\n", *md)
	}
	if *outJSON != "" {
		data, _ := json.Marshal(report, jsontext.WithIndent("  "))
		if err := os.WriteFile(*outJSON, append(data, '\n'), 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote JSON results to %s\n", *outJSON)
	}

	if failed > 0 {
		exit(1)
	}
}

func selectRuntimes(evieBin, compareRoot, filterCSV string, includeJIT bool) []Runtime {
	var requested map[string]bool
	if filterCSV != "" {
		requested = map[string]bool{}
		for _, part := range strings.Split(filterCSV, ",") {
			name := strings.TrimSpace(part)
			if name != "" {
				requested[name] = true
			}
		}
	}

	var out []Runtime
	for _, rt := range defaultRuntimes {
		if requested != nil {
			if !requested[rt.Name] {
				continue
			}
		} else if rt.OptIn && !includeJIT {
			continue
		}

		if rt.Name == "evie" {
			rt.Resolved = evieBin
			out = append(out, rt)
			continue
		}

		if rt.Build != "" {
			bin, err := buildRunner(compareRoot, filepath.Dir(evieBin), rt)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not build %s, skipping: %v\n", rt.Name, err)
				continue
			}
			rt.Resolved = bin
			out = append(out, rt)
			continue
		}

		found := ""
		for _, bin := range rt.Bins {
			if p, err := exec.LookPath(bin); err == nil {
				found = p
				break
			}
		}
		if found == "" {
			if requested != nil {
				fmt.Fprintf(os.Stderr, "warning: requested runtime %q not found on PATH, skipping\n", rt.Name)
			}
			continue
		}
		rt.Resolved = found
		out = append(out, rt)
	}
	return out
}

func runBench(bench string, rt Runtime, script, expect string, warmup, runs int, timeout time.Duration) Cell {
	cell := Cell{Bench: bench, Lang: rt.Name, Runs: runs}
	args := append(slices.Clone(rt.Args), script)

	once := func() (time.Duration, string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, rt.Resolved, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		start := time.Now()
		err := cmd.Run()
		elapsed := time.Since(start)
		if ctx.Err() == context.DeadlineExceeded {
			return elapsed, "", fmt.Errorf("timed out after %s", timeout)
		}
		if err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return elapsed, "", fmt.Errorf("%s", firstLine(msg))
		}
		return elapsed, strings.TrimSpace(stdout.String()), nil
	}

	for range warmup {
		_, got, err := once()
		if err != nil {
			cell.Error = err.Error()
			return cell
		}
		if got != expect {
			cell.Error = fmt.Sprintf("got %q, want %q", got, expect)
			return cell
		}
	}

	times := make([]time.Duration, 0, runs)
	for range runs {
		elapsed, got, err := once()
		if err != nil {
			cell.Error = err.Error()
			return cell
		}
		if got != expect {
			cell.Error = fmt.Sprintf("got %q, want %q", got, expect)
			return cell
		}
		times = append(times, elapsed)
	}

	slices.Sort(times)
	cell.OK = true
	cell.MinNs = int64(times[0])
	cell.MedianNs = int64(times[len(times)/2])
	if len(times)%2 == 0 {
		cell.MedianNs = int64((times[len(times)/2-1] + times[len(times)/2]) / 2)
	}

	var mean, sq float64
	for _, t := range times {
		mean += float64(t)
	}
	mean /= float64(len(times))
	for _, t := range times {
		sq += (float64(t) - mean) * (float64(t) - mean)
	}
	if mean > 0 {
		cell.StdDev = 100 * math.Sqrt(sq/float64(len(times))) / mean
	}
	return cell
}

func printTable(r Report) {
	fmt.Println("=== Summary (median wall time, process start included) ===")
	fmt.Printf("%-14s", "benchmark")
	for _, lang := range r.Languages {
		fmt.Printf(" %11s", lang)
	}
	fmt.Println()
	fmt.Println(strings.Repeat("-", 14+12*len(r.Languages)))

	for _, b := range r.Benchmarks {
		fmt.Printf("%-14s", b)
		for _, lang := range r.Languages {
			c := r.Cells[b+"/"+lang]
			switch {
			case c.Skipped:
				fmt.Printf(" %11s", "-")
			case !c.OK:
				fmt.Printf(" %11s", "FAIL")
			default:
				fmt.Printf(" %11s", dur(c.MedianNs))
			}
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("-", 14+12*len(r.Languages)))
	fmt.Printf("%-14s", "geomean")
	geomeans := make([]float64, len(r.Languages))
	for i, lang := range r.Languages {
		gm := langGeomean(r, lang)
		geomeans[i] = gm
		if gm > 0 {
			fmt.Printf(" %11s", dur(int64(gm)))
		} else {
			fmt.Printf(" %11s", "-")
		}
	}
	fmt.Println()

	if evieIdx := slices.Index(r.Languages, "evie"); evieIdx >= 0 && geomeans[evieIdx] > 0 {
		fmt.Printf("%-14s", "vs evie")
		for _, lang := range r.Languages {
			if lang == "evie" {
				fmt.Printf(" %11s", "1.00x")
				continue
			}
			ratio := pairedRatio(r, lang, "evie")
			if ratio > 0 {
				fmt.Printf(" %10.2fx", ratio)
			} else {
				fmt.Printf(" %11s", "-")
			}
		}
		fmt.Println()
		fmt.Println("(>1.00x means Evie is faster; <1.00x means the other runtime is faster)")
	}
}

func renderMarkdown(r Report) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Evie Cross-Language Benchmarks\n\n")
	fmt.Fprintf(&sb, "`%s` · %s · %s · %d CPUs · %d runs (median wall time, process start included)\n\n",
		r.Commit, r.Date.Format("2006-01-02 15:04 UTC"), r.OS, r.CPUs, r.Runs)

	sb.WriteString("| benchmark |")
	for _, lang := range r.Languages {
		fmt.Fprintf(&sb, " %s |", lang)
	}
	sb.WriteString("\n|---|")
	for range r.Languages {
		sb.WriteString("---:|")
	}
	sb.WriteString("\n")

	for _, b := range r.Benchmarks {
		fmt.Fprintf(&sb, "| `%s` |", b)
		for _, lang := range r.Languages {
			c := r.Cells[b+"/"+lang]
			switch {
			case c.Skipped:
				sb.WriteString(" - |")
			case !c.OK:
				sb.WriteString(" **FAIL** |")
			default:
				fmt.Fprintf(&sb, " %s |", dur(c.MedianNs))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("| **geomean** |")
	for _, lang := range r.Languages {
		gm := langGeomean(r, lang)
		if gm > 0 {
			fmt.Fprintf(&sb, " **%s** |", dur(int64(gm)))
		} else {
			sb.WriteString(" - |")
		}
	}
	sb.WriteString("\n")

	if slices.Contains(r.Languages, "evie") {
		sb.WriteString("| **vs evie** |")
		for _, lang := range r.Languages {
			if lang == "evie" {
				sb.WriteString(" **1.00x** |")
				continue
			}
			if ratio := pairedRatio(r, lang, "evie"); ratio > 0 {
				fmt.Fprintf(&sb, " %.2fx |", ratio)
			} else {
				sb.WriteString(" - |")
			}
		}
		sb.WriteString("\n\n")
		sb.WriteString("> Values `>1.00x` mean Evie is faster by that factor (paired geometric mean across shared passing benchmarks).\n")
	}
	return sb.String()
}

func langGeomean(r Report, lang string) float64 {
	var sum float64
	var n int
	for _, b := range r.Benchmarks {
		c := r.Cells[b+"/"+lang]
		if !c.OK || c.MedianNs <= 0 {
			continue
		}
		sum += math.Log(float64(c.MedianNs))
		n++
	}
	if n == 0 {
		return 0
	}
	return math.Exp(sum / float64(n))
}

func pairedRatio(r Report, lang, base string) float64 {
	var sum float64
	var n int
	for _, b := range r.Benchmarks {
		c := r.Cells[b+"/"+lang]
		bc := r.Cells[b+"/"+base]
		if !c.OK || !bc.OK || c.MedianNs <= 0 || bc.MedianNs <= 0 {
			continue
		}
		sum += math.Log(float64(c.MedianNs) / float64(bc.MedianNs))
		n++
	}
	if n == 0 {
		return 0
	}
	return math.Exp(sum / float64(n))
}

func buildEvie(repoRoot, compareRoot string) (string, func(), error) {
	ext := ""
	if goruntime.GOOS == "windows" {
		ext = ".exe"
	}
	tmpDir, err := os.MkdirTemp(compareRoot, ".evie-bin-*")
	if err != nil {
		return "", nil, err
	}
	binPath := filepath.Join(tmpDir, "evie"+ext)
	cmd := exec.Command("go", "build", "-o", binPath, "./evie")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("failed to build evie: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	// Pre-launch once so first-run OS binary scans (e.g. Windows Defender) don't skew -warmup 0 runs.
	_ = exec.Command(binPath, "--version").Run()
	return binPath, func() { os.RemoveAll(tmpDir) }, nil
}

// buildRunner builds a runtime from the runners module, which keeps the
// embeddable Go interpreters and their versions out of Evie's own go.mod,
// into dir next to the evie binary
func buildRunner(compareRoot, dir string, rt Runtime) (string, error) {
	bin := filepath.Join(dir, rt.Name)
	if goruntime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, rt.Build)
	cmd.Dir = filepath.Join(compareRoot, "runners")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%v\n%s", err, strings.TrimSpace(string(out)))
	}
	_ = exec.Command(bin, "-h").Run() // a first launch is scanned by some antivirus; keep it out of the timings
	return bin, nil
}

func discoverSuites(suitesDir string, re *regexp.Regexp) ([]string, error) {
	entries, err := os.ReadDir(suitesDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if re != nil && !re.MatchString(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names, nil
}

func findCompareDir(dir string) (string, error) {
	if dir != "" {
		return filepath.Abs(dir)
	}
	for _, c := range []string{"benchmarks/compare", "compare", "."} {
		if _, err := os.Stat(filepath.Join(c, "suites")); err == nil {
			return filepath.Abs(c)
		}
	}
	return "", fmt.Errorf("cannot find benchmarks/compare directory; pass -dir")
}

func findRepoRoot(compareRoot string) string {
	dir := compareRoot
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return compareRoot
		}
		dir = parent
	}
}

func dur(ns int64) string {
	d := time.Duration(ns)
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.0fµs", float64(d)/float64(time.Microsecond))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func git(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	exit(2)
}

// cleanups run before the runner exits early; os.Exit skips deferred calls,
// which used to leave the .evie-bin-* build folder behind
var (
	cleanupsMu sync.Mutex
	cleanups   []func()
)

func atExit(fn func()) {
	cleanupsMu.Lock()
	defer cleanupsMu.Unlock()
	cleanups = append(cleanups, fn)
}

// exit runs the cleanups and exits with code
func exit(code int) {
	cleanupsMu.Lock()
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
	cleanups = nil
	cleanupsMu.Unlock()
	os.Exit(code)
}

// stopOnInterrupt cleans up and exits on Ctrl+C
func stopOnInterrupt() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	go func() {
		<-signals
		exit(130)
	}()
}
