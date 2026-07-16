import { describe, expect, test } from "bun:test";
import { spawnSync } from "node:child_process";
import { readFileSync, rmSync } from "node:fs";
import {
  createPiHookRuntime,
  decodeConfig,
  resolvePackageFile,
  runProcess,
  type HookConfigV1,
  type HookDescriptor,
  type PiExtensionAPI,
  type ProcessRunner,
} from "../src/index.js";

type Handler = (event: Record<string, unknown>, context: { signal?: AbortSignal }) => Promise<unknown> | unknown;

class FakePi implements PiExtensionAPI {
  readonly handlers = new Map<string, Handler>();
  on(event: string, handler: Handler): void {
    if (this.handlers.has(event)) throw new Error(`duplicate handler ${event}`);
    this.handlers.set(event, handler);
  }
  async emit(event: string, value: Record<string, unknown> = {}, signal?: AbortSignal): Promise<unknown> {
    const handler = this.handlers.get(event);
    if (handler === undefined) throw new Error(`missing handler ${event}`);
    return await handler(value, signal === undefined ? {} : { signal });
  }
}

const allowRunner: ProcessRunner = async () => ({ exitCode: 0, signal: null, stdout: "", stderr: "" });

function hook(identity: string, event: HookDescriptor["event"], extra: Partial<HookDescriptor> = {}): HookDescriptor {
  return {
    identity,
    event,
    handler: { mode: "exec", program: "ignored", arguments: [] },
    timeoutMilliseconds: 1_000,
    asynchronous: false,
    failurePolicy: "open",
    order: 0,
    ...extra,
  };
}

function config(...hooks: HookDescriptor[]): HookConfigV1 {
  return { version: 1, hooks };
}

describe("schema v1", () => {
  test("decodes the shared Go-shaped fixture and sorts hooks", async () => {
    const fixture = await Bun.file(new URL("../testdata/hooks.v1.json", import.meta.url)).json();
    const decoded = decodeConfig(fixture);
    expect(decoded.hooks.map((value) => value.identity)).toEqual(["hook/session", "hook/pre-tool"]);
  });

  test("decodes the shared shell fixture with an empty argument array", async () => {
    const fixture = await Bun.file(new URL("../testdata/shell-hook.v1.json", import.meta.url)).json();
    const decoded = decodeConfig(fixture);
    expect(decoded.hooks).toHaveLength(1);
    expect(decoded.hooks[0]?.handler).toEqual({ mode: "shell", arguments: [], shellCommand: "printf done" });
  });

  test("matches Go byte ordering for non-ASCII hook identities", async () => {
    const fixture = await Bun.file(new URL("../testdata/hook-order.v1.json", import.meta.url)).json() as {
      config: unknown;
      expectedIdentities: string[];
    };
    const decoded = decodeConfig(fixture.config);
    expect(decoded.hooks.map((value) => value.identity)).toEqual(fixture.expectedIdentities);
  });

  test("preserves repeated exec arguments", () => {
    const repeated = hook("repeat", "session-start", {
      handler: {
        mode: "exec",
        program: "printf",
        arguments: [{ literal: "%s %s" }, { literal: "value" }, { literal: "value" }],
      },
    });
    expect(decodeConfig(config(repeated)).hooks[0]?.handler.arguments).toEqual([
      { literal: "%s %s" }, { literal: "value" }, { literal: "value" },
    ]);
  });

  test("rejects unknown versions, fields, duplicates, and malformed commands", () => {
    expect(() => decodeConfig({ version: 2, hooks: [] })).toThrow("unsupported hook schema version 2");
    expect(() => decodeConfig({ version: 1, hooks: [], extra: true })).toThrow("unknown field");
    expect(() => decodeConfig(config(hook("same", "session-start"), hook("same", "session-end")))).toThrow("duplicate hook identity");
    const invalid = hook("bad", "session-start", { handler: { mode: "shell", shellCommand: "true", arguments: [/* invalid at runtime */] } });
    (invalid.handler.arguments as unknown[]).push("bad");
    expect(() => decodeConfig(config(invalid))).toThrow("arguments must be empty");
  });

  test("rejects closed session-start and post-tool hooks", () => {
    for (const event of ["session-start", "post-tool"] as const) {
      expect(() => decodeConfig(config(hook(`closed-${event}`, event, { failurePolicy: "closed" }))))
        .toThrow(`failurePolicy closed is enforceable only for pre-tool hooks, not ${event}`);
    }
  });
});

