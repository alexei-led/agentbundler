package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// DecodeSourceManifestJSON decodes and validates a source manifest with strict JSON object handling.
func DecodeSourceManifestJSON(data []byte) (SourceManifest, []Diagnostic) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return SourceManifest{}, []Diagnostic{invalidManifestDiagnostic(err.Error())}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var manifest SourceManifest
	if err := decoder.Decode(&manifest); err != nil {
		return SourceManifest{}, []Diagnostic{invalidManifestDiagnostic(err.Error())}
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return SourceManifest{}, []Diagnostic{invalidManifestDiagnostic(err.Error())}
	}
	return manifest, ValidateSourceManifest(manifest)
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
