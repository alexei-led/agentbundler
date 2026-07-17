package hookdecision

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestWrapPOSIXTranslatesCanonicalDecisions(t *testing.T) {
	for _, test := range []struct {
		protocol Protocol
		decision string
		want     string
	}{
		{ProtocolClaude, `{"decision":"deny","reason":"no"}`, `"permissionDecision":"deny"`},
		{ProtocolCodex, `{"decision":"rewrite-input","input":{"command":"safe"}}`, `"updatedInput":{"command":"safe"}`},
		{ProtocolCopilot, `{"decision":"deny","reason":"no"}`, `"permissionDecision":"deny"`},
		{ProtocolCursor, `{"decision":"rewrite-input","input":{"command":"safe"}}`, `"updated_input":{"command":"safe"}`},
		{ProtocolGrok, `{"decision":"deny","reason":"no"}`, `"decision":"deny"`},
	} {
		t.Run(string(test.protocol)+test.want, func(t *testing.T) {
			hook := `node -e 'let s="";process.stdin.on("data",c=>s+=c);process.stdin.on("end",()=>{const x=JSON.parse(s);if(x.event!=="pre-tool"||x.hook!=="hook/guard"||x.piEvent.input.command!=="danger")process.exit(9);process.stdout.write(process.argv[1])})' ` + shellQuote(test.decision)
			command := exec.Command("/bin/sh", "-c", WrapPOSIX(hook, test.protocol, "hook/guard"))
			command.Stdin = strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"danger"}}`)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("wrapper failed: %v: %s", err, output)
			}
			if !strings.Contains(string(output), test.want) {
				t.Fatalf("output = %s, want %s", output, test.want)
			}
			var value any
			if err := json.Unmarshal(output, &value); err != nil {
				t.Fatalf("invalid vendor JSON: %v", err)
			}
		})
	}
}

func TestWrapPOSIXDistinguishesIntentionalDenyFromFailure(t *testing.T) {
	vendorInput := `{"padding":"` + strings.Repeat("x", 1<<20) + `"}`
	deny := exec.Command("/bin/sh", "-c", WrapPOSIX(`printf intentional >&2; exit 2`, ProtocolCopilot, "hook/guard"))
	deny.Stdin = strings.NewReader(vendorInput)
	output, err := deny.CombinedOutput()
	if err != nil || !strings.Contains(string(output), `"permissionDecision":"deny"`) {
		t.Fatalf("deny = (%v, %s)", err, output)
	}
	failure := exec.Command("/bin/sh", "-c", WrapPOSIX(`printf broken >&2; exit 7`, ProtocolCopilot, "hook/guard"))
	failure.Stdin = strings.NewReader(vendorInput)
	output, err = failure.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 || !strings.Contains(string(output), "broken") {
		t.Fatalf("failure = (%v, %s)", err, output)
	}
}
