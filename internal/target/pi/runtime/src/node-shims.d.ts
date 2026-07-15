declare module "node:child_process" {
  interface Writable { end(value?: string): void }
  interface Readable { on(event: "data", callback: (chunk: Uint8Array) => void): void }
  export interface ChildProcess {
    pid?: number;
    stdin: Writable;
    stdout: Readable | null;
    stderr: Readable | null;
    kill(signal?: string): boolean;
    once(event: "error", callback: (error: Error) => void): void;
    once(event: "close", callback: (code: number | null, signal: string | null) => void): void;
  }
  export function spawn(program: string, args: string[], options: {
    cwd: string; detached: boolean; stdio: ["pipe", "pipe", "pipe"];
  }): ChildProcess;
}
declare module "node:path" {
  export function isAbsolute(path: string): boolean;
  export function resolve(...paths: string[]): string;
  export function relative(from: string, to: string): string;
}
declare const process: {
  pid: number;
  platform: string;
  kill(pid: number, signal: string | number): void;
};
