// Command benchmarks runs the Evie benchmark suite, verifies each program's result
// and optionally compares the timings against a previously saved run.
//
//	go run ./benchmarks                          run everything, compare to baseline.json if present
//	go run ./benchmarks -filter fib -n 10        only benchmarks whose path matches "fib"
//	go run ./benchmarks -save                    also store the run under benchmarks/results/
//	go run ./benchmarks -save-baseline           make this run the new baseline
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"slices"
	"strings"
	"time"

	"github.com/hxkhan/evie"
)

type Result struct {
	Name     string  `json:"name"`
	OK       bool    `json:"ok"`
	Error    string  `json:"error,omitempty"`
	Runs     int     `json:"runs"`
	MedianNs int64   `json:"median_ns"`
	MinNs    int64   `json:"min_ns"`
	MaxNs    int64   `json:"max_ns"`
	StdDev   float64 `json:"stddev_pct"`
	Bytes    uint64  `json:"alloc_bytes"`
	Mallocs  uint64  `json:"mallocs"`
}

type Run struct {
	Commit    string    `json:"commit"`
	Branch    string    `json:"branch"`
	Dirty     bool      `json:"dirty"`
	Date      time.Time `json:"date"`
	GoVersion string    `json:"go_version"`
	OS        string    `json:"os"`
	CPUs      int       `json:"cpus"`
	Results   []Result  `json:"results"`
}

var expectRe = regexp.MustCompile(`^//\s*expect:\s*(.*?)\s*$`)

func main() {
	dir := flag.String("dir", "", "benchmark directory (default: auto-detect benchmarks/)")
	runs := flag.Int("n", 5, "timed runs per benchmark")
	warmup := flag.Int("warmup", 1, "untimed warm-up runs per benchmark")
	filter := flag.String("filter", "", "only run benchmarks whose path matches this regexp")
	baseline := flag.String("baseline", "", "results file to compare against (default: <dir>/results/baseline.json)")
	save := flag.Bool("save", false, "save this run to <dir>/results/<date>_<commit>.json")
	saveBaseline := flag.Bool("save-baseline", false, "save this run as <dir>/results/baseline.json")
	out := flag.String("out", "", "also write this run's JSON to the given path")
	md := flag.String("md", "", "write a markdown report to the given path")
	flag.Parse()

	root, err := findDir(*dir)
	if err != nil {
		fatal(err)
	}

	var re *regexp.Regexp
	if *filter != "" {
		if re, err = regexp.Compile(*filter); err != nil {
			fatal(err)
		}
	}

	files, err := discover(root, re)
	if err != nil {
		fatal(err)
	}
	if len(files) == 0 {
		fatal(fmt.Errorf("no benchmarks found in %s", root))
	}

	basePath := *baseline
	if basePath == "" {
		basePath = filepath.Join(root, "results", "baseline.json")
	}
	base := loadRun(basePath)

	run := Run{
		Commit:    git("rev-parse", "--short", "HEAD"),
		Branch:    git("rev-parse", "--abbrev-ref", "HEAD"),
		Dirty:     git("status", "--porcelain") != "",
		Date:      time.Now().UTC(),
		GoVersion: goruntime.Version(),
		OS:        goruntime.GOOS + "/" + goruntime.GOARCH,
		CPUs:      goruntime.NumCPU(),
	}

	fmt.Printf("evie benchmarks @ %s (%s)%s  runs=%d  cpus=%d\n", run.Commit, run.Branch, dirtyMark(run.Dirty), *runs, run.CPUs)
	if base != nil {
		fmt.Printf("baseline: %s @ %s\n", filepath.Base(basePath), base.Commit)
	}
	fmt.Println()
	fmt.Printf("%-26s %10s %10s %7s %10s %9s\n", "benchmark", "median", "min", "±", "alloc", "Δ base")

	for _, file := range files {
		name := filepath.ToSlash(strings.TrimSuffix(mustRel(root, file), ".ev"))
		res := bench(name, file, *warmup, *runs)
		run.Results = append(run.Results, res)
		printResult(res, find(base, name))
	}

	printSummary(run, base)

	if *save {
		path := filepath.Join(root, "results", fmt.Sprintf("%s_%s.json", run.Date.Format("2006-01-02T15-04-05"), run.Commit))
		writeRun(path, run)
		fmt.Println("saved", path)
	}
	if *saveBaseline {
		path := filepath.Join(root, "results", "baseline.json")
		writeRun(path, run)
		fmt.Println("saved", path)
	}
	if *out != "" {
		writeRun(*out, run)
	}
	if *md != "" {
		if err := os.WriteFile(*md, []byte(markdown(run, base)), 0o644); err != nil {
			fatal(err)
		}
	}

	for _, r := range run.Results {
		if !r.OK {
			os.Exit(1)
		}
	}
}