describe("generated aggregate fixture", () => {
  test("proves pre-tool decisions, passive dispatch, cancellation, and schema mismatch", async () => {
    const fixture = await Bun.file(new URL("../testdata/hooks-pi.v1.json", import.meta.url)).json();
    const decoded = decodeConfig(fixture);
    expect(decoded.hooks.map((value) => value.identity)).toEqual([
      "hook/deny", "hook/rewrite", "hook/post-tool", "hook/timeout",
    ]);

    const denyPi = new FakePi();
    createPiHookRuntime(denyPi, config(decoded.hooks[0]!), {
      packageRoot: "/tmp/package",
      runner: async () => ({ exitCode: 0, signal: null, stdout: '{"decision":"deny","reason":"blocked bash"}', stderr: "" }),
    });
    expect(await denyPi.emit("tool_call", { toolName: "bash", input: { command: "danger" } })).toEqual({ block: true, reason: "blocked bash" });

    const rewritePi = new FakePi();
    createPiHookRuntime(rewritePi, config(decoded.hooks[1]!), {
      packageRoot: "/tmp/package",
      runner: async () => ({ exitCode: 0, signal: null, stdout: '{"decision":"rewrite-input","input":{"command":"safe"}}', stderr: "" }),
    });
    const input = { command: "danger" };
    expect(await rewritePi.emit("tool_call", { toolName: "bash", input })).toBeUndefined();
    expect(input).toEqual({ command: "safe" });

    let passiveInput: unknown;
    let passiveDone!: () => void;
    const passiveCompleted = new Promise<void>((resolve) => { passiveDone = resolve; });
    const passivePi = new FakePi();
    createPiHookRuntime(passivePi, config(decoded.hooks[2]!), {
      packageRoot: "/tmp/package",
      runner: async (_command, options) => {
        passiveInput = options.input;
        passiveDone();
        return { exitCode: 0, signal: null, stdout: "", stderr: "" };
      },
    });
    await passivePi.emit("tool_result", { toolName: "bash", isError: false, output: "ok" });
    await passiveCompleted;
    expect(passiveInput).toEqual({ event: "post-tool", hook: "hook/post-tool", piEvent: { toolName: "bash", isError: false, output: "ok" } });

    let observedTimeout = 0;
    const timeoutPi = new FakePi();
    createPiHookRuntime(timeoutPi, config(decoded.hooks[3]!), {
      packageRoot: "/tmp/package",
      runner: async (_command, options) => {
        observedTimeout = options.timeoutMilliseconds;
        return { exitCode: 0, signal: null, stdout: "", stderr: "" };
      },
    });
    await timeoutPi.emit("tool_call", { toolName: "bash", input: {} });
    expect(observedTimeout).toBe(25);

    const cancelled = new AbortController();
    const cancellationPi = new FakePi();
    createPiHookRuntime(cancellationPi, config(decoded.hooks[3]!), {
      packageRoot: "/tmp/package",
      runner: async (_command, options) => await new Promise((_resolve, reject) => {
        options.signal?.addEventListener("abort", () => reject(options.signal?.reason), { once: true });
      }),
    });
    const pending = cancellationPi.emit("tool_call", { toolName: "bash", input: {} }, cancelled.signal);
    cancelled.abort(new Error("fixture cancelled"));
    expect(await pending).toBeUndefined();

    expect(() => decodeConfig({ ...(fixture as Record<string, unknown>), version: 2 })).toThrow("unsupported hook schema version 2");
  });
});

