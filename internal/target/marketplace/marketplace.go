// Package marketplace builds target-neutral marketplace catalog entries.
package marketplace

import (
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

var (
	catalogIdentifierPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	semanticVersionPattern   = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
)

// Person is normalized publication contact metadata.
type Person struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Entry is one ordered package catalog entry. Source is a package root relative
// to the target output root: "." for a flat package or its package ID otherwise.
type Entry struct {
	Name        string
	Source      string
	Description string
	Version     string
	Author      Person
	Homepage    string
	Repository  string
	License     string
	Keywords    []string
}

// Catalog is deterministic target-neutral data for a target-owned serializer.
type Catalog struct {
	Name        string
	Owner       Person
	Description string
	Version     string
	Entries     []Entry
}

// Build validates explicit publication metadata and builds entries ordered by
// package identity. It performs no filesystem, process, clock, Git, or network work.
func Build(input model.TargetRenderInput) (Catalog, []model.Diagnostic) {
	if input.PackageMode != model.TargetPackageModeSeparate {
		return Catalog{}, diagnostics("invalid-catalog-package-mode", "marketplace catalogs require separate package mode")
	}
	if len(input.Packages) == 0 {
		return Catalog{}, diagnostics("invalid-catalog-packages", "marketplace catalogs require at least one package")
	}

	name, err := requiredString(input.Distribution, "name", "distribution")
	if err != nil {
		return Catalog{}, diagnostics("invalid-distribution-metadata", err.Error())
	}
	if err := validateCatalogIdentifier(name, "distribution name"); err != nil {
		return Catalog{}, diagnostics("invalid-distribution-metadata", err.Error())
	}
	ownerValue, exists := input.Distribution["owner"]
	if !exists {
		return Catalog{}, diagnostics("invalid-distribution-metadata", "distribution requires owner")
	}
	owner, err := person(ownerValue, "distribution owner", false)
	if err != nil {
		return Catalog{}, diagnostics("invalid-distribution-metadata", err.Error())
	}
	description, err := requiredString(input.Distribution, "description", "distribution")
	if err != nil {
		return Catalog{}, diagnostics("invalid-distribution-metadata", err.Error())
	}
	if err := validateText(description, "distribution description"); err != nil {
		return Catalog{}, diagnostics("invalid-distribution-metadata", err.Error())
	}
	version, err := requiredString(input.Distribution, "version", "distribution")
	if err != nil {
		return Catalog{}, diagnostics("invalid-distribution-metadata", err.Error())
	}
	if err := validateSemanticVersion(version, "distribution version"); err != nil {
		return Catalog{}, diagnostics("invalid-distribution-metadata", err.Error())
	}
	for key := range input.Distribution {
		switch key {
		case "name", "owner", "description", "version":
		default:
			return Catalog{}, diagnostics("invalid-distribution-metadata", fmt.Sprintf("distribution field %q is not supported", key))
		}
	}

	packages := append([]model.NormalizedPackage(nil), input.Packages...)
	sort.Slice(packages, func(left, right int) bool { return packages[left].Identity < packages[right].Identity })
	catalog := Catalog{
		Name: name, Owner: owner, Description: description, Version: version,
		Entries: make([]Entry, 0, len(packages)),
	}
	identities := make(map[string]model.PackageID, len(packages))
	sources := make(map[string]model.PackageID, len(packages))
	for _, pkg := range packages {
		identity := string(pkg.Identity)
		if err := validateCatalogIdentifier(identity, "package ID"); err != nil {
			return Catalog{}, packageDiagnostics(pkg.Identity, "invalid-package-publication-metadata", err.Error())
		}
		identityKey := strings.ToLower(identity)
		if previous, exists := identities[identityKey]; exists {
			return Catalog{}, packageDiagnostics(pkg.Identity, "catalog-identity-collision", fmt.Sprintf("package IDs %q and %q collide in marketplace identity", previous, pkg.Identity))
		}
		identities[identityKey] = pkg.Identity

		source := identity
		if len(packages) == 1 {
			source = "."
		}
		sourceKey := strings.ToLower(source)
		if previous, exists := sources[sourceKey]; exists {
			return Catalog{}, packageDiagnostics(pkg.Identity, "catalog-path-collision", fmt.Sprintf("packages %q and %q resolve to the same catalog source %q", previous, pkg.Identity, source))
		}
		sources[sourceKey] = pkg.Identity

		entry, err := buildEntry(pkg, source)
		if err != nil {
			return Catalog{}, packageDiagnostics(pkg.Identity, "invalid-package-publication-metadata", err.Error())
		}
		catalog.Entries = append(catalog.Entries, entry)
	}
	return catalog, nil
}

func buildEntry(pkg model.NormalizedPackage, source string) (Entry, error) {
	description, err := requiredString(pkg.Metadata, "description", fmt.Sprintf("package %q", pkg.Identity))
	if err != nil {
		return Entry{}, err
	}
	if err := validateText(description, fmt.Sprintf("package %q description", pkg.Identity)); err != nil {
		return Entry{}, err
	}
	version, err := requiredString(pkg.Metadata, "version", fmt.Sprintf("package %q", pkg.Identity))
	if err != nil {
		return Entry{}, err
	}
	if err := validateSemanticVersion(version, fmt.Sprintf("package %q version", pkg.Identity)); err != nil {
		return Entry{}, err
	}
	authorValue, exists := pkg.Metadata["author"]
	if !exists {
		return Entry{}, fmt.Errorf("package %q requires author", pkg.Identity)
	}
	author, err := person(authorValue, fmt.Sprintf("package %q author", pkg.Identity), true)
	if err != nil {
		return Entry{}, err
	}
	homepage, err := requiredString(pkg.Metadata, "homepage", fmt.Sprintf("package %q", pkg.Identity))
	if err != nil {
		return Entry{}, err
	}
	if err := validateWebURL(homepage, fmt.Sprintf("package %q homepage", pkg.Identity)); err != nil {
		return Entry{}, err
	}
	repository, err := requiredString(pkg.Metadata, "repository", fmt.Sprintf("package %q", pkg.Identity))
	if err != nil {
		return Entry{}, err
	}
	if err := validateWebURL(repository, fmt.Sprintf("package %q repository", pkg.Identity)); err != nil {
		return Entry{}, err
	}
	license, err := requiredString(pkg.Metadata, "license", fmt.Sprintf("package %q", pkg.Identity))
	if err != nil {
		return Entry{}, err
	}
	if err := validateText(license, fmt.Sprintf("package %q license", pkg.Identity)); err != nil {
		return Entry{}, err
	}
	keywords, err := stringList(pkg.Metadata["keywords"], fmt.Sprintf("package %q keywords", pkg.Identity))
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Name: string(pkg.Identity), Source: source, Description: description, Version: version,
		Author: author, Homepage: homepage, Repository: repository, License: license, Keywords: keywords,
	}, nil
}

