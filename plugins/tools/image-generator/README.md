# AI Image Generator Plugin for ActonOS

An AI Image Generation plugin for ActonOS ReAct Agent Swarms running inside the Wazero WebAssembly sandbox.

## Features
- **Multi-Provider Support**: Switch seamlessly between **OpenAI (DALL-E 3 / Image Edits)**, **Grok (xAI)**, and **NanoBanana**.
- **Binary Reference Images**: Support binary images passed as Base64 strings or Data URIs with automatic `multipart/form-data` encoding.
- **Asynchronous Non-Blocking Execution**: Returns a Job ID immediately to avoid blocking agent deliberation loops.
- **Hardware Vault Integration**: API Keys are hardware-encrypted (`vault.db`) with Argon2id + AES-256-GCM.
- **EventBus Session Notifications**: Emits `session.message.outbound` events to push completed images directly to the user's active chat session.

## Tools Exposed
1. `generate_image`: Trigger background image generation with text prompts and optional binary reference images.
2. `check_image_status`: Query the execution status and image URL of any job.

## Building & Testing

```bash
# Validate manifest
go run ./cmd/acton-plugin validate -manifest ./plugins/tools/image-generator/manifest.json

# Run unit & sandbox tests
go test -v ./plugins/tools/image-generator/...

# Build WebAssembly binary
go run ./cmd/acton-plugin build -src ./plugins/tools/image-generator -out dist/image-generator.wasm

# Package distribution bundle
go run ./cmd/acton-plugin pack -manifest ./plugins/tools/image-generator/manifest.json -wasm dist/image-generator.wasm -out dist/image-generator.actonpkg
```