describe("Pi event mapping", () => {
  test("maps every supported lifecycle event and separates tool success from failure", async () => {
    const calls: Array<{ hook: string; input: unknown }> = [];
    const runner: ProcessRunner = async (_command, options) => {
      const input = options.input as { hook: string };
      calls.push({ hook: input.hook, input });
      return { exitCode: 0, signal: null, stdout: "", stderr: "" };
    };
    const pi = new FakePi();
    createPiHookRuntime(pi, config(
      hook("start", "session-start"), hook("end", "session-end"), hook("prompt", "prompt-submit"),
      hook("pre", "pre-tool"), hook("post", "post-tool"), hook("failed", "post-tool-failure"),
      hook("stop", "stop"), hook("precompact", "pre-compact"), hook("postcompact", "post-compact"),
    ), { packageRoot: "/tmp/package", runner });

    await pi.emit("session_start", { reason: "startup" });
    await pi.emit("before_agent_start", { prompt: "hello" });
    await pi.emit("tool_call", { toolName: "bash", input: { command: "true" } });
    await pi.emit("tool_result", { toolName: "bash", isError: false });
    await pi.emit("tool_result", { toolName: "bash", isError: true });
    await pi.emit("agent_end", { messages: [] });
    await pi.emit("session_before_compact", { reason: "manual" });
    await pi.emit("session_compact", { reason: "manual" });
    await pi.emit("session_shutdown", { reason: "quit" });

    expect(calls.map((call) => call.hook)).toEqual([
      "start", "prompt", "pre", "post", "failed", "stop", "precompact", "postcompact", "end",
    ]);
    expect((calls[2]?.input as { piEvent: unknown }).piEvent).toEqual({ toolName: "bash", input: { command: "true" } });
  });

  test("evaluates tool categories and deterministic order", async () => {
    const calls: string[] = [];
    const runner: ProcessRunner = async (_command, options) => {
      calls.push((options.input as { hook: string }).hook);
      return { exitCode: 0, signal: null, stdout: "", stderr: "" };
    };
    const pi = new FakePi();
    createPiHookRuntime(pi, config(
      hook("other", "pre-tool", { order: 0, matcher: { tools: ["other"] } }),
      hook("last", "pre-tool", { order: 20, matcher: { tools: ["command"] } }),
      hook("first", "pre-tool", { order: 10, matcher: { tools: ["command"] } }),
    ), { packageRoot: "/tmp/package", runner });
    await pi.emit("tool_call", { toolName: "bash", input: {} });
    expect(calls).toEqual(["first", "last"]);
  });
});

