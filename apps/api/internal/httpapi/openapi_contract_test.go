package httpapi

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

type openAPIStringSchema struct {
	Type      string                `yaml:"type"`
	MinLength *int                  `yaml:"minLength"`
	MaxLength *int                  `yaml:"maxLength"`
	Pattern   string                `yaml:"pattern"`
	OneOf     []openAPIStringSchema `yaml:"oneOf"`
}

func TestDesktopOAuthGrantOpenAPIContractMatchesRuntime(t *testing.T) {
	t.Parallel()
	codeSchema := loadDesktopOAuthGrantCodeSchema(t)
	if len(codeSchema.OneOf) != 2 {
		t.Fatalf("expected two desktop OAuth grant formats, got %d", len(codeSchema.OneOf))
	}

	candidates := []string{
		"",
		strings.Repeat("a", 31),
		strings.Repeat("a", 32),
		strings.Repeat("a", 33),
		strings.Repeat("a", 42),
		strings.Repeat("a", 43),
		strings.Repeat("a", 44),
		strings.Repeat("A", 32),
		strings.Repeat("A", 43),
		strings.Repeat("_", 43),
		strings.Repeat("-", 43),
		strings.Repeat("f", 32),
		strings.Repeat("g", 32),
		strings.Repeat(".", 43),
		strings.Repeat("=", 43),
		strings.Repeat("/", 43),
		strings.Repeat("+", 43),
		strings.Repeat(" ", 43),
		strings.Repeat("é", 43),
	}
	for length := 0; length <= 64; length++ {
		candidates = append(candidates,
			strings.Repeat("0", length),
			strings.Repeat("Z", length),
			strings.Repeat("_", length),
		)
	}
	for protocol := int64(1); protocol <= 2; protocol++ {
		for range 64 {
			grant, err := newDesktopOAuthGrantCode(protocol)
			if err != nil {
				t.Fatal(err)
			}
			candidates = append(candidates, grant)
		}
	}

	for _, candidate := range candidates {
		runtimeAccepts := validOAuthGrantCode(candidate)
		schemaAccepts := openAPIStringSchemaAccepts(t, codeSchema, candidate)
		if schemaAccepts != runtimeAccepts {
			t.Fatalf("desktop grant contract drift for %q: schema=%t runtime=%t", candidate, schemaAccepts, runtimeAccepts)
		}
	}
}

func loadDesktopOAuthGrantCodeSchema(t *testing.T) openAPIStringSchema {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	specBytes, err := os.ReadFile(filepath.Join(filepath.Dir(testFile), "../../../../packages/protocol/openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]openAPIStringSchema `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(specBytes, &spec); err != nil {
		t.Fatal(err)
	}
	requestSchema, ok := spec.Components.Schemas["ConsumeDesktopGitHubOAuthRequest"]
	if !ok {
		t.Fatal("ConsumeDesktopGitHubOAuthRequest schema is missing")
	}
	codeSchema, ok := requestSchema.Properties["code"]
	if !ok {
		t.Fatal("ConsumeDesktopGitHubOAuthRequest.code schema is missing")
	}
	return codeSchema
}

func openAPIStringSchemaAccepts(t *testing.T, schema openAPIStringSchema, value string) bool {
	t.Helper()
	if len(schema.OneOf) > 0 {
		matches := 0
		for _, candidate := range schema.OneOf {
			if openAPIStringSchemaAccepts(t, candidate, value) {
				matches++
			}
		}
		return matches == 1
	}
	if schema.Type != "" && schema.Type != "string" {
		return false
	}
	length := utf8.RuneCountInString(value)
	if schema.MinLength != nil && length < *schema.MinLength {
		return false
	}
	if schema.MaxLength != nil && length > *schema.MaxLength {
		return false
	}
	if schema.Pattern == "" {
		return true
	}
	pattern, err := regexp.Compile(schema.Pattern)
	if err != nil {
		t.Fatalf("compile OpenAPI pattern %q: %v", schema.Pattern, err)
	}
	return pattern.MatchString(value)
}
