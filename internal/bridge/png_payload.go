package bridge

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"strings"
	"sync"
)

const pngDataPrefix = "data:image/png;base64,"

type pngCacheEntry struct {
	key   [32]byte
	value string
}

// This in-memory cache avoids recompressing image history on every turn.
// Both its byte size and entry count are bounded, including unchanged images.
type pngPayloadOptimizer struct {
	mu      sync.Mutex
	entries []pngCacheEntry
	bytes   int
}

func (c *pngPayloadOptimizer) cached(key [32]byte) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, entry := range c.entries {
		if entry.key == key {
			copy(c.entries[i:], c.entries[i+1:])
			c.entries[len(c.entries)-1] = entry
			return entry.value, true
		}
	}
	return "", false
}

func (c *pngPayloadOptimizer) remember(key [32]byte, value string) {
	const budget = 64 << 20
	if len(value) > budget {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, entry := range c.entries {
		if entry.key == key {
			return
		}
	}
	for len(c.entries) > 0 && (len(c.entries) >= 32 || c.bytes+len(value) > budget) {
		c.bytes -= len(c.entries[0].value)
		c.entries[0] = pngCacheEntry{}
		c.entries = c.entries[1:]
	}
	c.entries = append(c.entries, pngCacheEntry{key, value})
	c.bytes += len(value)
}

func (c *pngPayloadOptimizer) imageURL(original string) string {
	if len(original) < 64<<10 || !strings.HasPrefix(original, pngDataPrefix) {
		return original
	}
	key := sha256.Sum256([]byte(original))
	if cached, ok := c.cached(key); ok {
		if cached != "" {
			return cached
		}
		return original
	}
	data, err := base64.StdEncoding.DecodeString(original[len(pngDataPrefix):])
	optimized := ""
	if err == nil {
		if compressed, err := recompressPNG(data); err == nil && len(compressed) < len(data) {
			candidate := pngDataPrefix + base64.StdEncoding.EncodeToString(compressed)
			if len(candidate) < len(original) {
				optimized = candidate
			}
		}
	}
	c.remember(key, optimized)
	if optimized == "" {
		return original
	}
	return optimized
}

// Only declared image content is transformed. Never search text, tool arguments,
// schemas or metadata for embedded image-like strings.
func (c *pngPayloadOptimizer) optimize(request object) int {
	changed := 0
	content := func(parts []any) {
		for _, part := range parts {
			image := obj(part)
			if image["type"] != "input_image" {
				continue
			}
			original := str(image["image_url"])
			if next := c.imageURL(original); next != original {
				image["image_url"] = next
				changed++
			}
		}
	}
	for _, item := range arr(request["input"]) {
		entry := obj(item)
		switch str(entry["type"]) {
		case "message":
			content(arr(entry["content"]))
		case "":
			if str(entry["role"]) != "" {
				content(arr(entry["content"]))
			}
		case "function_call_output", "custom_tool_call_output":
			content(arr(entry["output"]))
		}
	}
	return changed
}

// Recompress only IDAT's zlib stream, keeping the inflated scanlines identical.
// Every other chunk (including color profiles and metadata) remains byte exact.
// This does not decode/re-encode pixels, change PNG filters or remove alpha.
func recompressPNG(data []byte) ([]byte, error) {
	invalid := errors.New("unsupported or invalid PNG")
	if len(data) < 8 || !bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return nil, invalid
	}
	first, end := -1, -1
	readers := []io.Reader{}
	finished := false
	for pos := 8; pos < len(data); {
		if len(data)-pos < 12 {
			return nil, invalid
		}
		n := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		if n > len(data)-pos-12 {
			return nil, invalid
		}
		next := pos + 12 + n
		kind := string(data[pos+4 : pos+8])
		if crc32.ChecksumIEEE(data[pos+4:pos+8+n]) != binary.BigEndian.Uint32(data[pos+8+n:next]) {
			return nil, invalid
		}
		if pos == 8 && (kind != "IHDR" || n != 13) {
			return nil, invalid
		}
		if kind == "IDAT" {
			if first == -1 {
				first = pos
			} else if end != pos {
				return nil, invalid
			}
			readers = append(readers, bytes.NewReader(data[pos+8:pos+8+n]))
			end = next
		}
		if kind == "IEND" {
			if n != 0 || next != len(data) {
				return nil, invalid
			}
			finished = true
		}
		pos = next
	}
	if first == -1 || !finished {
		return nil, invalid
	}
	reader, err := zlib.NewReader(io.MultiReader(readers...))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	// Bound decompression at the untrusted image boundary, not just JSON size.
	const expandedLimit = 128 << 20
	n, copyErr := io.Copy(writer, io.LimitReader(reader, expandedLimit+1))
	closeErr := writer.Close()
	if err = errors.Join(copyErr, closeErr); err != nil {
		return nil, err
	}
	if n > expandedLimit {
		return nil, errors.New("PNG inflated data exceeds compression limit")
	}
	if compressed.Len()+12 >= end-first {
		return data, nil
	}
	out := make([]byte, 0, first+compressed.Len()+12+len(data)-end)
	out = append(out, data[:first]...)
	chunk := make([]byte, 12+compressed.Len())
	binary.BigEndian.PutUint32(chunk[:4], uint32(compressed.Len()))
	copy(chunk[4:8], "IDAT")
	copy(chunk[8:], compressed.Bytes())
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	out = append(out, chunk...)
	out = append(out, data[end:]...)
	return out, nil
}