func requiredString(metadata map[string]any, key, owner string) (string, error) {
	value, exists := metadata[key]
	if !exists {
		return "", fmt.Errorf("%s requires %s", owner, key)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s %s must be a string", owner, key)
	}
	if err := validateText(text, owner+" "+key); err != nil {
		return "", err
	}
	return text, nil
}

func person(value any, field string, allowURL bool) (Person, error) {
	switch typed := value.(type) {
	case string:
		if err := validateText(typed, field); err != nil {
			return Person{}, err
		}
		return Person{Name: typed}, nil
	case map[string]any:
		return personMap(typed, field, allowURL)
	case map[string]string:
		values := make(map[string]any, len(typed))
		for key, item := range typed {
			values[key] = item
		}
		return personMap(values, field, allowURL)
	default:
		return Person{}, fmt.Errorf("%s must be a string or object", field)
	}
}

func personMap(values map[string]any, field string, allowURL bool) (Person, error) {
	name, err := requiredString(values, "name", field)
	if err != nil {
		return Person{}, err
	}
	result := Person{Name: name}
	for key, value := range values {
		if key == "name" {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return Person{}, fmt.Errorf("%s %s must be a string", field, key)
		}
		switch key {
		case "email":
			if err := validateEmail(text, field+" email"); err != nil {
				return Person{}, err
			}
			result.Email = text
		case "url":
			if !allowURL {
				return Person{}, fmt.Errorf("%s field %q is not supported", field, key)
			}
			if err := validateWebURL(text, field+" url"); err != nil {
				return Person{}, err
			}
			result.URL = text
		default:
			return Person{}, fmt.Errorf("%s field %q is not supported", field, key)
		}
	}
	return result, nil
}

func stringList(value any, field string) ([]string, error) {
	var values []string
	switch typed := value.(type) {
	case []string:
		values = append(values, typed...)
	case []any:
		values = make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s must contain only strings", field)
			}
			values = append(values, text)
		}
	default:
		return nil, fmt.Errorf("%s must be a non-empty string array", field)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s must be a non-empty string array", field)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateText(value, field+" item"); err != nil {
			return nil, err
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%s contains duplicate value %q", field, value)
		}
		seen[key] = struct{}{}
	}
	sort.Strings(values)
	return values, nil
}

func validateCatalogIdentifier(value, field string) error {
	if !catalogIdentifierPattern.MatchString(value) {
		return fmt.Errorf("%s must be a lowercase kebab-case identifier", field)
	}
	return nil
}

func validateText(value, field string) error {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s must be non-empty, trimmed text without NUL", field)
	}
	return nil
}

func validateSemanticVersion(value, field string) error {
	matches := semanticVersionPattern.FindStringSubmatch(value)
	if matches == nil {
		return fmt.Errorf("%s must be a semantic version", field)
	}
	if matches[4] != "" {
		for _, identifier := range strings.Split(matches[4], ".") {
			if len(identifier) > 1 && identifier[0] == '0' {
				allDigits := true
				for _, character := range identifier {
					if character < '0' || character > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					return fmt.Errorf("%s must be a semantic version", field)
				}
			}
		}
	}
	return nil
}

func validateWebURL(value, field string) error {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("%s must be an absolute HTTP or HTTPS URL without credentials", field)
	}
	return nil
}

func validateEmail(value, field string) error {
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return fmt.Errorf("%s must be an email address", field)
	}
	return nil
}

func diagnostics(code, message string) []model.Diagnostic {
	return []model.Diagnostic{{Code: code, Severity: model.SeverityError, Message: message}}
}

func packageDiagnostics(packageID model.PackageID, code, message string) []model.Diagnostic {
	return []model.Diagnostic{{Code: code, Severity: model.SeverityError, Message: message, Asset: model.AssetID("package/" + string(packageID))}}
}
