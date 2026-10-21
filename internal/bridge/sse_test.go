package bridge

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestParseStreamingErrorShapes(t *testing.T) {
	detail := object{"code": "invalid_request_body", "message": "Encrypted function output content could not be decrypted or decoded."}
	for name, input := range map[string]object{
		"wrapped":  {"error": detail},
		"response": {"response": object{"error": detail}},
		"topLevel": {"type": "error", "code": detail["code"], "message": detail["message"]},
	} {
		t.Run(name, func(t *testing.T) {
			got := parseError(input)
			if got["code"] != detail["code"] || got["message"] != detail["message"] {
				t.Fatal("upstream error not recognized", got)
			}
			body := object{"model": "gpt-test", "input": []any{object{"type": "agent_message", "content": []any{object{"type": "encrypted_content", "encrypted_content": "opaque"}}}}}
			next, rules, changed := repairRequest(body, input)
			if changed || rules != nil || !bytes.Equal(jsonBytes(next), jsonBytes(body)) {
				t.Fatal("decryption failure must not trigger speculative request changes")
			}
		})
	}
	for _, input := range []object{nil, {"type": "response.failed", "response": object{"error": nil}}, {"code": detail["code"], "message": detail["message"]}, {"type": "error"}} {
		if parseError(input) != nil {
			t.Fatal("unrelated or empty event interpreted as an error", input)
		}
	}
}

func TestSSEFailureDetails(t *testing.T) {
	for _, named := range []bool{false, true} {
		t.Run(map[bool]string{false: "typed", true: "named"}[named], func(t *testing.T) {
			failure := object{"type": "response.failed", "response": object{"id": "r", "status": "failed", "output": []any{}, "error": nil}}
			detail := object{"type": "error", "code": "invalid_request_body", "message": "Encrypted function output content could not be decrypted or decoded.", "param": nil}
			if named {
				delete(failure, "type")
				delete(detail, "type")
			}
			source := "event: response.failed\r\nid: first\r\ndata: " + string(jsonBytes(failure)) +
				"\r\n\r\n: keepalive\r\n\r\nretry: 1000\r\n\r\nevent: error\r\nid: second\r\ndata: " + string(jsonBytes(detail)) + "\r\n\r\n"
			var out bytes.Buffer
			var observed []object
			if err := normalizeSSE(oneByteReader{strings.NewReader(source)}, &out, func(v object) { observed = append(observed, v) }); err != nil {
				t.Fatal(err)
			}
			var frames []sseEvent
			if err := readSSE(&out, func(e sseEvent) error { frames = append(frames, e); return nil }); err != nil {
				t.Fatal(err)
			}
			if len(frames) != 4 || len(observed) != 2 {
				t.Fatalf("lost error events: frames=%d observed=%d", len(frames), len(observed))
			}
			if frames[0].lines[0] != ": keepalive" || frames[1].lines[0] != "retry: 1000" {
				t.Fatal("keepalives were lost or buffered")
			}
			got, name := frames[2].data()
			if name != "response.failed" || got["type"] != "response.failed" || nested(got, "response", "status") != "failed" ||
				nested(got, "response", "id") != "r" || nested(got, "response", "error", "code") != detail["code"] ||
				nested(got, "response", "error", "message") != detail["message"] {
				t.Fatalf("failure details not preserved: %s", jsonBytes(got))
			}
			if !bytes.Equal(jsonBytes(parseError(observed[0])), jsonBytes(parseError(observed[1]))) {
				t.Fatal("failure and following error disagree")
			}
			if frames[2].separator != "\r\n" || frames[2].lines[1] != "id: first" {
				t.Fatal("SSE framing changed")
			}
			got, _ = frames[3].data()
			if !bytes.Equal(jsonBytes(got), jsonBytes(detail)) {
				t.Fatal("following error event changed")
			}
		})
	}
}

func TestSSEOtherFramesRemainUnchanged(t *testing.T) {
	for name, source := range map[string]string{
		"success":         "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"output\":[]}}\n\n",
		"detailedFailure": "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"Original error\"}}}\n\n",
		"invalidJSON":     "event: error\ndata: invalid\n\n",
		"comments":        ": ping\n\n: goodbye\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := normalizeSSE(strings.NewReader(source), &out, func(object) {}); err != nil {
				t.Fatal(err)
			}
			if out.String() != source {
				t.Fatalf("changed stream:\n%s\nwant:\n%s", out.String(), source)
			}
		})
	}
}

func TestSSEFailureWithoutDetails(t *testing.T) {
	for _, detail := range []string{"null", "{}", `{"code":"server_error"}`} {
		for _, suffix := range []string{"", "\n\n", "\n\ndata: [DONE]\n\n", "\n\n: ping\n\n", "\n\nevent: error\ndata: invalid\n\n"} {
			source := `data: {"type":"response.failed","response":{"id":"r","status":"failed","output":[],"error":` + detail + `}}` + suffix
			var out bytes.Buffer
			var observed []object
			if err := normalizeSSE(strings.NewReader(source), &out, func(v object) { observed = append(observed, v) }); err != nil {
				t.Fatal(err)
			}
			if len(observed) != 1 || nested(observed[0], "response", "status") != "failed" ||
				!strings.Contains(str(parseError(observed[0])["message"]), "without an error message") ||
				str(parseError(observed[0])["code"]) == "" || strings.Contains(out.String(), "response.completed") {
				t.Fatal("unexplained failure or false completion", out.String())
			}
			if detail == `{"code":"server_error"}` && parseError(observed[0])["code"] != "server_error" {
				t.Fatal("original upstream code was lost")
			}
			if !strings.Contains(out.String(), str(parseError(observed[0])["message"])) {
				t.Fatal("client did not receive diagnostic")
			}
			if !strings.HasSuffix(out.String(), "\n\n") {
				t.Fatal("repaired failure must have a complete SSE frame", out.String())
			}
		}
	}
}

type brokenStreamReader struct{}

func (brokenStreamReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type brokenStreamWriter struct{ calls int }

func (w *brokenStreamWriter) Write([]byte) (int, error) {
	w.calls++
	return 0, io.ErrClosedPipe
}

func TestSSEPendingFailurePreservesIOErrors(t *testing.T) {
	source := "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":null}}\n\n"
	var out bytes.Buffer
	err := normalizeSSE(io.MultiReader(strings.NewReader(source), brokenStreamReader{}), &out, func(object) {})
	if !errors.Is(err, io.ErrUnexpectedEOF) || !strings.Contains(out.String(), "without an error message") {
		t.Fatal("lost pending failure or read error", err, out.String())
	}
	w := &brokenStreamWriter{}
	err = normalizeSSE(strings.NewReader(source+"data: {\"type\":\"error\",\"code\":\"invalid_request_body\",\"message\":\"Failed\"}\n\n"), w, func(object) {})
	if !errors.Is(err, io.ErrClosedPipe) || w.calls != 1 {
		t.Fatal("lost or retried write error", err, w.calls)
	}
	w = &brokenStreamWriter{}
	err = normalizeSSE(strings.NewReader(source+": ping\n\n"), w, func(object) {})
	if !errors.Is(err, io.ErrClosedPipe) || w.calls != 1 {
		t.Fatal("retried a failed keepalive write", err, w.calls)
	}
}
