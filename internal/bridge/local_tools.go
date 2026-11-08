package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type localCustomTools map[[2]string]bool

func localToolKey(tool object) [2]string {
	return [2]string{str(tool["namespace"]), str(tool["name"])}
}

func prepareLocalCustomTools(body object) (localCustomTools, error) {
	custom := localCustomTools{}
	seen := map[[2]string]bool{}
	var convert func([]any, string) error
	convert = func(tools []any, namespace string) error {
		for _, entry := range tools {
			tool := obj(entry)
			if tool["type"] == "namespace" {
				if err := convert(arr(tool["tools"]), str(tool["name"])); err != nil {
					return err
				}
			}
			key := [2]string{namespace, str(tool["name"])}
			if tool["type"] == "function" || tool["type"] == "custom" {
				if seen[key] && (custom[key] || tool["type"] == "custom") {
					return errors.New("Local freeform tools must have distinct names within their namespace")
				}
				seen[key] = true
			}
			if tool["type"] != "custom" {
				continue
			}
			name := str(tool["name"])
			if name == "" {
				return errors.New("A local freeform tool must have a name")
			}
			custom[[2]string{namespace, name}] = true
			description := str(tool["description"]) + "\nSupply the complete raw tool input in the input string, without Markdown fences."
			if format := obj(tool["format"]); str(format["definition"]) != "" {
				description += "\nInput grammar:\n" + str(format["definition"])
			}
			delete(tool, "format")
			tool["type"], tool["description"] = "function", description
			tool["parameters"] = object{"type": "object", "properties": object{
				"input": object{"type": "string", "description": "The complete raw input for this tool."}},
				"required": []any{"input"}, "additionalProperties": false}
		}
		return nil
	}
	if err := convert(arr(body["tools"]), ""); err != nil {
		return nil, err
	}
	for _, entry := range arr(body["input"]) {
		item := obj(entry)
		switch item["type"] {
		case "custom_tool_call":
			input, ok := item["input"].(string)
			if !ok {
				return nil, errors.New("Local freeform tool history must contain a string input")
			}
			item["type"], item["arguments"] = "function_call", string(jsonBytes(object{"input": input}))
			delete(item, "input")
		case "custom_tool_call_output":
			item["type"] = "function_call_output"
		}
	}
	if choice := obj(body["tool_choice"]); choice["type"] == "custom" {
		choice["type"] = "function"
	}
	return custom, nil
}

