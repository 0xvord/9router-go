package proxy

// DARKWAVE persona injector — Go port of ~/.9router/ltx-hook.js semantics.
// Config (SHARED with srouter, fail-closed): ~/.srouter/darkwave/config.json
//   { "enabled": true, "core_path": "...", "models": [] }
//   models: [] = all models; otherwise case-insensitive substring match.
// Marker idempotency identical to srouter/JS: "DARKWAVE.EXE — System Prompt".
// Dialects (identical to ltx-hook.injectPersona):
//   messages[]    -> unshift {role:system, content:core}   (OpenAI, LEADING)
//   input present  -> instructions = core + "\n\n" + old   (Responses)
//   system string  -> core + "\n\n" + old ; array -> unshift text block (Anthropic)
//   max_tokens only-> system = core                        (Anthropic w/o system)
// BOZ: intentionally absent — hook chain order LTX -> BOZ means persona's system
// message makes every request non-bare, so BOZ (bare-requests-only) never fires;
// suppressing BOZ via LTX is the intended semantics per ltx-hook.js comment.
// thinking-suppress: config enabled:false (inert) — not ported.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	personaCfgPath      = "/home/ubuntu/.srouter/darkwave/config.json"
	personaDefaultCore  = "/home/ubuntu/.hermes/personas/DARKWAVE_CORE.md"
	personaMarker       = "DARKWAVE.EXE — System Prompt"
	personaSkipHostList = "localhost,127.0.0.1,0.0.0.0,::1,10.11.0.10"
)

// Mirrors JS isLlmUrl path regex: /(chat/completions|completions|messages|responses)$
var personaLLMPath = regexp.MustCompile(`/(chat/completions|completions|messages|responses)$`)
var personaSkipHosts = map[string]bool{}

func init() {
	for _, h := range strings.Split(personaSkipHostList, ",") {
		personaSkipHosts[h] = true
	}
}

type personaConfig struct {
	Enabled   bool
	CorePath  string
	Models    []string
	mtimeMs   int64
}

type personaCoreEntry struct {
	Value string
	Path  string
	mtimeMs int64
}

var personaMu sync.Mutex
var personaCfg *personaConfig
var personaCore *personaCoreEntry

func personaLoadConfig() personaConfig {
	fail := personaConfig{Enabled: false, CorePath: personaDefaultCore, Models: []string{}}
	fi, err := os.Stat(personaCfgPath)
	if err != nil {
		return fail
	}
	mt := fi.ModTime().UnixMilli()
	personaMu.Lock()
	if personaCfg != nil && personaCfg.mtimeMs == mt {
		cfg := *personaCfg
		personaMu.Unlock()
		return cfg
	}
	personaMu.Unlock()

	raw, err := os.ReadFile(personaCfgPath)
	if err != nil {
		return fail
	}
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) != nil {
		return fail
	}
	cfg := personaConfig{Enabled: false, CorePath: personaDefaultCore, Models: []string{}, mtimeMs: mt}
	if v, ok := m["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := m["core_path"].(string); ok && v != "" {
		cfg.CorePath = v
	}
	if arr, ok := m["models"].([]interface{}); ok {
		for _, e := range arr {
			if s, ok := e.(string); ok {
				cfg.Models = append(cfg.Models, s)
			}
		}
	}
	personaMu.Lock()
	personaCfg = &cfg
	personaMu.Unlock()
	return cfg
}

func personaLoadCore(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	mt := fi.ModTime().UnixMilli()
	personaMu.Lock()
	if personaCore != nil && personaCore.Path == path && personaCore.mtimeMs == mt {
		v := personaCore.Value
		personaMu.Unlock()
		return v
	}
	personaMu.Unlock()

	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	val := string(b)
	personaMu.Lock()
	personaCore = &personaCoreEntry{Value: val, Path: path, mtimeMs: mt}
	personaMu.Unlock()
	return val
}

