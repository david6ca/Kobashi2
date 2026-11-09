package bridge

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func pngFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 512, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 512; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 71, uint8(x + y)})
		}
	}
	var data bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.NoCompression}
	if err := encoder.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func pngTestChunk(kind string, payload []byte) []byte {
	chunk := make([]byte, len(payload)+12)
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(payload)))
	copy(chunk[4:8], kind)
	copy(chunk[8:], payload)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	return chunk
}

func pngStreams(t *testing.T, data []byte) (metadata, scanlines []byte) {
	t.Helper()
	var idat bytes.Buffer
	for pos := 8; pos < len(data); {
		n := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		next := pos + n + 12
		if string(data[pos+4:pos+8]) == "IDAT" {
			idat.Write(data[pos+8 : pos+8+n])
		} else {
			metadata = append(metadata, data[pos:next]...)
		}
		pos = next
	}
	r, err := zlib.NewReader(&idat)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	scanlines, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func assertLosslessPNG(t *testing.T, original, compressed []byte) {
	t.Helper()
	beforeMetadata, beforeScanlines := pngStreams(t, original)
	afterMetadata, afterScanlines := pngStreams(t, compressed)
	if !bytes.Equal(beforeMetadata, afterMetadata) || !bytes.Equal(beforeScanlines, afterScanlines) {
		t.Fatal("PNG metadata or inflated image data changed")
	}
	before, err := png.Decode(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	after, err := png.Decode(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	if before.Bounds() != after.Bounds() {
		t.Fatal("image dimensions changed")
	}
	for y := before.Bounds().Min.Y; y < before.Bounds().Max.Y; y++ {
		for x := before.Bounds().Min.X; x < before.Bounds().Max.X; x++ {
			br, bg, bb, ba := before.At(x, y).RGBA()
			ar, ag, ab, aa := after.At(x, y).RGBA()
			if br != ar || bg != ag || bb != ab || ba != aa {
				t.Fatal("pixel color or alpha changed", x, y)
			}
		}
	}
}

func TestPNGRecompressionPreservesPixelsAndMetadata(t *testing.T) {
	original := pngFixture(t)
	// Preserve color information as well as arbitrary textual metadata. Test
	// chunks before and after IDAT, and multiple consecutive IDAT chunks.
	_, raw := pngStreams(t, original)
	var encoded bytes.Buffer
	w, err := zlib.NewWriterLevel(&encoded, zlib.NoCompression)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(raw)
	_ = w.Close()
	data := append([]byte{}, original[:33]...)
	data = append(data, pngTestChunk("gAMA", []byte{0, 0, 177, 143})...)
	data = append(data, pngTestChunk("sRGB", []byte{0})...)
	data = append(data, pngTestChunk("tEXt", []byte("Description\x00reference image"))...)
	mid := encoded.Len() / 2
	data = append(data, pngTestChunk("IDAT", encoded.Bytes()[:mid])...)
	data = append(data, pngTestChunk("IDAT", encoded.Bytes()[mid:])...)
	data = append(data, pngTestChunk("tEXt", []byte("Author\x00preserve me"))...)
	data = append(data, pngTestChunk("IEND", nil)...)
	compressed, err := recompressPNG(data)
	if err != nil || len(compressed) >= len(data) {
		t.Fatal("fixture did not compress", err, len(data), len(compressed))
	}
	assertLosslessPNG(t, data, compressed)
	unchanged := pngFixture(t)
	unchanged[40] ^= 1 // Invalid CRC must not be silently repaired.
	var optimizer pngPayloadOptimizer
	for _, url := range []string{
		pngDataPrefix + base64.StdEncoding.EncodeToString(unchanged),
		pngDataPrefix + strings.Repeat("!", 70<<10),
		"data:image/jpeg;base64," + strings.Repeat("A", 70<<10),
		"https://example.invalid/reference.png",
	} {
		if optimizer.imageURL(url) != url {
			t.Fatal("unsupported or invalid image was changed")
		}
	}
}

func TestPNGProxyOptimizationPreservesRequestSemantics(t *testing.T) {
	for _, client := range []string{"Codex", "Copilot"} {
		t.Run(client, func(t *testing.T) {
			logs := captureDiagnostics(t)
			original := pngFixture(t)
			url := pngDataPrefix + base64.StdEncoding.EncodeToString(original)
			content := []any{object{"type": "input_image", "image_url": url, "detail": "original"}, object{"type": "input_text", "text": "private prompt"}}
			request := object{"model": "gpt-test", "stream": false, "max_output_tokens": json.Number("9007199254740993"),
				"input": []any{
					object{"role": "user", "content": content},
					object{"type": "custom_tool_call_output", "call_id": "call-1", "output": content},
					object{"type": "function_call_output", "call_id": "call-2", "output": content},
					object{"type": "function_call", "name": "image_gen", "arguments": string(jsonBytes(object{"reference": url}))},
				},
				"metadata": object{"reference": url},
				"tools":    []any{object{"type": "function", "name": "image_gen", "parameters": object{"type": "input_image", "image_url": url}}}}
			payload := jsonBytes(request)
			calls := 0
			a := testApp(t, func(r *http.Request) (*http.Response, error) {
				calls++
				data, _ := io.ReadAll(r.Body)
				if len(data) >= len(payload) {
					t.Fatal("payload was not reduced")
				}
				got, err := readObject(bytes.NewReader(data), 2<<20)
				if err != nil {
					t.Fatal(err)
				}
				for i, key := range []string{"content", "output", "output"} {
					part := obj(arr(obj(arr(got["input"])[i])[key])[0])
					compressed, err := base64.StdEncoding.DecodeString(str(part["image_url"])[len(pngDataPrefix):])
					if err != nil {
						t.Fatal(err)
					}
					assertLosslessPNG(t, original, compressed)
					// Restore only the image URL before comparing all other fields.
					part["image_url"] = url
				}
				if !bytes.Equal(jsonBytes(got), payload) {
					t.Fatal("text, tools, metadata, numeric values or image references changed")
				}
				return response(200, object{"status": "completed"}), nil
			})
			a.state = session{Token: "account", Codex: true, Copilot: true}
			a.bearer, a.expiry = "current", float64(time.Now().Add(time.Hour).Unix())
			a.models.sessionStarted = true
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(payload))
			if client == "Codex" {
				a.codex(w, r)
			} else {
				r.Header.Set("Authorization", "Bearer PROXY_MANAGED")
				a.copilotApp(w, r)
			}
			if calls != 1 || w.Code != 200 || !strings.Contains(logs.String(), "lossless_png images=3") {
				t.Fatal("unexpected forwarding or missing compression diagnostics", calls, w.Code)
			}
			if strings.Contains(logs.String(), "private prompt") || strings.Contains(logs.String(), "base64,") {
				t.Fatal("image or prompt leaked into logs")
			}
		})
	}
}

func TestPNGOptimizerConcurrentCache(t *testing.T) {
	original := pngFixture(t)
	url := pngDataPrefix + base64.StdEncoding.EncodeToString(original)
	var optimizer pngPayloadOptimizer
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			next := optimizer.imageURL(url)
			compressed, err := base64.StdEncoding.DecodeString(next[len(pngDataPrefix):])
			if err != nil || len(compressed) >= len(original) {
				t.Error("concurrent optimization failed", err)
			}
		})
	}
	workers.Wait()
	if len(optimizer.entries) != 1 {
		t.Fatal("duplicate cache entries for the same image")
	}
	for i := range 40 {
		optimizer.remember([32]byte{byte(i + 1)}, "")
	}
	if len(optimizer.entries) > 32 {
		t.Fatal("unchanged image cache grew without a bound")
	}
}