describe("decision translation and failure policy", () => {
  test("allows, denies, and applies only validated input rewrites", async () => {
    const outputs = [
      '{"decision":"allow"}',
      '{"decision":"rewrite-input","input":{"command":"safe","nested":{"ok":true}}}',
      '{"decision":"deny","reason":"blocked"}',
    ];
    const runner: ProcessRunner = async () => ({ exitCode: 0, signal: null, stdout: outputs.shift() ?? "", stderr: "" });
    const pi = new FakePi();
    createPiHookRuntime(pi, config(hook("a", "pre-tool", { order: 1 }), hook("b", "pre-tool", { order: 2 }), hook("c", "pre-tool", { order: 3 })), {
      packageRoot: "/tmp/package", runner,
    });
    const input = { command: "danger" };
    expect(await pi.emit("tool_call", { toolName: "bash", input })).toEqual({ block: true, reason: "blocked" });
    expect(input).toEqual({ command: "safe", nested: { ok: true } });
  });

  test("never mutates input for malformed or unsafe rewrite output", async () => {
    for (const stdout of [
      '{"decision":"rewrite-input","input":[]}',
      '{"decision":"rewrite-input","input":{"constructor":{}}}',
      '{not json}',
    ]) {
      const errors: Error[] = [];
      const pi = new FakePi();
      createPiHookRuntime(pi, config(hook("rewrite", "pre-tool", { failurePolicy: "open" })), {
        packageRoot: "/tmp/package",
        runner: async () => ({ exitCode: 0, signal: null, stdout, stderr: "" }),
        onError: (error) => errors.push(error),
      });
      const input = { command: "unchanged" };
      expect(await pi.emit("tool_call", { toolName: "bash", input })).toBeUndefined();
      expect(input).toEqual({ command: "unchanged" });
      expect(errors).toHaveLength(1);
    }
  });

  test("fails open or closed on pre-tool command failure and reports passive decisions", async () => {
    const failing: ProcessRunner = async () => ({ exitCode: 7, signal: null, stdout: "", stderr: "boom" });
    const openPi = new FakePi();
    createPiHookRuntime(openPi, config(hook("open", "pre-tool", { failurePolicy: "open" })), { packageRoot: "/tmp/package", runner: failing });
    expect(await openPi.emit("tool_call", { toolName: "bash", input: {} })).toBeUndefined();

    const closedPi = new FakePi();
    createPiHookRuntime(closedPi, config(hook("closed", "pre-tool", { failurePolicy: "closed" })), { packageRoot: "/tmp/package", runner: failing });
    expect(await closedPi.emit("tool_call", { toolName: "bash", input: {} })).toEqual({ block: true, reason: "hook exited with code 7: boom" });

    const errors: Error[] = [];
    const passivePi = new FakePi();
    createPiHookRuntime(passivePi, config(hook("passive", "session-start")), {
      packageRoot: "/tmp/package",
      runner: async () => ({ exitCode: 0, signal: null, stdout: '{"decision":"deny"}', stderr: "" }),
      onError: (error) => errors.push(error),
    });
    expect(await passivePi.emit("session_start")).toBeUndefined();
    expect(errors.map((error) => error.message)).toEqual(["deny is only valid for pre-tool hooks"]);
  });
});

