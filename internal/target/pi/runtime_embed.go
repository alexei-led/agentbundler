package pi

import "embed"

const embeddedRuntimeRoot = "extensions/_agentbundler-hooks"

var embeddedRuntimePaths = []string{
	"runtime/src/index.ts",
	"runtime/src/matcher.ts",
	"runtime/src/process.ts",
	"runtime/src/runtime.ts",
	"runtime/src/schema.ts",
}

//go:embed runtime/src/index.ts runtime/src/matcher.ts runtime/src/process.ts runtime/src/runtime.ts runtime/src/schema.ts
var embeddedRuntime embed.FS

type runtimeFile struct {
	name       string
	bytes      []byte
	executable bool
}

func runtimeFiles() ([]runtimeFile, error) {
	files := make([]runtimeFile, 0, len(embeddedRuntimePaths))
	for _, path := range embeddedRuntimePaths {
		data, err := embeddedRuntime.ReadFile(path)
		if err != nil {
			return nil, err
		}
		files = append(files, runtimeFile{name: path[len("runtime/src/"):], bytes: data})
	}
	return files, nil
}
