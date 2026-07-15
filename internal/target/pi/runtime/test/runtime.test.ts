import { describe, expect, test } from "bun:test";
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

  test("rejects unknown versions, fields, duplicates, and malformed commands", () => {
    expect(() => decodeConfig({ version: 2, hooks: [] })).toThrow("unsupported hook schema version 2");
    expect(() => decodeConfig({ version: 1, hooks: [], extra: true })).toThrow("unknown field");
    expect(() => decodeConfig(config(hook("same", "session-start"), hook("same", "session-end")))).toThrow("duplicate hook identity");
    const invalid = hook("bad", "session-start", { handler: { mode: "shell", shellCommand: "true", arguments: [/* invalid at runtime */] } });
    (invalid.handler.arguments as unknown[]).push("bad");
    expect(() => decodeConfig(config(invalid))).toThrow("arguments must be empty");
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

  test("fails open or closed on command failure and malformed passive decisions", async () => {
    const failing: ProcessRunner = async () => ({ exitCode: 7, signal: null, stdout: "", stderr: "boom" });
    const openPi = new FakePi();
    createPiHookRuntime(openPi, config(hook("open", "pre-tool", { failurePolicy: "open" })), { packageRoot: "/tmp/package", runner: failing });
    expect(await openPi.emit("tool_call", { toolName: "bash", input: {} })).toBeUndefined();

    const closedPi = new FakePi();
    createPiHookRuntime(closedPi, config(hook("closed", "pre-tool", { failurePolicy: "closed" })), { packageRoot: "/tmp/package", runner: failing });
    expect(await closedPi.emit("tool_call", { toolName: "bash", input: {} })).toEqual({ block: true, reason: "hook exited with code 7: boom" });

    const passivePi = new FakePi();
    createPiHookRuntime(passivePi, config(hook("passive", "session-start", { failurePolicy: "closed" })), {
      packageRoot: "/tmp/package",
      runner: async () => ({ exitCode: 0, signal: null, stdout: '{"decision":"deny"}', stderr: "" }),
    });
    expect(passivePi.emit("session_start")).rejects.toThrow("only valid for pre-tool");
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
  test("cancels asynchronous work and is idempotent", async () => {
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
    await pi.emit("session_shutdown");
    expect(aborts).toBe(1);
  });
});
