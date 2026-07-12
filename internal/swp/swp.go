package swp

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

type MessageType string

const (
	DEPLOY MessageType = "DEPLOY"
	EXEC   MessageType = "EXEC"
	PING   MessageType = "PING"
)

type WireMesage struct {
	Type MessageType `json:"type"`
	ID   string      `json:"id"`
	Data interface{} `json:"data"`
}

type DeployPayload struct {
	Hash         string `json:"hash"`
	ContractName string `json:"contract_name"`
	Version      string `json:"version"`
	Owner        string `json:"owner"`
	Source       []byte `json:"source"`
}

type ToolAction struct {
	Type      string   `json:"type"`
	Method    string   `json:"method,omitempty"`
	Url       string   `json:"url,omitempty"`
	Operation string   `json:"operation,omitempty"`
	Path      string   `json:"path,omitempty"`
	Command   string   `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Agent     string            `json:"agent,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
}

type ToolStepInput struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ToolStep struct {
	Function string          `json:"function"`
	Input    []ToolStepInput `json:"input,omitempty"`
	Action   *ToolAction     `json:"action,omitempty"`
}

type ToolStmt struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Steps       []ToolStep `json:"steps"`
}

type ModelStmt struct {
	Provider    string  `json:"provider"`
	Name        string  `json:"name"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

type BehaviorStmt struct {
	SystemPrompt string `json:"system_prompt"`
	MaxSteps     int    `json:"max_steps"`
	OnDeny       string `json:"on_deny"`
	OnError      string `json:"on_error"`
}

type SkillStmt struct {
	Name    string   `json:"name"`
	Content string   `json:"content"`
	Uses    []string `json:"uses"`
}

type TriggerStmt struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

type AgentInfo struct {
	Hash     string `json:"hash"`
	Name     string `json:"name"`
	Purpose  string `json:"purpose"`
	Tools    []ToolStmt
	Model    ModelStmt
	Behavior BehaviorStmt
	Skills   []SkillStmt   `json:"skills"`
	Triggers []TriggerStmt `json:"triggers"`
}

type ArtifactMetadata struct {
	Bytecode     []byte                 `json:"bytecode"`
	ConstPool    []interface{}          `json:"const_pool"`
	Functions    map[string]interface{} `json:"functions"`
	FunctionName map[int]string         `json:"function_name"`
	Types        map[string]interface{} `json:"types"`
	InitStorage  map[int]interface{}    `json:"init_storage"`
	AgentInfo    AgentInfo              `json:"agent_info"`
}

type AgentMeta struct {
	Hash    string `json:"hash"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type DeployResponse struct {
	Agent            AgentMeta        `json:"agent"`
	ContractHash     string           `json:"contract_hash"`
	ContractName     string           `json:"contract_name"`
	ContractOwner    string           `json:"contract_owner"`
	ContractVersion  string           `json:"contract_version"`
	Functions        []string         `json:"functions"`
	ContractArtifact ArtifactMetadata `json:"contract_artifact"`
}

type ExecPayload struct {
	ArtifactHash     string           `json:"contract_id"`
	ContractArtifact ArtifactMetadata `json:"contract_artifact"`
	Function         string           `json:"function"`
	Args             map[string]any   `json:"args"`
	ContextId        string           `json:"context_id,omitempty"`
	CallerID         string           `json:"caller_id"`
	CoreArgs         map[string]any
}

type TraceLog struct {
	Step       string `json:"step"`
	Msg        string `json:"msg"`
	DurationMs int64  `json:"duration"`
}

type AuditLog struct {
	Time  string `json:"time"`
	Event string `json:"event"`
	Actor string `json:"actor"`
}

type ExecResponse struct {
	ArtifactHash string        `json:"artifact_hash"`
	Function     string        `json:"function"`
	Journal      []interface{} `json:"journal"`
	Timestamp    int64         `json:"timestamp"`
}

type PingPayload struct {
	Timestamp int64 `json:"timestamp"`
}

type WireResponse struct {
	Type    MessageType     `json:"type"`
	ID      string          `json:"id"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   interface{}     `json:"error,omitempty"`
}

// ErrorString returns a flat string representation of Error, which may arrive
// from the VVM as a plain string or as a structured object like
// {"code": "...", "message": "..."}.
func (r *WireResponse) ErrorString() string {
	switch v := r.Error.(type) {
	case nil:
		return ""
	case string:
		return v
	case map[string]interface{}:
		code, _ := v["code"].(string)
		msg, _ := v["message"].(string)
		switch {
		case code != "" && msg != "":
			return code + ": " + msg
		case msg != "":
			return msg
		case code != "":
			return code
		}
		b, _ := json.Marshal(v)
		return string(b)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

type SwpClient struct {
	addr string
	conn net.Conn
	mu   sync.Mutex
}

func NewSwpClient(addr string) *SwpClient {
	return &SwpClient{
		addr: addr,
	}
}

func (sc *SwpClient) Connect() error {
	conn, err := net.Dial("tcp", sc.addr)
	if err != nil {
		panic(err)
	}
	sc.conn = conn
	return nil
}

func (sc *SwpClient) Send(msg WireMesage, resp any) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	return sc.sendWithRetry(msg, resp, true)
}

func (sc *SwpClient) sendWithRetry(msg WireMesage, resp any, canRetry bool) error {
	fmt.Printf("[SWP] Sending message type=%s id=%s (retry=%v)\n", msg.Type, msg.ID, !canRetry)

	if err := Encode(sc.conn, msg); err != nil {
		fmt.Printf("[SWP] Encode error: %v\n", err)
		if canRetry {
			if err := sc.reconnect(); err != nil {
				return err
			}
			return sc.sendWithRetry(msg, resp, false)
		}
		return err
	}
	fmt.Println("[SWP] Encode success, waiting for response...")

	if err := Decode(sc.conn, resp); err != nil {
		fmt.Printf("[SWP] Decode error: %v\n", err)
		if canRetry {
			if err := sc.reconnect(); err != nil {
				return err
			}
			return sc.sendWithRetry(msg, resp, false)
		}
		return err
	}
	fmt.Println("[SWP] Decode success")

	return nil
}

func (sc *SwpClient) reconnect() error {
	fmt.Println("[SWP] Reconnecting...")
	if sc.conn != nil {
		sc.conn.Close()
	}
	conn, err := net.Dial("tcp", sc.addr)
	if err != nil {
		fmt.Printf("[SWP] Reconnect failed: %v\n", err)
		return err
	}
	sc.conn = conn
	fmt.Println("[SWP] Reconnected!")
	return nil
}

func (sc *SwpClient) Close() error {
	return sc.conn.Close()
}
