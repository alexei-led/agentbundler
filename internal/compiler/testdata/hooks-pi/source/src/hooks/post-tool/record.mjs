for await (const _chunk of process.stdin) { /* consume input */ }
process.stdout.write(JSON.stringify({ decision: "allow" }));
