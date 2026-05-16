// Package mcp implements just enough of the Model Context Protocol over stdio
// (newline-delimited JSON-RPC 2.0) to expose a small set of tools to a
// running crush session. It is intentionally dependency-free.
package mcp

import "encoding/json"

const protocolVersion = "2024-11-05"

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// initializeResult is the body of the initialize response.
type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      map[string]any `json:"serverInfo"`
}

// Tool is a single tool exposed via tools/list.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// CallContent is one block of a tool result.
type CallContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// CallResult is the body of tools/call.
type CallResult struct {
	Content []CallContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// TextResult is a convenience for the common case of returning a single text
// block (typically JSON-encoded data).
func TextResult(text string) *CallResult {
	return &CallResult{Content: []CallContent{{Type: "text", Text: text}}}
}

// ErrorResult formats an error as an isError text block. Tool errors are
// returned in-band so the model sees the failure as part of the conversation
// rather than a protocol error.
func ErrorResult(text string) *CallResult {
	return &CallResult{Content: []CallContent{{Type: "text", Text: text}}, IsError: true}
}