func personaModelMatches(id string, models []string) bool {
	if len(models) == 0 {
		return true
	}
	lower := strings.ToLower(id)
	for _, m := range models {
		if strings.Contains(lower, strings.ToLower(m)) {
			return true
		}
	}
	return false
}

// personaIsLlmURL mirrors JS isLlmUrl.
func personaIsLlmURL(raw string) bool {
	// cheap prefix gate before URL parse
	if !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "http://") {
		return false
	}
	// strip scheme
	rest := raw
	if i := strings.Index(raw, "://"); i >= 0 {
		rest = raw[i+3:]
	}
	hostPath := rest
	host := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		host = rest[:i]
		hostPath = rest[i:]
	} else {
		hostPath = "/"
	}
	// strip port
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	// strip query/fragment from path
	if i := strings.IndexAny(hostPath, "?#"); i >= 0 {
		hostPath = hostPath[:i]
	}
	if personaSkipHosts[host] {
		return false
	}
	return personaLLMPath.MatchString(hostPath)
}

// ApplyPersona mutates an outbound LLM body per ltx-hook.injectPersona.
// Returns the (possibly unchanged) body and whether it changed.
func ApplyPersona(url string, body []byte) ([]byte, bool) {
	if len(body) == 0 || !personaIsLlmURL(url) {
		return body, false
	}
	// idempotency: marker already present -> untouched (mirrors JS includes(MARKER))
	if bytes.Contains(body, []byte(personaMarker)) {
		return body, false
	}
	// cheap gate: chat payloads always carry "model"
	if !bytes.Contains(body, []byte(`"model"`)) {
		return body, false
	}

	cfg := personaLoadConfig()
	if !cfg.Enabled {
		return body, false
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // exact number round-trip (snowflake ids, temps)
	var payload map[string]interface{}
	if err := dec.Decode(&payload); err != nil {
		return body, false
	}

	model, _ := payload["model"].(string)
	if model == "" {
		return body, false
	}
	if !personaModelMatches(model, cfg.Models) {
		return body, false
	}

	core := personaLoadCore(cfg.CorePath)
	if core == "" || !strings.Contains(core, personaMarker) {
		return body, false
	}

	changed := false
	if msgs, ok := payload["messages"].([]interface{}); ok {
		payload["messages"] = append([]interface{}{map[string]interface{}{"role": "system", "content": core}}, msgs...)
		changed = true
	} else if _, has := payload["input"]; has {
		old, _ := payload["instructions"].(string)
		payload["instructions"] = core + "\n\n" + old
		changed = true
	} else if sys, has := payload["system"]; has {
		switch s := sys.(type) {
		case string:
			payload["system"] = core + "\n\n" + s
		case []interface{}:
			payload["system"] = append([]interface{}{map[string]interface{}{"type": "text", "text": core}}, s...)
		default:
			payload["system"] = core
		}
		changed = true
	} else if _, has := payload["max_tokens"]; has {
		payload["system"] = core
		changed = true
	}
	if !changed {
		return body, false
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return body, false
	}
	log.Printf("[DARKWAVE] chain applied -> %s (%dB) via %s", model, len(out), url)
	return out, true
}

// personaDBG mirrors [DARKWAVE-DBG] diagnostics line.
func personaDBG(url string, headers map[string]string, body []byte) {
	auth := ""
	for k, v := range headers {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "authorization") || strings.Contains(lk, "api-key") {
			if len(v) > 18 {
				auth = v[:14] + "..." + v[len(v)-4:] + " len=" + fmt.Sprintf("%d", len(v))
			} else {
				auth = v
			}
			break
		}
	}
	model := "?"
	if i := bytes.Index(body, []byte(`"model":"`)); i >= 0 {
		seg := body[i+9:]
		if j := bytes.IndexByte(seg, '"'); j >= 0 && j < 80 {
			model = string(seg[:j])
		}
	}
	log.Printf("[DARKWAVE-DBG] %s | auth=%s | model=%s | bodyLen=%d | t=%s", url, auth, model, len(body), time.Now().Format("15:04:05"))
}
