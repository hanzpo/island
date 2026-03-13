package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// StreamParser processes Claude Code --output-format stream-json events
// and produces formatted display lines that mirror the interactive CLI
// experience (tool call indicators, text output, result summaries).
type StreamParser struct{}

// NewStreamParser creates a StreamParser.
func NewStreamParser() *StreamParser {
	return &StreamParser{}
}

// streamEvent is the top-level envelope for stream-json output.
type streamEvent struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	Message *streamMessage  `json:"message,omitempty"`

	// Direct tool event fields (when type is tool_use at top level).
	Tool  string          `json:"tool,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// Content block streaming fields.
	ContentBlock *contentBlock `json:"content_block,omitempty"`
	Delta        *cbDelta      `json:"delta,omitempty"`

	// Result fields.
	TotalCostUSD float64 `json:"total_cost_usd,omitempty"`
	DurationMS   float64 `json:"duration_ms,omitempty"`
	DurationAPI  float64 `json:"duration_api_ms,omitempty"`
	NumTurns     int     `json:"num_turns,omitempty"`
	Result       string  `json:"result,omitempty"`

	// Session info
	SessionID string `json:"session_id,omitempty"`
}

type streamMessage struct {
	Role       string         `json:"role,omitempty"`
	Content    []contentBlock `json:"content,omitempty"`
	Model      string         `json:"model,omitempty"`
	StopReason string         `json:"stop_reason,omitempty"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	ID    string          `json:"id,omitempty"`
}

type cbDelta struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

// ParseLine parses a single stream-json line and returns formatted display
// lines. Returns nil if the event should be hidden (system events, thinking,
// tool results, etc.).
func (p *StreamParser) ParseLine(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var event streamEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		// Not valid JSON — pass through as plain text.
		return []string{line}
	}

	switch event.Type {
	case "assistant":
		return p.formatMessage(&event)
	case "message":
		return p.formatMessage(&event)
	case "tool_use":
		return p.formatToolUse(event.Tool, event.Name, event.Input)
	case "content_block_start":
		return p.formatContentBlockStart(&event)
	case "content_block_delta":
		return p.formatContentBlockDelta(&event)
	default:
		// system, init, tool_result, content_block_stop, etc. — skip.
		return nil
	}
}

func (p *StreamParser) formatMessage(event *streamEvent) []string {
	if event.Message == nil {
		return nil
	}

	var lines []string
	for _, block := range event.Message.Content {
		switch block.Type {
		case "text":
			if text := strings.TrimRight(block.Text, "\n"); text != "" {
				lines = append(lines, strings.Split(text, "\n")...)
			}
		case "tool_use":
			toolLines := p.formatToolUse(block.Name, block.Name, block.Input)
			lines = append(lines, toolLines...)
		// Skip: "thinking", "tool_result", etc.
		}
	}
	return lines
}

func (p *StreamParser) formatContentBlockStart(event *streamEvent) []string {
	if event.ContentBlock == nil {
		return nil
	}
	block := event.ContentBlock

	switch block.Type {
	case "tool_use":
		return p.formatToolUse(block.Name, block.Name, block.Input)
	case "text":
		if text := strings.TrimRight(block.Text, "\n"); text != "" {
			return strings.Split(text, "\n")
		}
	}
	return nil
}

func (p *StreamParser) formatContentBlockDelta(event *streamEvent) []string {
	if event.Delta == nil {
		return nil
	}

	if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
		text := strings.TrimRight(event.Delta.Text, "\n")
		if text != "" {
			return strings.Split(text, "\n")
		}
	}
	return nil
}

func (p *StreamParser) formatToolUse(tool, name string, input json.RawMessage) []string {
	toolName := tool
	if toolName == "" {
		toolName = name
	}
	if toolName == "" {
		return nil
	}

	summary := summarizeToolInput(toolName, input)
	if summary != "" {
		return []string{fmt.Sprintf("  ● %s(%s)", toolName, summary)}
	}
	return []string{fmt.Sprintf("  ● %s", toolName)}
}

func (p *StreamParser) formatResult(event *streamEvent) []string {
	var parts []string
	if event.TotalCostUSD > 0 {
		parts = append(parts, fmt.Sprintf("$%.4f", event.TotalCostUSD))
	}
	if event.NumTurns > 0 {
		parts = append(parts, fmt.Sprintf("%d turns", event.NumTurns))
	}
	dur := event.DurationMS
	if dur > 0 {
		secs := dur / 1000.0
		if secs >= 60 {
			parts = append(parts, fmt.Sprintf("%.0fm%.0fs", secs/60, float64(int(secs)%60)))
		} else {
			parts = append(parts, fmt.Sprintf("%.1fs", secs))
		}
	}

	if len(parts) > 0 {
		return []string{fmt.Sprintf("  ✓ Done (%s)", strings.Join(parts, ", "))}
	}
	return []string{"  ✓ Done"}
}

// summarizeToolInput extracts a brief description of a tool call's input.
func summarizeToolInput(tool string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}

	var m map[string]interface{}
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}

	switch tool {
	case "Read":
		if fp, ok := m["file_path"].(string); ok {
			return filepath.Base(fp)
		}
	case "Edit":
		if fp, ok := m["file_path"].(string); ok {
			return filepath.Base(fp)
		}
	case "Write":
		if fp, ok := m["file_path"].(string); ok {
			return filepath.Base(fp)
		}
	case "Bash":
		if cmd, ok := m["command"].(string); ok {
			if desc, ok := m["description"].(string); ok && desc != "" {
				return truncateStr(desc, 60)
			}
			return truncateStr(cmd, 60)
		}
	case "Grep":
		if pat, ok := m["pattern"].(string); ok {
			s := fmt.Sprintf(`"%s"`, truncateStr(pat, 40))
			if path, ok := m["path"].(string); ok && path != "" {
				s += " in " + filepath.Base(path)
			}
			return s
		}
	case "Glob":
		if pat, ok := m["pattern"].(string); ok {
			return pat
		}
	case "Agent":
		if desc, ok := m["description"].(string); ok {
			return truncateStr(desc, 50)
		}
	case "Skill":
		if skill, ok := m["skill"].(string); ok {
			return skill
		}
	case "WebSearch":
		if q, ok := m["query"].(string); ok {
			return truncateStr(q, 50)
		}
	case "WebFetch":
		if u, ok := m["url"].(string); ok {
			return truncateStr(u, 60)
		}
	}
	return ""
}

func truncateStr(s string, n int) string {
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}
