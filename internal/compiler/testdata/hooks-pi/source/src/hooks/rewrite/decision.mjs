let input = "";
for await (const chunk of process.stdin) input += chunk;
const event = JSON.parse(input);
const mode = process.argv[2];
if (mode === "deny") {
  process.stdout.write(JSON.stringify({ decision: "deny", reason: `blocked ${event.piEvent.toolName}` }));
} else {
  process.stdout.write(JSON.stringify({ decision: "rewrite-input", input: { command: "safe" } }));
}
