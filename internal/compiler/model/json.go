package model

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// DecodeSourceManifestJSON decodes and validates a source manifest with strict JSON object handling.
func DecodeSourceManifestJSON(data []byte) (SourceManifest, []Diagnostic) {
	var manifest SourceManifest
	if err := DecodeStrictJSONObject(data, &manifest); err != nil {
		return SourceManifest{}, []Diagnostic{invalidManifestDiagnostic(err.Error())}
	}
	return manifest, ValidateSourceManifest(manifest)
}

// DecodeHookDescriptorJSON decodes the author-owned fields of one portable hook.
// Identity and location are assigned by the source importer and are not accepted
// from author JSON.
func DecodeHookDescriptorJSON(data []byte, identity AssetID, location SourceLocation) (HookDescriptor, error) {
	var raw struct {
		Event               *HookEvent         `json:"event"`
		Matcher             *HookMatcher       `json:"matcher"`
		Handler             *HookCommand       `json:"handler"`
		TimeoutMilliseconds *int               `json:"timeoutMilliseconds"`
		Asynchronous        *bool              `json:"asynchronous"`
		FailurePolicy       *HookFailurePolicy `json:"failurePolicy"`
		Environment         []string           `json:"environment"`
		Order               *int               `json:"order"`
	}
	if err := DecodeStrictJSONObject(data, &raw); err != nil {
		return HookDescriptor{}, err
	}
	if raw.Event == nil {
		return HookDescriptor{}, fmt.Errorf("event is required")
	}
	if raw.Handler == nil {
		return HookDescriptor{}, fmt.Errorf("handler is required")
	}
	if raw.TimeoutMilliseconds == nil {
		return HookDescriptor{}, fmt.Errorf("timeoutMilliseconds is required")
	}
	if raw.Asynchronous == nil {
		return HookDescriptor{}, fmt.Errorf("asynchronous is required")
	}
	if raw.FailurePolicy == nil {
		return HookDescriptor{}, fmt.Errorf("failurePolicy is required")
	}
	if raw.Order == nil {
		return HookDescriptor{}, fmt.Errorf("order is required")
	}
	return HookDescriptor{
		Identity:            identity,
		Location:            location,
		Event:               *raw.Event,
		Matcher:             raw.Matcher,
		Handler:             *raw.Handler,
		TimeoutMilliseconds: *raw.TimeoutMilliseconds,
		Asynchronous:        *raw.Asynchronous,
		FailurePolicy:       *raw.FailurePolicy,
		Environment:         append([]string(nil), raw.Environment...),
		Order:               *raw.Order,
	}, nil
}

// DecodeOverlayFileContentJSON decodes one overlay file value. A string is
// shorthand for non-executable UTF-8 content. Object values contain exactly one
// of text or base64 and may set executable.
func DecodeOverlayFileContentJSON(data []byte, location SourceLocation) (FileContent, error) {
	var text string
	if err := DecodeStrictJSON(data, &text); err == nil && !bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return FileContent{Bytes: []byte(text), Origin: []SourceLocation{cloneJSONSourceLocation(location)}}, nil
	}

	var raw struct {
		Text       json.RawMessage `json:"text"`
		Base64     json.RawMessage `json:"base64"`
		Executable json.RawMessage `json:"executable"`
	}
	if err := DecodeStrictJSONObject(data, &raw); err != nil {
		return FileContent{}, fmt.Errorf("must be a UTF-8 string or an object with exactly one of text or base64: %w", err)
	}
	if (raw.Text == nil) == (raw.Base64 == nil) {
		return FileContent{}, fmt.Errorf("object must contain exactly one of text or base64")
	}

	var content []byte
	if raw.Text != nil {
		var value *string
		if err := DecodeStrictJSON(raw.Text, &value); err != nil || value == nil {
			if err == nil {
				err = fmt.Errorf("must be a string")
			}
			return FileContent{}, fmt.Errorf("text: %w", err)
		}
		content = []byte(*value)
	} else {
		var value *string
		if err := DecodeStrictJSON(raw.Base64, &value); err != nil || value == nil {
			if err == nil {
				err = fmt.Errorf("must be a string")
			}
			return FileContent{}, fmt.Errorf("base64: %w", err)
		}
		decoded, err := base64.StdEncoding.DecodeString(*value)
		if err != nil {
			return FileContent{}, fmt.Errorf("base64: %w", err)
		}
		content = decoded
	}

	executable := false
	if raw.Executable != nil {
		var value *bool
		if err := DecodeStrictJSON(raw.Executable, &value); err != nil || value == nil {
			if err == nil {
				err = fmt.Errorf("must be a boolean")
			}
			return FileContent{}, fmt.Errorf("executable: %w", err)
		}
		executable = *value
	}
	return FileContent{
		Bytes:      content,
		Executable: executable,
		Origin:     []SourceLocation{cloneJSONSourceLocation(location)},
	}, nil
}

func cloneJSONSourceLocation(location SourceLocation) SourceLocation {
	clone := SourceLocation{Path: location.Path}
	if location.Line != nil {
		line := *location.Line
		clone.Line = &line
	}
	if location.Column != nil {
		column := *location.Column
		clone.Column = &column
	}
	return clone
}

// DecodeStrictJSON decodes one UTF-8 JSON value, rejecting duplicate object keys,
// unknown struct fields, and trailing values.
func DecodeStrictJSON(data []byte, destination any) error {
	if len(data) == 0 || !utf8.Valid(data) {
		return fmt.Errorf("must be UTF-8 JSON")
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}

// DecodeStrictJSONObject decodes one strict JSON object.
func DecodeStrictJSONObject(data []byte, destination any) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("must be a JSON object")
	}
	return DecodeStrictJSON(data, destination)
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid JSON: multiple top-level values")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("invalid JSON object key")
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			keys[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	default:
		return fmt.Errorf("invalid JSON delimiter %q", delimiter)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid JSON: multiple top-level values")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func invalidManifestDiagnostic(message string) Diagnostic {
	return Diagnostic{
		Code:     "invalid-source-manifest",
		Severity: SeverityError,
		Message:  message,
	}
}
