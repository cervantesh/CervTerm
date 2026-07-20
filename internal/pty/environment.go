package pty

import (
	"sort"
	"strings"
)

// MergeEnvironment combines an inherited environment with configured values.
// Configured values always win. Keys use the child platform's identity rules,
// and the result is sorted by normalized key so process creation is reproducible
// and never exposes duplicate keys to the child.
func MergeEnvironment(inherited []string, configured map[string]string, windows bool) []string {
	type entry struct {
		key   string
		value string
	}
	merged := make(map[string]entry, len(inherited)+len(configured))
	normalize := func(key string) string {
		if windows {
			return strings.ToUpper(key)
		}
		return key
	}
	for _, item := range inherited {
		key, value, ok := splitEnvironmentEntry(item, windows)
		if !ok {
			continue
		}
		merged[normalize(key)] = entry{key: key, value: value}
	}
	configuredKeys := make([]string, 0, len(configured))
	for key := range configured {
		configuredKeys = append(configuredKeys, key)
	}
	sort.Strings(configuredKeys)
	for _, key := range configuredKeys {
		merged[normalize(key)] = entry{key: key, value: configured[key]}
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		item := merged[key]
		result = append(result, item.key+"="+item.value)
	}
	return result
}

func splitEnvironmentEntry(item string, windows bool) (string, string, bool) {
	start := 0
	if windows && strings.HasPrefix(item, "=") {
		start = 1 // preserve Windows drive-current-directory entries such as =C:=C:\\.
	}
	index := strings.IndexByte(item[start:], '=')
	if index < 0 {
		return "", "", false
	}
	index += start
	if index == 0 {
		return "", "", false
	}
	return item[:index], item[index+1:], true
}
