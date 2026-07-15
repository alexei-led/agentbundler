import json
import sys
import time

mode = sys.argv[1]
if mode == "sleep":
    time.sleep(5)
    raise SystemExit(0)
if mode == "flood":
    sys.stdout.write("o" * 4096)
    sys.stderr.write("e" * 4096)
    raise SystemExit(0)
if mode == "exit":
    sys.stderr.write("fixture exit\n")
    raise SystemExit(23)

try:
    request = json.load(sys.stdin)
    command = request["tool_input"]["command"]
    if request["hook_event_name"] != "PreToolUse" or request["tool_name"] != "Bash" or not isinstance(command, str):
        raise ValueError("unexpected hook input")
except (KeyError, TypeError, ValueError, json.JSONDecodeError) as error:
    sys.stderr.write(f"invalid hook input: {error}\n")
    raise SystemExit(64)

output = {"hookSpecificOutput": {"hookEventName": "PreToolUse"}}
decision = output["hookSpecificOutput"]
if mode == "allow":
    decision["permissionDecision"] = "allow"
elif mode == "deny":
    decision["permissionDecision"] = "deny"
    decision["permissionDecisionReason"] = f"fixture denied: {command}"
elif mode == "rewrite":
    decision["permissionDecision"] = "allow"
    decision["updatedInput"] = {"command": f"{command} --safe"}
else:
    sys.stderr.write(f"unknown mode: {mode}\n")
    raise SystemExit(64)

json.dump(output, sys.stdout, separators=(",", ":"))
sys.stdout.write("\n")