// Optional offline verification against a recorded session; never sends images
// to an API and reports only aggregate sizes, not session content.
func TestPNGRecordedPreviewCompression(t *testing.T) {
	path := os.Getenv("KOBASHI_TEST_PNG_SESSION")
	if path == "" {
		t.Skip("KOBASHI_TEST_PNG_SESSION is not set")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	var optimizer pngPayloadOptimizer
	before, after, count := 0, 0, 0
	started := time.Now()
	for {
		var record object
		if err := decoder.Decode(&record); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		for _, value := range arr(obj(record["payload"])["output"]) {
			part := obj(value)
			url := str(part["image_url"])
			if part["type"] != "input_image" || !strings.HasPrefix(url, "data:image/") {
				continue
			}
			next := optimizer.imageURL(url)
			before += len(url)
			after += len(next)
			count++
			if next != url {
				original, _ := base64.StdEncoding.DecodeString(url[len(pngDataPrefix):])
				compressed, _ := base64.StdEncoding.DecodeString(next[len(pngDataPrefix):])
				assertLosslessPNG(t, original, compressed)
			}
		}
	}
	if count == 0 || after >= before {
		t.Fatal("recorded images did not compress", count, before, after)
	}
	t.Logf("images=%d original_encoded_bytes=%d compressed_encoded_bytes=%d saved_bytes=%d duration_ms=%d", count, before, after, before-after, time.Since(started).Milliseconds())
}
