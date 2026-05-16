package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// ToolHandler executes a single tool call. Return non-nil error only for
// protocol-level failures; surface tool-level failures via ErrorResult.
type ToolHandler func(ctx context.Context, args json.RawMessage) (*CallResult, error)

type toolEntry struct {
	def     Tool
	handler ToolHandler
}

// Server is a stdio MCP server. Register tools, then call Serve with stdin/stdout.
type Server struct {
	name    string
	version string
	tools   map[string]toolEntry
	order   []string
	mu      sync.Mutex
}

func NewServer(name, version string) *Server {
	return &Server{name: name, version: version, tools: map[string]toolEntry{}}
}

func (s *Server) RegisterTool(t Tool, h ToolHandler) {
	s.tools[t.Name] = toolEntry{def: t, handler: h}
	s.order = append(s.order, t.Name)
}

// Serve reads newline-delimited JSON-RPC from in and writes responses to out.
// Returns when in is closed.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	// MCP messages can be long (large tool results). Bump the buffer.
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(out, nil, -32700, "parse error: "+err.Error())
			continue
		}
		// Notifications have no ID — no response expected.
		if len(req.ID) == 0 {
			s.handleNotification(req)
			continue
		}
		resp := s.dispatch(ctx, req)
		s.writeMessage(out, resp)
	}
	return scanner.Err()
}

func (s *Server) handleNotification(req request) {
	// We don't currently react to notifications/initialized etc.
	_ = req
}

func (s *Server) dispatch(ctx context.Context, req request) response {
	switch req.Method {
	case "initialize":
		return response{
			JSONRPC: "2.0", ID: req.ID,
			Result: initializeResult{
				ProtocolVersion: protocolVersion,
				Capabilities:    map[string]any{"tools": map[string]any{}},
				ServerInfo:      map[string]any{"name": s.name, "version": s.version},
			},
		}
	case "tools/list":
		tools := make([]Tool, 0, len(s.order))
		for _, name := range s.order {
			tools = append(tools, s.tools[name].def)
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": tools}}
	case "tools/call":
		return s.callTool(ctx, req)
	case "ping":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	default:
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found: " + req.Method}}
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) callTool(ctx context.Context, req request) response {
	var p toolCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}}
	}
	entry, ok := s.tools[p.Name]
	if !ok {
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "unknown tool: " + p.Name}}
	}
	result, err := entry.handler(ctx, p.Arguments)
	if err != nil {
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: err.Error()}}
	}
	return response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) writeMessage(out io.Writer, v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = out.Write(append(b, '\n'))
}

func (s *Server) writeError(out io.Writer, id json.RawMessage, code int, message string) {
	s.writeMessage(out, response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

// AsJSONText marshals v to compact JSON and returns it as a text CallResult.
func AsJSONText(v any) *CallResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ErrorResult(fmt.Sprintf("marshal: %v", err))
	}
	return TextResult(string(b))
}
