for await (const _chunk of process.stdin) { /* consume input */ }
await new Promise((resolve) => setTimeout(resolve, 1000));