func localCustomInput(arguments string) (string, error) {
	var value struct {
		Input *string `json:"input"`
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("Invalid local freeform tool arguments: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", errors.New("Local freeform tool arguments must contain one JSON object")
	}
	if value.Input == nil {
		return "", errors.New("Local freeform tool arguments must contain an input string")
	}
	return *value.Input, nil
}

func (custom localCustomTools) restoreItem(item object, partial bool) error {
	if item["type"] != "function_call" || !custom[localToolKey(item)] {
		return nil
	}
	input := ""
	if !partial {
		arguments, ok := item["arguments"].(string)
		if !ok {
			return errors.New("Local freeform tool response is missing its arguments")
		}
		var err error
		input, err = localCustomInput(arguments)
		if err != nil {
			return err
		}
	}
	item["type"], item["input"] = "custom_tool_call", input
	delete(item, "arguments")
	return nil
}

func (custom localCustomTools) restoreResponse(response object) error {
	status := str(response["status"])
	partial := status == "in_progress" || status == "queued" || status == "failed" || status == "incomplete"
	for _, entry := range arr(response["output"]) {
		if err := custom.restoreItem(obj(entry), partial); err != nil {
			return err
		}
	}
	return nil
}

type localCustomCall struct {
	itemID    string
	callID    string
	key       [2]string
	arguments strings.Builder
	input     string
	done      bool
	itemDone  bool
}

type localCustomStreamError struct {
	err        error
	responseID string
	sequence   int
}

func (err *localCustomStreamError) Error() string { return err.err.Error() }
func (err *localCustomStreamError) Unwrap() error { return err.err }

func (custom localCustomTools) restoreStream(source io.Reader, output io.Writer) error {
	calls := map[int]*localCustomCall{}
	sequence := 0
	responseID := "resp_local_tool_error"
	terminal := errors.New("local response complete")
	write := func(e sseEvent, event object) error {
		if event != nil {
			if strings.HasPrefix(str(event["type"]), "response.") {
				event["sequence_number"] = sequence
				sequence++
			}
			e = e.withData(event)
			for i, line := range e.lines {
				if strings.HasPrefix(line, "event:") {
					e.lines[i] = "event: " + str(event["type"])
				}
			}
		}
		_, err := io.WriteString(output, strings.Join(e.lines, "\n")+"\n\n")
		return err
	}
	emitInput := func(call *localCustomCall, index int, arguments string) error {
		input, err := localCustomInput(arguments)
		if err != nil {
			return err
		}
		if call.done {
			if call.input != input {
				return errors.New("Local freeform tool input changed after completion")
			}
			return nil
		}
		for _, kind := range []string{"delta", "done"} {
			event := object{"type": "response.custom_tool_call_input." + kind, "item_id": call.itemID, "output_index": index}
			if kind == "delta" {
				event["delta"] = input
			} else {
				event["input"] = input
			}
			e := sseEvent{lines: []string{"event: " + str(event["type"]), "data: {}"}, separator: "\n"}
			if err := write(e, event); err != nil {
				return err
			}
		}
		call.input, call.done = input, true
		return nil
	}
	err := readSSE(source, func(e sseEvent) error {
		event, name := e.data()
		if event == nil {
			return write(e, nil)
		}
		kind := str(event["type"])
		if kind == "" {
			kind = name
			event["type"] = kind
		}
		index := int(number(event["output_index"]))
		item := obj(event["item"])
		if kind == "response.output_item.added" && item["type"] == "function_call" && custom[localToolKey(item)] {
			calls[index] = &localCustomCall{itemID: str(item["id"]), callID: str(item["call_id"]), key: localToolKey(item)}
			if err := custom.restoreItem(item, true); err != nil {
				return err
			}
		}
		if call := calls[index]; call != nil {
			switch kind {
			case "response.function_call_arguments.delta":
				delta, ok := event["delta"].(string)
				if !ok || call.arguments.Len()+len(delta) > 16<<20 {
					return errors.New("Invalid or oversized local freeform tool arguments")
				}
				call.arguments.WriteString(delta)
				return nil
			case "response.function_call_arguments.done":
				arguments, ok := event["arguments"].(string)
				if !ok {
					arguments = call.arguments.String()
				}
				return emitInput(call, index, arguments)
			case "response.output_item.done":
				if item["type"] != "function_call" || localToolKey(item) != call.key ||
					str(item["id"]) != call.itemID || str(item["call_id"]) != call.callID {
					return errors.New("Local freeform tool identity changed during streaming")
				}
				if err := emitInput(call, index, str(item["arguments"])); err != nil {
					return err
				}
				call.itemDone = true
			}
		}
		if err := custom.restoreItem(item, kind == "response.output_item.added"); err != nil {
			return err
		}
		if response := obj(event["response"]); response != nil {
			if id := str(response["id"]); id != "" {
				responseID = id
			}
			if err := custom.restoreResponse(response); err != nil {
				return err
			}
			if kind == "response.completed" {
				items := arr(response["output"])
				for i, entry := range items {
					item := obj(entry)
					if item["type"] == "custom_tool_call" {
						call := calls[i]
						if call == nil || !call.itemDone || call.input != str(item["input"]) || call.itemID != str(item["id"]) ||
							call.callID != str(item["call_id"]) || call.key != localToolKey(item) {
							return errors.New("Local freeform stream completed with inconsistent tool output")
						}
					}
				}
				for index, call := range calls {
					if !call.itemDone || index < 0 || index >= len(items) || obj(items[index])["type"] != "custom_tool_call" {
						return errors.New("Local freeform stream completed before its tool output")
					}
				}
			}
		}
		if err := write(e, event); err != nil {
			return err
		}
		if kind == "response.completed" || kind == "response.failed" || kind == "response.incomplete" {
			return terminal
		}
		return nil
	})
	if errors.Is(err, terminal) {
		return nil
	}
	if err == nil {
		err = errors.New("Local freeform response stream ended without a terminal event")
	}
	return &localCustomStreamError{err, responseID, sequence}
}

type localCustomStreamBody struct {
	*io.PipeReader
	source io.ReadCloser
}

func (body *localCustomStreamBody) Close() error {
	return errors.Join(body.PipeReader.Close(), body.source.Close())
}

func (l *localManager) restoreCustomResponse(ctx context.Context, response *http.Response, custom localCustomTools) error {
	if response.StatusCode != http.StatusOK {
		return nil
	}
	response.Header.Del("Content-Length")
	response.ContentLength = -1
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		source := response.Body
		reader, writer := io.Pipe()
		response.Body = &localCustomStreamBody{reader, source}
		go func() {
			defer source.Close()
			defer writer.Close()
			if err := custom.restoreStream(source, writer); err != nil && ctx.Err() == nil {
				l.app.report(err)
				streamError := &localCustomStreamError{err: err, responseID: "resp_local_tool_error"}
				errors.As(err, &streamError)
				event := object{"type": "response.failed", "sequence_number": streamError.sequence,
					"response": object{"id": streamError.responseID, "object": "response", "status": "failed", "output": []any{},
						"error": object{"code": "invalid_local_tool_response", "message": err.Error()}}}
				io.WriteString(writer, "event: response.failed\ndata: "+string(jsonBytes(event))+"\n\n")
			}
		}()
		return nil
	}
	data, err := readResponse(response, 256<<20)
	if err != nil {
		return err
	}
	var result object
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("Invalid local Responses API output: %w", err)
	}
	if result == nil {
		return errors.New("Local Responses API output must be a JSON object")
	}
	if err := custom.restoreResponse(result); err != nil {
		return err
	}
	response.Body = io.NopCloser(strings.NewReader(string(jsonBytes(result))))
	return nil
}
