// Package hookdecision renders a canonical decision-hook process wrapper into vendor protocols.
package hookdecision

import (
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// Protocol identifies a vendor pre-tool decision output protocol.
type Protocol string

const (
	ProtocolClaude  Protocol = "claude"
	ProtocolCodex   Protocol = "codex"
	ProtocolCopilot Protocol = "copilot"
	ProtocolCursor  Protocol = "cursor"
	ProtocolGrok    Protocol = "grok"
)

// UsesDecisionCapability reports whether a hook declares a portable decision cell.
func UsesDecisionCapability(uses []model.CapabilityUse) bool {
	for _, use := range uses {
		if use.Key == "hook.decision.block" || use.Key == "hook.decision.rewrite-input" {
			return true
		}
	}
	return false
}

// WrapPOSIX runs command with canonical Agent Bundler input and translates its decision output.
func WrapPOSIX(command string, protocol Protocol, identity string) string {
	arguments := []string{"node", "-e", decisionScript, "--", string(protocol), identity, "/bin/sh", "-c", command}
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		quoted[index] = shellQuote(argument)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

const decisionScript = `const {spawnSync}=require("node:child_process");
const fs=require("node:fs");
const [protocol,identity,program,...args]=process.argv.slice(1);
const raw=fs.readFileSync(0,"utf8");
let vendor={};
try{vendor=JSON.parse(raw||"{}")}catch{}
const toolName=vendor.tool_name??vendor.toolName??vendor.tool??"";
const toolInput=vendor.tool_input??vendor.toolInput??vendor.input??{};
const canonical=JSON.stringify({event:"pre-tool",hook:identity,piEvent:{toolName,input:toolInput},vendorEvent:vendor})+"\n";
const result=spawnSync(program,args,{input:canonical,encoding:"utf8",env:process.env});
const reason=(result.stderr||"").trim()||"blocked by hook";
if(result.error){process.stderr.write(String(result.error));process.exit(1)}
if(result.status===2){write({decision:"deny",reason});process.exit(0)}
if(result.status!==0){process.stdout.write(result.stdout||"");process.stderr.write(result.stderr||"");process.exit(result.status??1)}
const text=(result.stdout||"").trim();
if(text==="")process.exit(0);
let decision;
try{decision=JSON.parse(text)}catch{process.stderr.write("hook stdout must be one decision object\n");process.exit(1)}
if(decision.decision==="allow")process.exit(0);
if(decision.decision==="deny"){write({decision:"deny",reason:typeof decision.reason==="string"?decision.reason:"blocked by hook"});process.exit(0)}
if(decision.decision==="rewrite-input"&&decision.input&&typeof decision.input==="object"&&!Array.isArray(decision.input)){write({decision:"rewrite",input:decision.input});process.exit(0)}
process.stderr.write("unsupported hook decision\n");process.exit(1);
function write(value){let output;if(protocol==="claude"||protocol==="codex")output={hookSpecificOutput:{hookEventName:"PreToolUse",permissionDecision:value.decision==="deny"?"deny":"allow",...(value.decision==="deny"?{permissionDecisionReason:value.reason}:{updatedInput:value.input})}};else if(protocol==="copilot")output=value.decision==="deny"?{permissionDecision:"deny",permissionDecisionReason:value.reason}:{permissionDecision:"allow",modifiedArgs:value.input};else if(protocol==="cursor")output=value.decision==="deny"?{permission:"deny",user_message:value.reason,agent_message:value.reason}:{permission:"allow",updated_input:value.input};else if(protocol==="grok"&&value.decision==="deny")output={decision:"deny",reason:value.reason};else{process.stderr.write("hook decision is unsupported by target\n");process.exit(1)}process.stdout.write(JSON.stringify(output))}`
