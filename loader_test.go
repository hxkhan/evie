package evie_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hxkhan/evie"
)

func TestLoaderSingleFile(t *testing.T) {
	dir := t.TempDir()
	mainFile := filepath.Join(dir, "main.ev")
	code := `
	package main
	fn main() {
		return 40 + 2
	}
	`
	if err := os.WriteFile(mainFile, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := evie.RunFile(mainFile)
	if err != nil {
		t.Fatalf("RunFile failed: %v", err)
	}
	if val.Int() != 42 {
		t.Fatalf("expected 42, got %v", val.Int())
	}
}

func TestLoaderMultiFileRelativeImport(t *testing.T) {
	dir := t.TempDir()

	mathFile := filepath.Join(dir, "math.ev")
	mathCode := `
	package math
	fn add(a, b) => a + b
	fn multiply(a, b) => a * b
	`
	if err := os.WriteFile(mathFile, []byte(mathCode), 0644); err != nil {
		t.Fatal(err)
	}

	mainFile := filepath.Join(dir, "main.ev")
	mainCode := `
	package main imports("./math")
	fn main() {
		return math.add(10, 5) + math.multiply(2, 3)
	}
	`
	if err := os.WriteFile(mainFile, []byte(mainCode), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := evie.RunFile(mainFile)
	if err != nil {
		t.Fatalf("RunFile failed: %v", err)
	}
	// 10 + 5 = 15; 2 * 3 = 6; 15 + 6 = 21
	if val.Int() != 21 {
		t.Fatalf("expected 21, got %v", val.Int())
	}
}

func TestLoaderDirectoryPackage(t *testing.T) {
	dir := t.TempDir()
	calcDir := filepath.Join(dir, "calc")
	if err := os.MkdirAll(calcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// File 1 in calc package
	addFile := filepath.Join(calcDir, "add.ev")
	addCode := `
	package calc
	fn add(a, b) => a + b
	`
	if err := os.WriteFile(addFile, []byte(addCode), 0644); err != nil {
		t.Fatal(err)
	}

	// File 2 in calc package
	subFile := filepath.Join(calcDir, "sub.ev")
	subCode := `
	package calc
	fn sub(a, b) => a - b
	`
	if err := os.WriteFile(subFile, []byte(subCode), 0644); err != nil {
		t.Fatal(err)
	}

	mainFile := filepath.Join(dir, "main.ev")
	mainCode := `
	package main imports("./calc")
	fn main() {
		return calc.add(100, 20) + calc.sub(50, 10)
	}
	`
	if err := os.WriteFile(mainFile, []byte(mainCode), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := evie.RunFile(mainFile)
	if err != nil {
		t.Fatalf("RunFile failed: %v", err)
	}
	// 120 + 40 = 160
	if val.Int() != 160 {
		t.Fatalf("expected 160, got %v", val.Int())
	}
}

func TestLoaderNestedRelativeImports(t *testing.T) {
	dir := t.TempDir()

	// Root helper
	constFile := filepath.Join(dir, "constants.ev")
	constCode := `
	package constants
	fn base() => 100
	`
	if err := os.WriteFile(constFile, []byte(constCode), 0644); err != nil {
		t.Fatal(err)
	}

	// Sub directory
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	subFile := filepath.Join(subDir, "worker.ev")
	subCode := `
	package worker imports("../constants")
	fn compute(x) {
		return constants.base() + x
	}
	`
	if err := os.WriteFile(subFile, []byte(subCode), 0644); err != nil {
		t.Fatal(err)
	}

	mainFile := filepath.Join(dir, "main.ev")
	mainCode := `
	package main imports("./sub/worker")
	fn main() {
		return worker.compute(25)
	}
	`
	if err := os.WriteFile(mainFile, []byte(mainCode), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := evie.RunFile(mainFile)
	if err != nil {
		t.Fatalf("RunFile failed: %v", err)
	}
	if val.Int() != 125 {
		t.Fatalf("expected 125, got %v", val.Int())
	}
}

func TestLoaderRunProject(t *testing.T) {
	dir := t.TempDir()

	helperFile := filepath.Join(dir, "helper.ev")
	helperCode := `
	package helper
	fn greeting() => "Hello from project!"
	`
	if err := os.WriteFile(helperFile, []byte(helperCode), 0644); err != nil {
		t.Fatal(err)
	}

	mainFile := filepath.Join(dir, "main.ev")
	mainCode := `
	package main imports("./helper")
	fn main() {
		return helper.greeting()
	}
	`
	if err := os.WriteFile(mainFile, []byte(mainCode), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := evie.RunProject(dir)
	if err != nil {
		t.Fatalf("RunProject failed: %v", err)
	}
	if val.String() != "Hello from project!" {
		t.Fatalf("expected 'Hello from project!', got %v", val.String())
	}
}

func TestLoaderCyclicImportDetection(t *testing.T) {
	dir := t.TempDir()

	aFile := filepath.Join(dir, "a.ev")
	bFile := filepath.Join(dir, "b.ev")

	aCode := `
	package a imports("./b")
	fn fromA() => 1
	`
	bCode := `
	package b imports("./a")
	fn fromB() => 2
	`
	if err := os.WriteFile(aFile, []byte(aCode), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bFile, []byte(bCode), 0644); err != nil {
		t.Fatal(err)
	}

	mainFile := filepath.Join(dir, "main.ev")
	mainCode := `
	package main imports("./a")
	fn main() => a.fromA()
	`
	if err := os.WriteFile(mainFile, []byte(mainCode), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := evie.RunFile(mainFile)
	if err == nil {
		t.Fatalf("expected cyclic import error, got nil")
	}
	if !strings.Contains(err.Error(), "cyclic import detected") {
		t.Fatalf("expected 'cyclic import detected', got %v", err)
	}
}