func bench(name, file string, warmup, runs int) Result {
	res := Result{Name: name, Runs: runs}

	expect, err := readExpect(file)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	once := func() (time.Duration, string, error) {
		eng := evie.New()
		start := time.Now()
		v, err := eng.RunFile(file)
		elapsed := time.Since(start)
		if err != nil {
			return elapsed, "", err
		}
		return elapsed, v.String(), nil
	}

	for range warmup {
		if _, _, err := once(); err != nil {
			res.Error = err.Error()
			return res
		}
	}

	times := make([]time.Duration, 0, runs)
	var before, after goruntime.MemStats
	for i := range runs {
		goruntime.GC()
		goruntime.ReadMemStats(&before)
		elapsed, got, err := once()
		goruntime.ReadMemStats(&after)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		if got != expect {
			res.Error = fmt.Sprintf("result %q, expected %q", got, expect)
			return res
		}
		if i == 0 {
			res.Bytes = after.TotalAlloc - before.TotalAlloc
			res.Mallocs = after.Mallocs - before.Mallocs
		}
		times = append(times, elapsed)
	}

	slices.Sort(times)
	res.OK = true
	res.MinNs = int64(times[0])
	res.MaxNs = int64(times[len(times)-1])
	res.MedianNs = int64(times[len(times)/2])
	if len(times)%2 == 0 {
		res.MedianNs = int64((times[len(times)/2-1] + times[len(times)/2]) / 2)
	}

	var mean, sq float64
	for _, t := range times {
		mean += float64(t)
	}
	mean /= float64(len(times))
	for _, t := range times {
		sq += (float64(t) - mean) * (float64(t) - mean)
	}
	res.StdDev = 100 * math.Sqrt(sq/float64(len(times))) / mean
	return res
}

func readExpect(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m := expectRe.FindStringSubmatch(line); m != nil {
			return m[1], nil
		}
		if line != "" && !strings.HasPrefix(line, "//") {
			break
		}
	}
	return "", fmt.Errorf("missing '// expect: <result>' header")
}

