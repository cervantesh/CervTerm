package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"cervterm/internal/fontdesc"
)

const (
	DiagnosticRedacted   = "<redacted>"
	DiagnosticConfigured = "<configured>"
	DiagnosticUnset      = "<unset>"
)

// ConfigDiagnostic is a detached, renderer-neutral snapshot of effective
// schema-v2 configuration values. Fields are always in public schema order.
type ConfigDiagnostic struct {
	ConfigVersion int                     `json:"config_version"`
	Fields        []ConfigFieldDiagnostic `json:"fields"`
}

// ConfigFieldDiagnostic describes one public schema leaf. Value is either a
// JSON scalar/list or one of the diagnostic marker constants above.
type ConfigFieldDiagnostic struct {
	Metadata   FieldMetadata      `json:"metadata"`
	Value      string             `json:"value"`
	Provenance []ProvenanceRecord `json:"provenance,omitempty"`
	ShadowedBy string             `json:"shadowed_by,omitempty"`
}

type diagnosticDescriptor struct {
	Family          string  `json:"family"`
	CollectionFace  string  `json:"collection_face,omitempty"`
	CollectionIndex *uint32 `json:"collection_index,omitempty"`
	Weight          int     `json:"weight"`
	Style           string  `json:"style"`
	Stretch         int     `json:"stretch"`
	AttributeMode   string  `json:"attribute_mode"`
}

type diagnosticIntRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type diagnosticRuneRange struct {
	First int32 `json:"first"`
	Last  int32 `json:"last"`
}

type diagnosticRuleMatch struct {
	Weight  *diagnosticIntRange   `json:"weight,omitempty"`
	Styles  []string              `json:"styles,omitempty"`
	Stretch *diagnosticIntRange   `json:"stretch,omitempty"`
	Ranges  []diagnosticRuneRange `json:"ranges,omitempty"`
	Class   string                `json:"class,omitempty"`
}

type diagnosticRule struct {
	Match diagnosticRuleMatch  `json:"match"`
	Use   diagnosticDescriptor `json:"use"`
}

// ConfigDiagnosticPathError reports an invalid exact field filter. Its text is
// intentionally limited to the supplied path so callers cannot accidentally
// echo a value paired with the filter.
type ConfigDiagnosticPathError struct {
	Path string
}

func (e ConfigDiagnosticPathError) Error() string { return e.Path }

// DiagnoseConfig returns deterministic public schema-v2 leaf diagnostics.
// Filters are exact leaf paths; duplicates are ignored and output remains in
// schema order. An empty filter list selects every public leaf.
func DiagnoseConfig(cfg Config, provenance Provenance, filters []string) (ConfigDiagnostic, error) {
	metadata, err := SchemaFields(CurrentSchemaVersion)
	if err != nil {
		return ConfigDiagnostic{}, err
	}
	leaves := make([]FieldMetadata, 0, len(metadata))
	byPath := make(map[string]FieldMetadata, len(metadata))
	for _, field := range metadata {
		if !field.Available || field.Kind == KindTable {
			continue
		}
		leaves = append(leaves, field)
		byPath[field.Path] = field
	}

	selected := make(map[string]struct{}, len(filters))
	for _, path := range filters {
		field, ok := byPath[path]
		if !ok || field.Kind == KindTable {
			return ConfigDiagnostic{}, ConfigDiagnosticPathError{Path: path}
		}
		selected[path] = struct{}{}
	}

	result := ConfigDiagnostic{ConfigVersion: CurrentSchemaVersion}
	for _, field := range leaves {
		if len(filters) != 0 {
			if _, ok := selected[field.Path]; !ok {
				continue
			}
		}
		value, err := diagnosticFieldValue(cfg, field, provenance)
		if err != nil {
			return ConfigDiagnostic{}, fmt.Errorf("diagnose config field %q: %w", field.Path, err)
		}
		diagnostic := ConfigFieldDiagnostic{
			Metadata: field, Value: value, Provenance: diagnosticProvenance(provenance, field.Path),
		}
		if field.Path == "font.family" && len(cfg.Font.Descriptors) != 0 {
			diagnostic.ShadowedBy = "font.descriptors"
		}
		result.Fields = append(result.Fields, diagnostic)
	}
	return result, nil
}

// BuildConfigDiagnostic is an explicit constructor alias for DiagnoseConfig.
func BuildConfigDiagnostic(cfg Config, provenance Provenance, filters []string) (ConfigDiagnostic, error) {
	return DiagnoseConfig(cfg, provenance, filters)
}

