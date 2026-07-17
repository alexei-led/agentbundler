# Vendored Pi subagent runtime

`pi-subagents-runtime-0.34.0.tgz` contains the production npm dependency tree for
`pi-subagents@0.34.0`, without optional or development dependencies. Agent Bundler
unpacks regular files into generated Pi package `node_modules/` trees and registers
`pi-subagents` directly in `pi.extensions`.

Source packages are MIT licensed. The archive was produced with:

```sh
npm install --ignore-scripts --package-lock=false --omit=dev --omit=optional \
  --no-audit --no-fund pi-subagents@0.34.0
tar -czf pi-subagents-runtime-0.34.0.tgz node_modules
```

SHA-256: `aa24f310b4553536328fe861d17860fd40d8709bae9fa4a06721342584cb637b`
