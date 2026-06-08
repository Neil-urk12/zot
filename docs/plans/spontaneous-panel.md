# Plan: Spontaneous `open_panel` + Human-in-the-Loop Tool Gate

## Problem

`open_panel` is only valid as the `action` of a `command_response`, which must
be a direct reply to a `command_invoked` frame. A panel can therefore only open
when the user types a slash command. There is no way for an extension to open a
panel in response to a tool call — making human-in-the-loop approval gates,
secret collection, and freeform user-input patterns impossible without awkward
workarounds (`/approve` slash commands, `threading.Event` polling, etc.).

---

## What we are building

**Part 1 (required):** A new top-level `open_panel` frame an extension can send
at any time, uncoupled from any command invocation.

**Part 2 (falls out of Part 1 for free):** A blocking tool result pattern where
a tool goroutine opens a panel, waits on a Go channel for the user's response,
and only then returns a `tool_result`. No new wire frames needed — standard
concurrency on the extension side.

**Part 3 (separate, required for intercepting built-in tools):** Raise or
remove the 5-second `event_intercept_response` timeout so that human-interactive
intercept handlers don't time out before the user can respond.

---

## Implementation plan

### 1. `packages/agent/extproto/extproto.go`

Add one new struct (≈5 lines):

```go
// OpenPanelFromExt is a spontaneous one-way frame an extension can send
// at any time to open an interactive panel without a prior command invocation.
type OpenPanelFromExt struct {
    Type  string    `json:"type"`  // "open_panel"
    Panel PanelSpec `json:"panel"`
}
```

### 2. `packages/agent/extensions/manager.go`

Add one case to the `readLoop` switch (≈5 lines):

```go
case "open_panel":
    var op extproto.OpenPanelFromExt
    if err := json.Unmarshal(line, &op); err == nil {
        m.hooks.OpenPanel(ext.Manifest.Name, op.Panel)
    }
```

`HostHooks.OpenPanel` already exists. `interactive.go` requires **zero changes**.

### 3. `packages/agent/ext/ext.go`

Add one method on `Extension` (≈5 lines):

```go
// OpenPanel opens an interactive panel spontaneously from extension code
// without requiring a slash command. Safe to call from a tool handler goroutine.
func (e *Extension) OpenPanel(id, title string, lines []string, footer string) {
    _ = e.send(extproto.OpenPanelFromExt{
        Type:  "open_panel",
        Panel: extproto.PanelSpec{ID: id, Title: title, Lines: lines, Footer: footer},
    })
}
```

### 4. Part 3 — intercept timeout (built-in tool gating)

Locate the hardcoded 5-second deadline in `packages/agent/extensions/manager.go`
(the `InterceptEvent` / `pendingIntercept` timeout). Two options, pick one:

- **Option A (preferred):** Add `intercept_timeout_sec` to `Manifest` in
  `manager.go` and honour it when building the intercept deadline. Zero/absent
  means keep 5s default.
- **Option B:** Add a `timeout_ms` field to `EventInterceptFromHost` in
  `extproto.go` that the host sets per-call when the target extension declares
  the `panels` capability.

### 5. `docs/extensions.md`

- Add `open_panel` to the Extension → host frame table.
- Add a short prose section under Phase 4 describing the spontaneous form,
  the blocking tool pattern, and the secret-collection pattern.
- Note the concurrent-panel limitation.

### 6. `examples/extensions/` (new example)

Add `examples/extensions/approval/` — a minimal extension demonstrating:
- An LLM-callable tool that opens an approval panel before proceeding.
- A secret-collection variant (masked password input).

No changes to any other file.

---

## Example usages

### Simple approve / deny

```go
e.Tool("risky_op", "Performs a risky operation.", schema, func(args json.RawMessage) ext.ToolResult {
    result := make(chan bool, 1)
    pid := "approve-" + randomID()

    e.OnPanelKey(pid, func(key, text string) {
        switch {
        case key == "rune" && text == "y":
            e.ClosePanel(pid); result <- true
        case key == "rune" && text == "n", key == "esc":
            e.ClosePanel(pid); result <- false
        }
    }, func() { result <- false })

    e.OpenPanel(pid, "Approve?",
        []string{"Agent wants to run: " + summary(args), "", "  y  approve", "  n  deny"},
        "y approve  n deny  esc cancel")

    if !<-result {
        return ext.TextErrorResult("user denied")
    }
    return doWork(args)
})
```

### Secret / credential collection

The secret is used directly inside the extension and never written to any JSON
frame or the transcript. The model receives only a success/failure status.

