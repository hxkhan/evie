# Cross-language benchmark runner wrapper; -Affinity pins it to a set of cores, such as 0x0FFF for the P-cores of an i5-13400F.
# Usage:
#   .\benchmarks\compare\bench.ps1
#   .\benchmarks\compare\bench.ps1 -Filter "fib|sieve" -Runs 5
#   .\benchmarks\compare\bench.ps1 -Langs "evie,lua54,qjs" -Jit
param (
    [string]$Filter = "",
    [string]$Langs = "",
    [int]$Runs = 5,
    [int]$Warmup = 1,
    [switch]$Jit,
    [string]$Markdown = "",
    [string]$Json = "",
    [int64]$Affinity = 0
)

$ErrorActionPreference = "Stop"

$argsList = @("-dir", $PSScriptRoot, "-n", "$Runs", "-warmup", "$Warmup")
if ($Filter) { $argsList += @("-filter", $Filter) }
if ($Langs)  { $argsList += @("-langs", $Langs) }
if ($Jit)    { $argsList += "-jit" }
if ($Markdown) { $argsList += @("-md", $Markdown) }
if ($Json)     { $argsList += @("-json", $Json) }

$tmpExe = Join-Path $PSScriptRoot ".compare-runner.exe"
try {
    go build -o $tmpExe $PSScriptRoot
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $p = Start-Process -FilePath $tmpExe -ArgumentList $argsList -PassThru -NoNewWindow
    if ($Affinity -gt 0) {
        $p.ProcessorAffinity = [IntPtr]$Affinity
    }
    $p.WaitForExit()
    exit $p.ExitCode
} finally {
    if (Test-Path $tmpExe) {
        Remove-Item $tmpExe -Force -ErrorAction SilentlyContinue
    }
}
