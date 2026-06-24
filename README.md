<!-- togo-header -->
<div align="center">
  <img src=".github/assets/togo-mark.svg" alt="togo" height="64" />
  <h1>togo-framework/ocr</h1>
  <p>
    <a href="https://to-go.dev/marketplace"><img src="https://img.shields.io/badge/marketplace-to--go.dev-1FC7DC" alt="marketplace" /></a>
    <a href="https://pkg.go.dev/github.com/togo-framework/ocr"><img src="https://pkg.go.dev/badge/github.com/togo-framework/ocr.svg" alt="pkg.go.dev" /></a>
    <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT" />
  </p>
  <p><strong>Part of the <a href="https://to-go.dev">togo</a> framework.</strong></p>
</div>

## Install

```bash
togo install togo-framework/ocr
```

<!-- /togo-header -->

<p align="center"><img src="https://to-go.dev/togo-mark.svg" height="64" alt="togo"></p>

# togo · ocr

**Image → text for togo apps.** A swappable OCR driver, a Go API, and a REST endpoint. Built to sit under the togo AI part.

```bash
togo install togo-framework/ocr
```

Pick the engine with `OCR_DRIVER`:

| Driver | How | Notes |
|---|---|---|
| `tesseract` *(default)* | the local `tesseract` binary (stdin→stdout) | real OCR, offline; install Tesseract on the host. `Options.lang` (e.g. `eng`, `ara`, `eng+ara`). |
| `ai` | the togo [`ai`](https://github.com/togo-framework/ai) plugin (a multimodal model) | best-effort — passes the image as a data-URL prompt. Needs a vision-capable provider; see the note below. |

## Use it

**Go:**

```go
import "github.com/togo-framework/ocr"

svc, _ := ocr.FromKernel(k)
text, err := svc.Extract(ctx, imageBytes, ocr.Options{Lang: "eng"})
```

**REST** — `POST /api/ocr`:

```bash
# multipart
curl -X POST localhost:8080/api/ocr -F image=@scan.png
# or JSON (base64 or data-URL)
curl -X POST localhost:8080/api/ocr -H 'content-type: application/json' \
  -d '{"image":"<base64>","options":{"lang":"eng"}}'
# → {"text":"…"}
```

## Add an engine

Implement `ocr.Extractor` and `ocr.RegisterDriver("paddle", factory)` in your plugin's `init()`.

> **AI driver note:** the `ai` plugin's `Message.Content` is currently plain text, so the image is embedded as a data-URL in the prompt. Robust vision OCR needs a multimodal provider whose driver forwards image data-URLs — or an enhancement to the `ai` plugin's `Message` to carry structured image parts. For reliable offline OCR, use `tesseract`.

MIT © togo

<!-- togo-sponsors -->
---

<div align="center">
  <h3>Premium sponsors</h3>
  <p>
    <a href="https://id8media.com"><strong>ID8 Media</strong></a> &nbsp;·&nbsp;
    <a href="https://one-studio.co"><strong>One Studio</strong></a>
  </p>
  <p><sub>Support togo — <a href="https://github.com/sponsors/fadymondy">become a sponsor</a>.</sub></p>
</div>
<!-- /togo-sponsors -->
