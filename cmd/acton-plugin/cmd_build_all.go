package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/actonos/plugin-sdk/sdk"
)

type buildResult struct {
	id     string
	rel    string
	status string
	wasmKB float64
	pkgKB  float64
	dur    time.Duration
}

func runBuildAll(args []string) error {
	fsFlags := flag.NewFlagSet("build-all", flag.ContinueOnError)
	pluginsDir := fsFlags.String("dir", "plugins", "Directory to scan for plugins")
	distDir := fsFlags.String("out", "dist", "Output directory for .wasm and .actonpkg files")
	clean := fsFlags.Bool("clean", false, "Clean dist directory before building")

	if err := fsFlags.Parse(args); err != nil {
		return err
	}

	if *clean {
		_ = os.RemoveAll(*distDir)
	}
	if err := os.MkdirAll(*distDir, 0755); err != nil {
		return fmt.Errorf("creating dist directory '%s': %w", *distDir, err)
	}

	scanDirs := []string{*pluginsDir}

	var manifests []string
	for _, root := range scanDirs {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && d.Name() == "manifest.json" {
				manifests = append(manifests, path)
			}
			return nil
		})
	}

	if len(manifests) == 0 {
		return fmt.Errorf("no plugins found in: %v", scanDirs)
	}

	fmt.Println("=================================================================")
	fmt.Println("🚀 ActonOS Plugin SDK - Batch Build & Package Toolchain")
	fmt.Println("=================================================================")
	fmt.Printf("Found %d plugin(s) to process into '%s'...\n\n", len(manifests), *distDir)

	var results []buildResult
	totalStart := time.Now()

	for _, manifestPath := range manifests {
		pluginDir := filepath.Dir(manifestPath)
		manifestBytes, err := os.ReadFile(manifestPath)
		if err != nil {
			fmt.Printf("❌ Failed reading manifest '%s': %v\n", manifestPath, err)
			continue
		}

		var manifest sdk.PluginManifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			fmt.Printf("❌ Failed parsing manifest '%s': %v\n", manifestPath, err)
			continue
		}

		pluginID := manifest.ID
		if pluginID == "" {
			pluginID = filepath.Base(pluginDir)
		}

		fmt.Printf("▶ Building [%s] (%s v%s)...\n", pluginID, manifest.Name, manifest.Version)
		startTime := time.Now()

		wasmOut := filepath.Join(*distDir, pluginID+".wasm")
		pkgOut := filepath.Join(*distDir, pluginID+".actonpkg")

		// 1. Compile WASM binary
		cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasmOut, "./"+pluginDir)
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("   ❌ Compilation failed: %v (output: %s)\n", err, string(out))
			results = append(results, buildResult{
				id:     pluginID,
				rel:    pluginDir,
				status: "FAILED",
			})
			continue
		}

		// Also update plugin/dist/plugin.wasm for local testing
		localDist := filepath.Join(pluginDir, "dist")
		_ = os.MkdirAll(localDist, 0755)
		localWasm := filepath.Join(localDist, "plugin.wasm")
		wasmBytes, _ := os.ReadFile(wasmOut)
		_ = os.WriteFile(localWasm, wasmBytes, 0644)

		wasmInfo, _ := os.Stat(wasmOut)
		wasmKB := float64(wasmInfo.Size()) / 1024.0

		// 2. Package into .actonpkg
		zipFile, err := os.Create(pkgOut)
		if err != nil {
			fmt.Printf("   ❌ Failed creating package file: %v\n", err)
			continue
		}

		zw := zip.NewWriter(zipFile)
		_ = addFileToZipDirect(zw, "manifest.json", manifestPath)
		_ = addFileToZipDirect(zw, "plugin.wasm", wasmOut)

		sigPath := filepath.Join(pluginDir, "dist", "signature.sig")
		if _, err := os.Stat(sigPath); err == nil {
			_ = addFileToZipDirect(zw, "signature.sig", sigPath)
		}
		readmePath := filepath.Join(pluginDir, "README.md")
		if _, err := os.Stat(readmePath); err == nil {
			_ = addFileToZipDirect(zw, "README.md", readmePath)
		}
		_ = zw.Close()
		_ = zipFile.Close()

		pkgInfo, _ := os.Stat(pkgOut)
		pkgKB := float64(pkgInfo.Size()) / 1024.0
		dur := time.Since(startTime)

		fmt.Printf("   ✅ Compiled & Packaged -> dist/%s.actonpkg (%.1f KB) in %v\n", pluginID, pkgKB, dur.Round(time.Millisecond))

		results = append(results, buildResult{
			id:     pluginID,
			rel:    pluginDir,
			status: "SUCCESS",
			wasmKB: wasmKB,
			pkgKB:  pkgKB,
			dur:    dur,
		})
	}

	fmt.Println("\n=================================================================")
	fmt.Println("📊 Build & Packaging Summary")
	fmt.Println("=================================================================")
	fmt.Printf("%-28s %-10s %-12s %-12s %-10s\n", "PLUGIN ID", "STATUS", "WASM SIZE", "PKG SIZE", "TIME")
	fmt.Println(strings.Repeat("-", 75))

	successCount := 0
	for _, r := range results {
		if r.status == "SUCCESS" {
			successCount++
			fmt.Printf("%-28s %-10s %-12s %-12s %-10s\n",
				r.id, r.status,
				fmt.Sprintf("%.1f KB", r.wasmKB),
				fmt.Sprintf("%.1f KB", r.pkgKB),
				fmt.Sprintf("%v", r.dur.Round(time.Millisecond)))
		} else {
			fmt.Printf("%-28s %-10s %-12s %-12s %-10s\n", r.id, r.status, "-", "-", "-")
		}
	}
	fmt.Println(strings.Repeat("-", 75))
	fmt.Printf("Total: %d | Success: %d | Failed: %d | Total Duration: %v\n",
		len(manifests), successCount, len(manifests)-successCount, time.Since(totalStart).Round(time.Millisecond))
	fmt.Printf("All packages are ready in: %s\n", *distDir)
	fmt.Println("=================================================================")

	return nil
}

func addFileToZipDirect(w *zip.Writer, name, srcPath string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	h, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	h.Name = name
	h.Method = zip.Deflate

	writer, err := w.CreateHeader(h)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, f)
	return err
}