describe("process dispatch", () => {
  test("resolves package files and rejects traversal", () => {
    expect(resolvePackageFile("/tmp/package", "hooks/run.js")).toBe("/tmp/package/hooks/run.js");
    for (const path of ["../escape", "/absolute", "hooks/../escape", "hooks\\escape", "./escape", ""]) {
      expect(() => resolvePackageFile("/tmp/package", path)).toThrow();
    }
  });

  test("dispatches exec and shell commands with JSON stdin", async () => {
    const exec = await runProcess({
      mode: "exec", program: "node", arguments: [
        { literal: "-e" },
        { literal: "let s='';process.stdin.on('data',c=>s+=c);process.stdin.on('end',()=>process.stdout.write(JSON.stringify({decision:JSON.parse(s).ok?'allow':'deny'})))" },
      ],
    }, { packageRoot: "/tmp", input: { ok: true }, timeoutMilliseconds: 2_000 });
    expect(JSON.parse(exec.stdout)).toEqual({ decision: "allow" });
    expect(exec.exitCode).toBe(0);

    const shell = await runProcess({ mode: "shell", shellCommand: "printf '{\"decision\":\"allow\"}'", arguments: [] }, {
      packageRoot: "/tmp", input: {}, timeoutMilliseconds: 2_000,
    });
    expect(JSON.parse(shell.stdout)).toEqual({ decision: "allow" });
  });

  test("does not inherit parent secrets while preserving command lookup", async () => {
    const sentinel = "AGENTBUNDLER_PI_HOOK_SECRET_SENTINEL";
    const previous = process.env[sentinel];
    process.env[sentinel] = "must-not-reach-hook";
    try {
      const result = await runProcess({
        mode: "exec",
        program: "node",
        arguments: [
          { literal: "-e" },
          { literal: `process.stdout.write(JSON.stringify({hasPath:typeof process.env.PATH==="string"&&process.env.PATH.length>0,hasSecret:Object.prototype.hasOwnProperty.call(process.env,${JSON.stringify(sentinel)})}))` },
        ],
      }, { packageRoot: "/tmp", input: {}, timeoutMilliseconds: 2_000 });
      expect(result.exitCode).toBe(0);
      expect(JSON.parse(result.stdout)).toEqual({ hasPath: true, hasSecret: false });
    } finally {
      if (previous === undefined) delete process.env[sentinel];
      else process.env[sentinel] = previous;
    }
  });

  test("contains broken-pipe errors when a hook closes stdin", () => {
    const runtimeURL = new URL("../src/process.ts", import.meta.url).href;
    const script = `
      import { runProcess } from ${JSON.stringify(runtimeURL)};
      try {
        await runProcess({mode:"exec",program:"node",arguments:[{literal:"-e"},{literal:"require('node:fs').closeSync(0);setInterval(()=>{},1000)"}]},{packageRoot:"/tmp",input:{payload:"x".repeat(8*1024*1024)},timeoutMilliseconds:2000});
        console.error("runProcess unexpectedly resolved");
        process.exitCode = 1;
      } catch (error) {
        if (!(error instanceof Error) || !error.message.includes("EPIPE")) {
          console.error(error);
          process.exitCode = 1;
        }
      }
    `;
    const host = spawnSync("node", ["--input-type=module", "--eval", script], { encoding: "utf8", timeout: 5_000 });
    if (host.status !== 0) throw new Error(`Node host exited ${String(host.status)}:\n${host.stderr}`);
  });

  test("bounds stdout and stderr independently", async () => {
    await expect(runProcess({ mode: "exec", program: "node", arguments: [{ literal: "-e" }, { literal: "process.stdout.write('x'.repeat(1000))" }] }, {
      packageRoot: "/tmp", input: {}, timeoutMilliseconds: 2_000, outputLimitBytes: 100,
    })).rejects.toThrow("stdout exceeded 100 byte limit");
    await expect(runProcess({ mode: "exec", program: "node", arguments: [{ literal: "-e" }, { literal: "process.stderr.write('x'.repeat(1000))" }] }, {
      packageRoot: "/tmp", input: {}, timeoutMilliseconds: 2_000, outputLimitBytes: 100,
    })).rejects.toThrow("stderr exceeded 100 byte limit");
  });

  test("terminates on timeout and cancellation", async () => {
    const command = { mode: "exec" as const, program: "node", arguments: [{ literal: "-e" }, { literal: "setInterval(()=>{},1000)" }] };
    await expect(runProcess(command, { packageRoot: "/tmp", input: {}, timeoutMilliseconds: 30 })).rejects.toThrow("timed out");
    const controller = new AbortController();
    const pending = runProcess(command, { packageRoot: "/tmp", input: {}, timeoutMilliseconds: 2_000, signal: controller.signal });
    setTimeout(() => controller.abort("cancelled by test"), 20);
    await expect(pending).rejects.toMatchObject({ name: "AbortError", message: "cancelled by test" });
  });
});