func diagnosticFieldValue(cfg Config, metadata FieldMetadata, provenance Provenance) (string, error) {
	if metadata.Sensitive || metadata.Path == "shell.env" {
		return DiagnosticRedacted, nil
	}
	if metadata.Path == "config_version" {
		return "2", nil
	}
	if metadata.Path == "keys" || metadata.Path == "events" {
		if hasConfiguredProvenance(provenance, metadata.Path) {
			return DiagnosticConfigured, nil
		}
		return DiagnosticUnset, nil
	}

	if metadata.Path == "font.descriptors" || metadata.Path == "font.fallback" {
		descriptors := cfg.Font.Descriptors
		if metadata.Path == "font.fallback" {
			descriptors = cfg.Font.Fallback
		}
		values, err := diagnosticDescriptors(descriptors)
		if err != nil {
			return "", fmt.Errorf("%s: %w", metadata.Path, err)
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	}
	if metadata.Path == "font.rules" {
		values := make([]diagnosticRule, len(cfg.Font.Rules))
		for index, rule := range cfg.Font.Rules {
			normalized, err := rule.Normalize()
			if err != nil {
				return "", fmt.Errorf("font.rules[%d]: %w", index+1, err)
			}
			use, err := diagnosticDescriptors([]fontdesc.Descriptor{normalized.Use})
			if err != nil {
				return "", err
			}
			match := diagnosticRuleMatch{Class: string(normalized.Match.Class)}
			if normalized.Match.Weight.Present {
				match.Weight = &diagnosticIntRange{Min: normalized.Match.Weight.Min, Max: normalized.Match.Weight.Max}
			}
			if normalized.Match.Stretch.Present {
				match.Stretch = &diagnosticIntRange{Min: normalized.Match.Stretch.Min, Max: normalized.Match.Stretch.Max}
			}
			for _, style := range normalized.Match.Styles {
				match.Styles = append(match.Styles, string(style))
			}
			for _, item := range normalized.Match.Ranges {
				match.Ranges = append(match.Ranges, diagnosticRuneRange{First: int32(item.First), Last: int32(item.Last)})
			}
			values[index] = diagnosticRule{Match: match, Use: use[0]}
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	}

	value, ok := configFieldBySchemaPath(reflect.ValueOf(cfg), strings.Split(metadata.Path, "."))
	if !ok {
		return "", fmt.Errorf("schema leaf has no Config field")
	}
	encoded, err := json.Marshal(value.Interface())
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func diagnosticDescriptors(descriptors []fontdesc.Descriptor) ([]diagnosticDescriptor, error) {
	values := make([]diagnosticDescriptor, len(descriptors))
	for index, descriptor := range descriptors {
		normalized, err := descriptor.Normalize()
		if err != nil {
			return nil, fmt.Errorf("descriptor[%d]: %w", index+1, err)
		}
		values[index] = diagnosticDescriptor{
			Family: normalized.Family, CollectionFace: normalized.CollectionFace,
			Weight: normalized.Weight, Style: string(normalized.Style), Stretch: normalized.Stretch,
			AttributeMode: string(normalized.AttributeMode),
		}
		if normalized.CollectionIndex.Present {
			collectionIndex := normalized.CollectionIndex.Value
			values[index].CollectionIndex = &collectionIndex
		}
	}
	return values, nil
}

func configFieldBySchemaPath(value reflect.Value, parts []string) (reflect.Value, bool) {
	current := value
	for _, part := range parts {
		for current.Kind() == reflect.Pointer {
			if current.IsNil() {
				return reflect.Value{}, false
			}
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		want := normalizedSchemaName(part)
		matched := reflect.Value{}
		typeInfo := current.Type()
		for index := 0; index < current.NumField(); index++ {
			fieldInfo := typeInfo.Field(index)
			if fieldInfo.PkgPath != "" {
				continue
			}
			if normalizedSchemaName(fieldInfo.Name) == want {
				matched = current.Field(index)
				break
			}
		}
		if !matched.IsValid() {
			return reflect.Value{}, false
		}
		current = matched
	}
	return current, current.IsValid() && current.CanInterface()
}

func normalizedSchemaName(value string) string {
	var normalized strings.Builder
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(unicode.ToLower(character))
		}
	}
	return normalized.String()
}

func diagnosticProvenance(provenance Provenance, path string) []ProvenanceRecord {
	var records []ProvenanceRecord
	for _, record := range provenance.Records() {
		if record.Path == path || strings.HasPrefix(record.Path, path+".") || strings.HasPrefix(record.Path, path+"[") {
			record.Overwritten = append([]ProvenanceOrigin(nil), record.Overwritten...)
			records = append(records, record)
		}
	}
	return records
}

func hasConfiguredProvenance(provenance Provenance, path string) bool {
	for _, record := range diagnosticProvenance(provenance, path) {
		if !record.Tombstone && record.Winner.Layer != "" && record.Winner.Layer != LayerDefaults {
			return true
		}
	}
	return false
}
