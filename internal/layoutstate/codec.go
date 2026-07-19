package layoutstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func Marshal(plan Plan) ([]byte, error) {
	if err := validateDocument(plan.document); err != nil {
		return nil, err
	}
	return marshalDocument(plan.document)
}

func marshalDocument(document Document) ([]byte, error) {
	data, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("layout: encode failed")
	}
	if len(data) > MaxJSONBytes {
		return nil, fmt.Errorf("layout: exceeds %d bytes", MaxJSONBytes)
	}
	return data, nil
}

func Unmarshal(data []byte) (Plan, error) {
	if len(data) > MaxJSONBytes {
		return Plan{}, fmt.Errorf("layout: exceeds %d bytes", MaxJSONBytes)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Plan{}, errors.New("layout: must be a JSON object")
	}
	if err := rejectDuplicateKeys(data); err != nil {
		return Plan{}, fmt.Errorf("layout: %w", err)
	}
	var shape any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&shape); err != nil {
		return Plan{}, fmt.Errorf("layout: %w", err)
	}
	if err := requireEOF(dec); err != nil {
		return Plan{}, fmt.Errorf("layout: %w", err)
	}
	root, ok := shape.(map[string]any)
	if !ok {
		return Plan{}, errors.New("layout: must be a JSON object")
	}
	if err := requireShape(root, "document"); err != nil {
		return Plan{}, err
	}
	var document Document
	dec = json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&document); err != nil {
		return Plan{}, fmt.Errorf("layout: %w", err)
	}
	return NewPlan(document)
}

func rejectDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := scanValue(dec, "document", 0); err != nil {
		return err
	}
	return requireEOF(dec)
}
func scanValue(dec *json.Decoder, path string, depth int) error {
	if depth > MaxTreeDepth+16 {
		return fmt.Errorf("%s: JSON depth exceeds %d", path, MaxTreeDepth+16)
	}
	t, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch d {
	case '{':
		seen := map[string]struct{}{}
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				return err
			}
			key := k.(string)
			if _, ok := seen[key]; ok {
				return fmt.Errorf("%s: duplicate object field", path)
			}
			seen[key] = struct{}{}
			if err := scanValue(dec, path+".field", depth+1); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case '[':
		i := 0
		for dec.More() {
			if err := scanValue(dec, fmt.Sprintf("%s[%d]", path, i), depth+1); err != nil {
				return err
			}
			i++
		}
		_, err = dec.Token()
		return err
	default:
		return errors.New("invalid JSON delimiter")
	}
}
func requireEOF(dec *json.Decoder) error {
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func requireShape(object map[string]any, path string) error {
	if path == "document" {
		if err := onlyFields(object, path, "version", "active_workspace", "workspaces"); err != nil {
			return err
		}
		if err := requiredFields(object, path, "version", "active_workspace", "workspaces"); err != nil {
			return err
		}
		if err := numberFields(object, path, "version", "active_workspace"); err != nil {
			return err
		}
		return eachObject(object["workspaces"], path+".workspaces", func(value map[string]any, itemPath string) error { return requireShape(value, "workspace@"+itemPath) })
	}
	if len(path) >= 10 && path[:10] == "workspace@" {
		itemPath := path[10:]
		if err := onlyFields(object, itemPath, "name", "active_window", "windows"); err != nil {
			return err
		}
		if err := requiredFields(object, itemPath, "name", "active_window", "windows"); err != nil {
			return err
		}
		if err := stringFields(object, itemPath, "name"); err != nil {
			return err
		}
		if err := numberFields(object, itemPath, "active_window"); err != nil {
			return err
		}
		return eachObject(object["windows"], itemPath+".windows", func(value map[string]any, windowPath string) error { return requireShape(value, "window@"+windowPath) })
	}
	if len(path) >= 7 && path[:7] == "window@" {
		itemPath := path[7:]
		if err := onlyFields(object, itemPath, "title", "bounds", "active_tab", "tabs", "appearance"); err != nil {
			return err
		}
		if err := requiredFields(object, itemPath, "title", "bounds", "active_tab", "tabs", "appearance"); err != nil {
			return err
		}
		if err := stringFields(object, itemPath, "title"); err != nil {
			return err
		}
		if err := numberFields(object, itemPath, "active_tab"); err != nil {
			return err
		}
		if err := oneObject(object["bounds"], itemPath+".bounds", func(value map[string]any, valuePath string) error {
			if err := onlyFields(value, valuePath, "x", "y", "width", "height", "monitor_hint"); err != nil {
				return err
			}
			if err := requiredFields(value, valuePath, "x", "y", "width", "height", "monitor_hint"); err != nil {
				return err
			}
			if err := numberFields(value, valuePath, "x", "y", "width", "height"); err != nil {
				return err
			}
			return stringFields(value, valuePath, "monitor_hint")
		}); err != nil {
			return err
		}
		if err := oneObject(object["appearance"], itemPath+".appearance", func(value map[string]any, valuePath string) error {
			if err := onlyFields(value, valuePath, "color_scheme", "background_opacity", "text_opacity", "blur", "font_size"); err != nil {
				return err
			}
			if err := requiredFields(value, valuePath, "color_scheme"); err != nil {
				return err
			}
			if err := stringFields(value, valuePath, "color_scheme"); err != nil {
				return err
			}
			if err := optionalNumberFields(value, valuePath, "background_opacity", "text_opacity", "font_size"); err != nil {
				return err
			}
			return optionalBoolFields(value, valuePath, "blur")
		}); err != nil {
			return err
		}
		return eachObject(object["tabs"], itemPath+".tabs", func(value map[string]any, tabPath string) error { return requireShape(value, "tab@"+tabPath) })
	}
	if len(path) >= 4 && path[:4] == "tab@" {
		itemPath := path[4:]
		if err := onlyFields(object, itemPath, "title", "focused_leaf", "root"); err != nil {
			return err
		}
		if err := requiredFields(object, itemPath, "title", "focused_leaf", "root"); err != nil {
			return err
		}
		if err := stringFields(object, itemPath, "title"); err != nil {
			return err
		}
		if err := numberFields(object, itemPath, "focused_leaf"); err != nil {
			return err
		}
		return oneObject(object["root"], itemPath+".root", func(value map[string]any, nodePath string) error { return requireShape(value, "node@"+nodePath) })
	}
	if len(path) >= 5 && path[:5] == "node@" {
		itemPath := path[5:]
		if err := onlyFields(object, itemPath, "type", "launch", "axis", "ratio", "first", "second"); err != nil {
			return err
		}
		if err := requiredFields(object, itemPath, "type"); err != nil {
			return err
		}
		if err := stringFields(object, itemPath, "type"); err != nil {
			return err
		}
		if err := optionalStringFields(object, itemPath, "axis"); err != nil {
			return err
		}
		if err := optionalNumberFields(object, itemPath, "ratio"); err != nil {
			return err
		}
		if err := optionalObjectFields(object, itemPath, "launch", "first", "second"); err != nil {
			return err
		}
		typeName, _ := object["type"].(string)
		if typeName == "pane" {
			if err := requiredFields(object, itemPath, "launch"); err != nil {
				return err
			}
			return oneObject(object["launch"], itemPath+".launch", func(value map[string]any, valuePath string) error {
				if err := onlyFields(value, valuePath, "target_id", "program", "args", "cwd"); err != nil {
					return err
				}
				if err := requiredFields(value, valuePath, "target_id", "program", "args", "cwd"); err != nil {
					return err
				}
				if err := stringFields(value, valuePath, "target_id", "program", "cwd"); err != nil {
					return err
				}
				args, ok := value["args"].([]any)
				if !ok {
					return fmt.Errorf("%s.args: must be a JSON array", valuePath)
				}
				for i, arg := range args {
					if _, ok := arg.(string); !ok {
						return fmt.Errorf("%s.args[%d]: must be a JSON string", valuePath, i)
					}
				}
				return nil
			})
		}
		if typeName == "split" {
			if err := requiredFields(object, itemPath, "axis", "ratio", "first", "second"); err != nil {
				return err
			}
			for _, field := range []string{"first", "second"} {
				if err := oneObject(object[field], itemPath+"."+field, func(value map[string]any, nodePath string) error { return requireShape(value, "node@"+nodePath) }); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func requiredFields(object map[string]any, path string, fields ...string) error {
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			return fmt.Errorf("%s: missing required field", path)
		}
	}
	return nil
}

func stringFields(object map[string]any, path string, fields ...string) error {
	for _, field := range fields {
		if _, ok := object[field].(string); !ok {
			return fmt.Errorf("%s.%s: must be a JSON string", path, field)
		}
	}
	return nil
}

func numberFields(object map[string]any, path string, fields ...string) error {
	for _, field := range fields {
		if _, ok := object[field].(json.Number); !ok {
			return fmt.Errorf("%s.%s: must be a JSON number", path, field)
		}
	}
	return nil
}

func optionalStringFields(object map[string]any, path string, fields ...string) error {
	for _, field := range fields {
		if value, exists := object[field]; exists {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s.%s: must be a JSON string", path, field)
			}
		}
	}
	return nil
}

func optionalNumberFields(object map[string]any, path string, fields ...string) error {
	for _, field := range fields {
		if value, exists := object[field]; exists {
			if _, ok := value.(json.Number); !ok {
				return fmt.Errorf("%s.%s: must be a JSON number", path, field)
			}
		}
	}
	return nil
}

func optionalBoolFields(object map[string]any, path string, fields ...string) error {
	for _, field := range fields {
		if value, exists := object[field]; exists {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s.%s: must be a JSON boolean", path, field)
			}
		}
	}
	return nil
}

func optionalObjectFields(object map[string]any, path string, fields ...string) error {
	for _, field := range fields {
		if value, exists := object[field]; exists {
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("%s.%s: must be a JSON object", path, field)
			}
		}
	}
	return nil
}

func onlyFields(object map[string]any, path string, allowed ...string) error {
	set := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		set[field] = struct{}{}
	}
	for field := range object {
		if _, ok := set[field]; !ok {
			return fmt.Errorf("%s: unknown field", path)
		}
	}
	return nil
}

func oneObject(value any, path string, fn func(map[string]any, string) error) error {
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: must be a JSON object", path)
	}
	return fn(object, path)
}

func eachObject(value any, path string, fn func(map[string]any, string) error) error {
	array, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s: must be a JSON array", path)
	}
	for i, item := range array {
		if err := oneObject(item, fmt.Sprintf("%s[%d]", path, i), fn); err != nil {
			return err
		}
	}
	return nil
}
