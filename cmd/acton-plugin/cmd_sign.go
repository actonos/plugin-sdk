package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runSign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	genKey := fs.Bool("gen-key", false, "Generate a new Ed25519 key pair")
	keyPath := fs.String("key", "plugin_ed25519.key", "Path to private key file")
	manifestPath := fs.String("manifest", "manifest.json", "Path to manifest.json")
	wasmPath := fs.String("wasm", "dist/plugin.wasm", "Path to compiled .wasm file")
	outputPath := fs.String("output", "dist/signature.sig", "Path to output signature file")
	fs.StringVar(outputPath, "out", "dist/signature.sig", "Path to output signature file (alias for -output)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *genKey {
		keyDir := filepath.Dir(*keyPath)
		if keyDir != "." && keyDir != "" {
			_ = os.MkdirAll(keyDir, 0755)
		}

		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("generating ed25519 key: %w", err)
		}

		if err := os.WriteFile(*keyPath, []byte(hex.EncodeToString(priv)), 0600); err != nil {
			return fmt.Errorf("saving private key: %w", err)
		}

		pubPath := *keyPath + ".pub"
		if err := os.WriteFile(pubPath, []byte(hex.EncodeToString(pub)), 0644); err != nil {
			return fmt.Errorf("saving public key: %w", err)
		}

		fmt.Printf("🔑 Generated new Ed25519 keypair:\n")
		fmt.Printf("   Private Key: %s (KEEP SECRET!)\n", *keyPath)
		fmt.Printf("   Public Key:  %s (Share with ActonOS)\n\n", pubPath)
		return nil
	}

	// Read key
	keyHex, err := os.ReadFile(*keyPath)
	if err != nil {
		return fmt.Errorf("reading private key '%s': %w (use --gen-key to create one)", *keyPath, err)
	}

	privKeyBytes, err := hex.DecodeString(strings.TrimSpace(string(keyHex)))
	if err != nil || len(privKeyBytes) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid private key in '%s', must be 64-byte hex string", *keyPath)
	}
	privKey := ed25519.PrivateKey(privKeyBytes)

	outDir := filepath.Dir(*outputPath)
	if outDir != "." && outDir != "" {
		_ = os.MkdirAll(outDir, 0755)
	}

	// Compute digest = SHA-256(manifest.json || plugin.wasm)
	manifestBytes, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest '%s': %w", *manifestPath, err)
	}

	wasmBytes, err := os.ReadFile(*wasmPath)
	if err != nil {
		return fmt.Errorf("reading wasm binary '%s': %w", *wasmPath, err)
	}

	h := sha256.New()
	h.Write(manifestBytes)
	h.Write(wasmBytes)
	digest := h.Sum(nil)

	signature := ed25519.Sign(privKey, digest)
	sigHex := hex.EncodeToString(signature)

	if err := os.WriteFile(*outputPath, []byte(sigHex), 0644); err != nil {
		return fmt.Errorf("writing signature file '%s': %w", *outputPath, err)
	}

	fmt.Printf("✍️  Successfully signed plugin bundle!\n")
	fmt.Printf("   Payload Digest (SHA-256): %s\n", hex.EncodeToString(digest))
	fmt.Printf("   Signature File:           %s\n", *outputPath)
	return nil
}
