package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/actonos/acton-plugin-sdk/sdk"
)

func runPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "manifest.json", "Path to manifest.json")
	wasmPath := fs.String("wasm", "dist/plugin.wasm", "Path to compiled .wasm file")
	sigPath := fs.String("sig", "dist/signature.sig", "Path to signature file (optional)")
	outputPath := fs.String("output", "", "Output .actonpkg bundle path (defaults to <id>-<version>.actonpkg)")
	fs.StringVar(outputPath, "out", "", "Output .actonpkg bundle path (alias for -output)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	manifestBytes, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest '%s': %w", *manifestPath, err)
	}

	var manifest sdk.PluginManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	outPkg := *outputPath
	if outPkg == "" {
		outPkg = fmt.Sprintf("dist/%s-%s.actonpkg", manifest.ID, manifest.Version)
	}

	outDir := filepath.Dir(outPkg)
	if outDir != "." && outDir != "" {
		_ = os.MkdirAll(outDir, 0755)
	}

	// Create zip archive
	zipFile, err := os.Create(outPkg)
	if err != nil {
		return fmt.Errorf("creating package file '%s': %w", outPkg, err)
	}
	defer zipFile.Close()

	w := zip.NewWriter(zipFile)

	// 1. Add manifest.json
	if err := addFileToZip(w, "manifest.json", *manifestPath); err != nil {
		return err
	}

	// 2. Add plugin.wasm
	if err := addFileToZip(w, "plugin.wasm", *wasmPath); err != nil {
		return err
	}

	// 3. Add signature if exists
	if _, err := os.Stat(*sigPath); err == nil {
		if err := addFileToZip(w, "signature.sig", *sigPath); err != nil {
			return err
		}
	}

	// 4. Add README.md if exists
	if _, err := os.Stat("README.md"); err == nil {
		_ = addFileToZip(w, "README.md", "README.md")
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("finalizing zip archive: %w", err)
	}

	info, err := os.Stat(outPkg)
	if err != nil {
		return err
	}

	sizeKB := float64(info.Size()) / 1024.0
	fmt.Printf("📦 Successfully packaged plugin into '%s' (%.2f KB)!\n", outPkg, sizeKB)
	fmt.Printf("Ready to upload directly into ActonOS Web UI (Settings > Plugins > Upload)!\n")
	return nil
}

func addFileToZip(w *zip.Writer, archiveName string, sourcePath string) error {
	fileToZip, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("opening file '%s': %w", sourcePath, err)
	}
	defer fileToZip.Close()

	info, err := fileToZip.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = archiveName
	header.Method = zip.Deflate

	writer, err := w.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, fileToZip)
	return err
}