describe("shutdown", () => {
  test("cancels asynchronous work, prevents new work, and is idempotent", async () => {
    let starts = 0;
    let aborts = 0;
    const runner: ProcessRunner = async (_command, options) => {
      starts++;
      return await new Promise((_resolve, reject) => {
        options.signal?.addEventListener("abort", () => {
          aborts++;
          reject(new Error("aborted"));
        }, { once: true });
      });
    };
    const pi = new FakePi();
    const runtime = createPiHookRuntime(pi, config(hook("async", "post-tool", { asynchronous: true })), {
      packageRoot: "/tmp/package", runner,
    });
    await pi.emit("tool_result", { toolName: "bash", isError: false });
    const first = runtime.shutdown();
    const second = runtime.shutdown();
    expect(first).toBe(second);
    await first;
    expect(starts).toBe(1);
    expect(aborts).toBe(1);
    await pi.emit("tool_result", { toolName: "bash", isError: false });
    await pi.emit("session_shutdown");
    expect(starts).toBe(1);
    expect(aborts).toBe(1);
  });

  test("waits for an asynchronous child process to exit", async () => {
    const pidFile = `/tmp/agentbundler-pi-runtime-${String(process.pid)}-${String(Date.now())}.pid`;
    const command = hook("async", "post-tool", {
      asynchronous: true,
      handler: {
        mode: "exec",
        program: "node",
        arguments: [
          { literal: "-e" },
          { literal: "const fs=require('node:fs');process.on('SIGTERM',()=>{});fs.writeFileSync(process.argv[1],'');setTimeout(()=>fs.writeFileSync(process.argv[1],String(process.pid)),20);setInterval(()=>{},1000)" },
          { literal: pidFile },
        ],
      },
    });
    const pi = new FakePi();
    const runtime = createPiHookRuntime(pi, config(command), { packageRoot: "/tmp" });

    try {
      await pi.emit("tool_result", { toolName: "bash", isError: false });
      let childPID = 0;
      for (let attempt = 0; attempt < 100 && childPID === 0; attempt++) {
        try {
          const candidate = Number.parseInt(readFileSync(pidFile, "utf8"), 10);
          if (Number.isSafeInteger(candidate) && candidate > 0) childPID = candidate;
        } catch { /* The child has not created the PID file yet. */ }
        if (childPID === 0) await new Promise((resolve) => setTimeout(resolve, 10));
      }
      if (childPID === 0) throw new Error("child process did not write its PID");

      await runtime.shutdown();
      let running = true;
      try { process.kill(childPID, 0); } catch { running = false; }
      expect(running).toBe(false);
    } finally {
      await runtime.shutdown();
      rmSync(pidFile, { force: true });
    }
  });

  test("waits for asynchronous session-end work before resolving", async () => {
    let active = 0;
    let starts = 0;
    let aborts = 0;
    let started!: () => void;
    let finish!: () => void;
    const didStart = new Promise<void>((resolve) => { started = resolve; });
    const canFinish = new Promise<void>((resolve) => { finish = resolve; });
    const runner: ProcessRunner = async (_command, options) => {
      starts++;
      active++;
      started();
      try {
        await new Promise<void>((resolve, reject) => {
          canFinish.then(resolve, reject);
          options.signal?.addEventListener("abort", () => {
            aborts++;
            reject(new Error("aborted"));
          }, { once: true });
        });
        return { exitCode: 0, signal: null, stdout: "", stderr: "" };
      } finally {
        active--;
      }
    };
    const pi = new FakePi();
    const runtime = createPiHookRuntime(pi, config(hook("end", "session-end", { asynchronous: true })), {
      packageRoot: "/tmp/package", runner,
    });

    const pending = pi.emit("session_shutdown", { reason: "quit" });
    await didStart;
    expect(starts).toBe(1);
    expect(aborts).toBe(0);
    expect(active).toBe(1);
    finish();
    await pending;
    expect(aborts).toBe(0);
    expect(active).toBe(0);
    await runtime.shutdown();
    await pi.emit("session_shutdown", { reason: "again" });
    expect(starts).toBe(1);
    expect(active).toBe(0);
  });

  test("cancels asynchronous session-end work on explicit runtime shutdown", async () => {
    let aborts = 0;
    let started!: () => void;
    const didStart = new Promise<void>((resolve) => { started = resolve; });
    const runner: ProcessRunner = async (_command, options) => {
      started();
      return await new Promise((_resolve, reject) => {
        options.signal?.addEventListener("abort", () => {
          aborts++;
          reject(new Error("aborted"));
        }, { once: true });
      });
    };
    const pi = new FakePi();
    const runtime = createPiHookRuntime(pi, config(hook("end", "session-end", { asynchronous: true })), {
      packageRoot: "/tmp/package", runner,
    });

    const pending = pi.emit("session_shutdown", { reason: "quit" });
    await didStart;
    const first = runtime.shutdown();
    const second = runtime.shutdown();
    expect(first).toBe(second);
    await Promise.all([pending, first]);
    expect(aborts).toBe(1);
  });
});