```go
e.Tool("fetch_authenticated", "Fetch a URL that requires a password.", schema,
    func(args json.RawMessage) ext.ToolResult {
        var in struct{ URL string `json:"url"` }
        json.Unmarshal(args, &in)

        type result struct{ secret string; ok bool }
        ch := make(chan result, 1)
        pid := "secret-" + randomID()
        var mu sync.Mutex
        var input string

        render := func() {
            mu.Lock(); masked := strings.Repeat("●", len([]rune(input))); mu.Unlock()
            e.RenderPanel(pid, "Password required",
                []string{"  URL: " + in.URL, "", "  Password: " + masked + "▌"},
                "enter confirm  esc cancel")
        }
        e.OnPanelKey(pid, func(key, text string) {
            mu.Lock()
            switch key {
            case "rune":      input += text
            case "backspace": if len(input) > 0 { r := []rune(input); input = string(r[:len(r)-1]) }
            case "enter":     secret := input; mu.Unlock(); e.ClosePanel(pid); ch <- result{secret, true}; return
            case "esc":       mu.Unlock(); e.ClosePanel(pid); ch <- result{}; return
            }
            mu.Unlock(); render()
        }, func() { ch <- result{} })

        e.OpenPanel(pid, "Password required",
            []string{"  URL: " + in.URL, "", "  Password: ▌"},
            "enter confirm  esc cancel")

        r := <-ch
        if !r.ok { return ext.TextErrorResult("cancelled") }
        return doFetch(in.URL, r.secret) // secret never leaves the extension process
    })
```

### Freeform text / override justification

Same pattern as secret collection but without masking and with the result
injected into the tool's output rather than used as a credential — e.g. a
human-written review comment, an override reason for a blocked action, or a
value the model should not control.

### Intercepting a built-in tool (requires Part 3)

```go
e.InterceptToolCallX(func(tool string, args json.RawMessage) ext.ToolCallDecision {
    if tool != "bash" { return ext.ToolCallDecision{} }
    ch := make(chan ext.ToolCallDecision, 1)
    pid := "guard-" + randomID()
    e.OnPanelKey(pid, func(key, text string) {
        if key == "rune" && text == "y" { e.ClosePanel(pid); ch <- ext.ToolCallDecision{} }
        if key == "rune" && text == "n" || key == "esc" {
            e.ClosePanel(pid); ch <- ext.ToolCallDecision{Block: true, Reason: "user denied"}
        }
    }, func() { ch <- ext.ToolCallDecision{Block: true, Reason: "panel closed"} })
    e.OpenPanel(pid, "Approve bash?", renderBashLines(args), "y approve  n deny")
    return <-ch // blocks intercept goroutine — requires Part 3 timeout increase
})
```

---

## Out-of-scope risks

**Intercept timeout (5s) blocks Part 3.**
The `event_intercept_response` timeout is hardcoded at 5 seconds. Human
interaction always exceeds this. Part 3 must be resolved before
`InterceptToolCallX` can be used as an approval gate for built-in tools.
Extension-registered tools are unaffected (no timeout on the tool goroutine).

**Only one panel open at a time.**
`extPanelDialog` is a single slot. A second spontaneous `open_panel` while
another panel is active will replace it. Extensions that may receive concurrent
tool calls must serialise approvals internally. Multi-panel stacking is a
separate future concern.

**Goroutine leak if panel is abandoned.**
If the user quits zot or the process is interrupted while a tool goroutine is
blocked on a channel, that goroutine leaks until process exit. Mitigation:
extension authors should `select` on a context cancellation channel alongside
the result channel. The SDK's `ToolHandler` signature does not currently expose
a context — passing one through is a separate improvement.

**No panel scrolling / wrapping.**
Panel lines are plain strings (ANSI colour permitted). There is no built-in word
wrap or scroll. Long prompts must be pre-wrapped by the extension. Adequate for
approve/deny and credential collection; insufficient for displaying large
structured content.

**Panel ID collisions under concurrent tool calls.**
If two tool calls for the same tool arrive concurrently, naive panel ID
generation could produce the same ID and stomp state. Use the tool-call ID (from
`ToolCallFromHost.ID`) as a suffix: `"approve-" + toolCallID`.

---

## Files changed

| File | Change |
|---|---|
| `packages/agent/extproto/extproto.go` | Add `OpenPanelFromExt` struct |
| `packages/agent/extensions/manager.go` | Add `case "open_panel":` in `readLoop`; Part 3: raise intercept timeout |
| `packages/agent/ext/ext.go` | Add `Extension.OpenPanel(...)` method |
| `docs/extensions.md` | Document spontaneous frame, blocking pattern, limitations |
| `examples/extensions/approval/` | New example extension (approval + secret collection) |
