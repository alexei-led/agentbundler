declare module "node:fs" {
  export function readFileSync(path: string, encoding: "utf8"): string;
  export function rmSync(path: string, options?: { force?: boolean }): void;
}

declare module "bun:test" {
  export function describe(name: string, body: () => void): void;
  export function test(name: string, body: () => unknown | Promise<unknown>): void;
  export function expect(value: unknown): {
    toBe(expected: unknown): void;
    toBeUndefined(): void;
    toEqual(expected: unknown): void;
    toHaveLength(expected: number): void;
    toThrow(expected?: string): void;
    rejects: {
      toThrow(expected?: string): Promise<void>;
      toMatchObject(expected: object): Promise<void>;
    };
  };
}
declare const Bun: {
  file(path: URL): { json(): Promise<unknown> };
};
