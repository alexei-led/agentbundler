// Package frontmatter parses the YAML frontmatter used by Agent Skills files.
package frontmatter

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse returns the frontmatter object and the exact body following it.
//
// The delimiters must occupy complete lines. YAML aliases, custom tags,
// timestamps, and duplicate mapping keys are rejected so the result remains a
// deterministic JSON-compatible value for every target renderer.
func Parse(data []byte) (map[string]any, string, error) {
	text := string(data)
	lines := strings.SplitAfter(text, "\n")
	if len(lines) == 0 || trimLineEnding(lines[0]) != "---" {
		return map[string]any{}, text, nil
	}
	for index := 1; index < len(lines); index++ {
		if trimLineEnding(lines[index]) != "---" {
			continue
		}
		value, err := decode(lines[1:index])
		if err != nil {
			return nil, "", fmt.Errorf("frontmatter: %w", err)
		}
		return value, strings.Join(lines[index+1:], ""), nil
	}
	return nil, "", fmt.Errorf("frontmatter opening delimiter has no closing delimiter")
}

func decode(lines []string) (map[string]any, error) {
	var document yaml.Node
	decoder := yaml.NewDecoder(strings.NewReader(strings.Join(lines, "")))
	if err := decoder.Decode(&document); err != nil {
		if err == io.EOF {
			return map[string]any{}, nil
		}
		return nil, err
	}
	value, err := convert(&document)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]any{}, nil
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("top-level value must be a mapping")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple YAML documents are not supported")
		}
		return nil, err
	}
	return result, nil
}

func convert(node *yaml.Node) (any, error) {
	if node == nil {
		return nil, nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		return convert(node.Content[0])
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return nil, fmt.Errorf("mapping has an odd number of nodes")
		}
		result := make(map[string]any, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return nil, fmt.Errorf("mapping keys must be strings")
			}
			if _, exists := result[key.Value]; exists {
				return nil, fmt.Errorf("duplicate YAML key %q", key.Value)
			}
			value, err := convert(node.Content[index+1])
			if err != nil {
				return nil, err
			}
			result[key.Value] = value
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := convert(child)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		return result, nil
	case yaml.ScalarNode:
		return scalar(node)
	case yaml.AliasNode:
		return nil, fmt.Errorf("YAML aliases are not supported")
	default:
		return nil, fmt.Errorf("unsupported YAML node kind %d", node.Kind)
	}
}

func scalar(node *yaml.Node) (any, error) {
	switch node.Tag {
	case "!!str":
		return node.Value, nil
	case "!!null":
		return nil, nil
	case "!!bool":
		value, err := strconv.ParseBool(strings.ToLower(node.Value))
		if err != nil {
			return nil, fmt.Errorf("invalid boolean %q", node.Value)
		}
		return value, nil
	case "!!int":
		value := strings.ReplaceAll(node.Value, "_", "")
		if strings.HasPrefix(strings.ToLower(value), "0x") {
			parsed, err := strconv.ParseUint(value[2:], 16, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid integer %q", node.Value)
			}
			return parsed, nil
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", node.Value)
		}
		return parsed, nil
	case "!!float":
		value := strings.ReplaceAll(node.Value, "_", "")
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return nil, fmt.Errorf("invalid finite number %q", node.Value)
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("YAML scalar tag %q is not supported", node.Tag)
	}
}

func trimLineEnding(value string) string {
	return strings.TrimSuffix(strings.TrimSuffix(value, "\n"), "\r")
}