func discover(root string, re *regexp.Regexp) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "results" {
			return filepath.SkipDir
		}
		if d.IsDir() || filepath.Ext(path) != ".ev" {
			return nil
		}
		if re != nil && !re.MatchString(filepath.ToSlash(mustRel(root, path))) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func findDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	for _, candidate := range []string{"benchmarks", "."} {
		if _, err := os.Stat(filepath.Join(candidate, "main.go")); err == nil {
			if _, err := os.Stat(filepath.Join(candidate, "math")); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("cannot find the benchmarks directory; pass -dir")
}

func printResult(r Result, base *Result) {
	if !r.OK {
		fmt.Printf("%-26s FAIL: %s\n", r.Name, r.Error)
		return
	}
	fmt.Printf("%-26s %10s %10s %6.1f%% %10s %9s\n",
		r.Name, dur(r.MedianNs), dur(r.MinNs), r.StdDev, bytes(r.Bytes), delta(r, base))
}

func printSummary(run Run, base *Run) {
	failed := 0
	for _, r := range run.Results {
		if !r.OK {
			failed++
		}
	}
	fmt.Println()
	fmt.Printf("geomean median: %s", dur(int64(geomean(run, nil))))
	if base != nil {
		if ratio := geomeanRatio(run, base); ratio > 0 {
			fmt.Printf("   vs baseline: %+.1f%%", (ratio-1)*100)
		}
	}
	fmt.Println()
	if failed > 0 {
		fmt.Printf("%d benchmark(s) FAILED\n", failed)
	}
}

func markdown(run Run, base *Run) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Evie benchmarks\n\n`%s` on `%s`%s · %s · %s · %d CPUs\n\n", run.Commit, run.Branch, dirtyMark(run.Dirty), run.Date.Format("2006-01-02 15:04 UTC"), run.GoVersion, run.CPUs)
	if base != nil {
		fmt.Fprintf(&sb, "Baseline: `%s` (%s)\n\n", base.Commit, base.Date.Format("2006-01-02"))
	}
	sb.WriteString("| benchmark | median | min | ± | alloc | Δ base |\n|---|---:|---:|---:|---:|---:|\n")
	for _, r := range run.Results {
		if !r.OK {
			fmt.Fprintf(&sb, "| %s | **FAIL** | | | | %s |\n", r.Name, r.Error)
			continue
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %.1f%% | %s | %s |\n", r.Name, dur(r.MedianNs), dur(r.MinNs), r.StdDev, bytes(r.Bytes), delta(r, find(base, r.Name)))
	}
	fmt.Fprintf(&sb, "\nGeomean median: **%s**", dur(int64(geomean(run, nil))))
	if base != nil {
		if ratio := geomeanRatio(run, base); ratio > 0 {
			fmt.Fprintf(&sb, " (%+.1f%% vs baseline)", (ratio-1)*100)
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// geomean of medians, optionally restricted to the names present in `only`
func geomean(run Run, only *Run) float64 {
	var sum float64
	var n int
	for _, r := range run.Results {
		if !r.OK || (only != nil && find(only, r.Name) == nil) {
			continue
		}
		sum += math.Log(float64(r.MedianNs))
		n++
	}
	if n == 0 {
		return 0
	}
	return math.Exp(sum / float64(n))
}

// geomeanRatio compares only the benchmarks that succeeded in both runs
func geomeanRatio(run Run, base *Run) float64 {
	var sum float64
	var n int
	for _, r := range run.Results {
		b := find(base, r.Name)
		if !r.OK || b == nil {
			continue
		}
		sum += math.Log(float64(r.MedianNs) / float64(b.MedianNs))
		n++
	}
	if n == 0 {
		return 0
	}
	return math.Exp(sum / float64(n))
}

func find(run *Run, name string) *Result {
	if run == nil {
		return nil
	}
	for i := range run.Results {
		if run.Results[i].Name == name && run.Results[i].OK {
			return &run.Results[i]
		}
	}
	return nil
}

func delta(r Result, base *Result) string {
	if base == nil || base.MedianNs == 0 {
		return "-"
	}
	return fmt.Sprintf("%+.1f%%", (float64(r.MedianNs)/float64(base.MedianNs)-1)*100)
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

func bytes(b uint64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(b)/(1<<20))
	default:
		return fmt.Sprintf("%.0fKB", float64(b)/(1<<10))
	}
}

func dirtyMark(dirty bool) string {
	if dirty {
		return " +dirty"
	}
	return ""
}

func git(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func loadRun(path string) *Run {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		fmt.Fprintf(os.Stderr, "warning: ignoring baseline %s: %v\n", path, err)
		return nil
	}
	return &run
}

func writeRun(path string, run Run) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatal(err)
	}
	data, _ := json.MarshalIndent(run, "", "  ")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		fatal(err)
	}
}

func mustRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(2)
}
