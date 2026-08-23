package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	output := fs.String("output", "dist/plugin.wasm", "Output WASM file path")
	fs.StringVar(output, "out", "dist/plugin.wasm", "Output WASM file path (alias for -output)")
	sourceDir := fs.String("source", ".", "Source directory containing main.go")
	fs.StringVar(sourceDir, "src", ".", "Source directory containing main.go (alias for -source)")
	useTinyGo := fs.Bool("tinygo", false, "Use TinyGo compiler instead of standard Go compiler")

	if err := fs.Parse(args); err != nil {
		return err
	}

	outDir := filepath.Dir(*output)
	if outDir != "." && outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("creating output directory '%s': %w", outDir, err)
		}
	}

	fmt.Printf("🔨 Building WebAssembly plugin from '%s'...\n", *sourceDir)
	startTime := time.Now()

	srcPkg := *sourceDir
	if !strings.HasPrefix(srcPkg, ".") && !filepath.IsAbs(srcPkg) {
		srcPkg = "./" + srcPkg
	}

	var cmd *exec.Cmd
	if *useTinyGo {
		fmt.Println("Using TinyGo compiler (-target=wasi)...")
		cmd = exec.Command("tinygo", "build", "-target=wasi", "-opt=2", "-o", *output, srcPkg)
	} else {
		fmt.Println("Using Standard Go compiler (GOOS=wasip1 GOARCH=wasm -buildmode=c-shared)...")
		cmd = exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", *output, srcPkg)
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("compilation failed: %w", err)
	}

	duration := time.Since(startTime)
	info, err := os.Stat(*output)
	if err != nil {
		return fmt.Errorf("checking output binary: %w", err)
	}

	sizeKB := float64(info.Size()) / 1024.0
	fmt.Printf("\n✅ Successfully compiled '%s' (%.2f KB) in %v!\n", *output, sizeKB, duration.Round(time.Millisecond))
	fmt.Printf("Tip: Run 'acton-plugin test --wasm=%s' to verify plugin execution.\n", *output)

	return nil
}
