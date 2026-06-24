// Package ocr is togo's OCR (image→text) plugin. It exposes a swappable
// Extractor driver, a Go API ocr.Extract(...), and a REST endpoint POST /api/ocr.
// Drivers register via ocr.RegisterDriver; pick one with OCR_DRIVER.
//
//   - "tesseract" (default): real OCR via the local `tesseract` binary.
//   - "ai": uses the togo `ai` plugin (a multimodal model) — best-effort; see README.
//
// Install: `togo install togo-framework/ocr` (blank-import registers it).
package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/togo-framework/ai"
	"github.com/togo-framework/togo"
)

// Options tune an extraction.
type Options struct {
	// Lang is a tesseract language code (e.g. "eng", "ara", "eng+ara"). Default "eng".
	Lang string `json:"lang,omitempty"`
	// Mime hints the image type for the ai driver (e.g. "image/png"). Default png.
	Mime string `json:"mime,omitempty"`
}

// Extractor turns image bytes into text.
type Extractor interface {
	Extract(ctx context.Context, image []byte, opts Options) (string, error)
}

// DriverFactory builds an Extractor from the kernel (env-configured).
type DriverFactory func(k *togo.Kernel) (Extractor, error)

var (
	regMu   sync.RWMutex
	drivers = map[string]DriverFactory{}
)

// RegisterDriver registers an OCR engine by name (call from a plugin's init()).
func RegisterDriver(name string, f DriverFactory) {
	regMu.Lock()
	drivers[name] = f
	regMu.Unlock()
}

func init() {
	RegisterDriver("tesseract", func(k *togo.Kernel) (Extractor, error) { return &tesseract{}, nil })
	RegisterDriver("ai", func(k *togo.Kernel) (Extractor, error) { return &aiDriver{k: k}, nil })

	togo.RegisterProviderFunc("ocr", togo.PriorityService, func(k *togo.Kernel) error {
		name := os.Getenv("OCR_DRIVER")
		if name == "" {
			name = "tesseract"
		}
		regMu.RLock()
		f, ok := drivers[name]
		regMu.RUnlock()
		if !ok {
			return fmt.Errorf("ocr: unknown driver %q (set OCR_DRIVER to tesseract or ai)", name)
		}
		e, err := f(k)
		if err != nil {
			return err
		}
		svc := &Service{extractor: e, driver: name}
		k.Set("ocr", svc)
		if k.Router != nil {
			k.Router.Post("/api/ocr", svc.handle)
		}
		return nil
	})
}

// Service is the ocr runtime stored on the kernel (k.Get("ocr")).
type Service struct {
	extractor Extractor
	driver    string
}

// Extract runs OCR on the image bytes.
func (s *Service) Extract(ctx context.Context, image []byte, opts Options) (string, error) {
	return s.extractor.Extract(ctx, image, opts)
}

// Driver returns the active engine name.
func (s *Service) Driver() string { return s.driver }

// FromKernel fetches the ocr service from the kernel container.
func FromKernel(k *togo.Kernel) (*Service, bool) {
	v, ok := k.Get("ocr")
	if !ok {
		return nil, false
	}
	s, ok := v.(*Service)
	return s, ok
}

// handle serves POST /api/ocr: a multipart "image" file OR JSON {"image": base64}.
func (s *Service) handle(w http.ResponseWriter, r *http.Request) {
	var (
		img  []byte
		opts Options
		err  error
	)
	if ct := r.Header.Get("Content-Type"); len(ct) >= 19 && ct[:19] == "multipart/form-data" {
		file, _, ferr := r.FormFile("image")
		if ferr != nil {
			http.Error(w, `{"error":"missing image file"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		img, err = io.ReadAll(file)
		opts.Lang = r.FormValue("lang")
	} else {
		var body struct {
			Image   string  `json:"image"`
			Options Options `json:"options"`
		}
		if derr := json.NewDecoder(r.Body).Decode(&body); derr != nil {
			http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
			return
		}
		opts = body.Options
		img, err = base64.StdEncoding.DecodeString(stripDataURL(body.Image))
	}
	if err != nil || len(img) == 0 {
		http.Error(w, `{"error":"could not read image"}`, http.StatusBadRequest)
		return
	}
	text, err := s.Extract(r.Context(), img, opts)
	if err != nil {
		http.Error(w, `{"error":"extract failed: `+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"text": text})
}

func stripDataURL(s string) string {
	if i := bytes.IndexByte([]byte(s), ','); i >= 0 && len(s) > 5 && s[:5] == "data:" {
		return s[i+1:]
	}
	return s
}

// ── tesseract driver: real OCR via the local `tesseract` binary ─────────────────

type tesseract struct{}

func (t *tesseract) Extract(ctx context.Context, image []byte, opts Options) (string, error) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return "", fmt.Errorf("tesseract not found on PATH — install it or set OCR_DRIVER=ai")
	}
	lang := opts.Lang
	if lang == "" {
		lang = "eng"
	}
	// `tesseract stdin stdout -l <lang>` reads the image from stdin, writes text to stdout.
	cmd := exec.CommandContext(ctx, "tesseract", "stdin", "stdout", "-l", lang)
	cmd.Stdin = bytes.NewReader(image)
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tesseract: %v: %s", err, errBuf.String())
	}
	return out.String(), nil
}

// ── ai driver: a multimodal model via the togo `ai` plugin ──────────────────────
//
// NOTE: the current ai.Message.Content is plain text, so the image is passed as a
// data-URL inside the prompt. This works only with providers whose driver forwards
// image data-URLs to a vision model. For robust vision OCR, prefer the tesseract
// driver, or enhance the ai plugin's Message to carry structured image parts.

type aiDriver struct{ k *togo.Kernel }

func (a *aiDriver) Extract(ctx context.Context, image []byte, opts Options) (string, error) {
	svc, ok := ai.FromKernel(a.k)
	if !ok {
		return "", fmt.Errorf("ocr(ai): the `ai` plugin is not installed/configured")
	}
	mime := opts.Mime
	if mime == "" {
		mime = "image/png"
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image)
	resp, err := svc.Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: "system", Content: "You are an OCR engine. Return ONLY the text found in the image, preserving line breaks. No commentary."},
			{Role: "user", Content: "Extract all text from this image.\n\n![image](" + dataURL + ")"},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
