package config

import (
	"sort"
	"strings"
)

type ProvenanceLayer string

const (
	LayerDefaults    ProvenanceLayer = "defaults"
	LayerInclude     ProvenanceLayer = "include"
	LayerPrimary     ProvenanceLayer = "primary"
	LayerEnvironment ProvenanceLayer = "environment"
	LayerProfile     ProvenanceLayer = "profile"
	LayerCLI         ProvenanceLayer = "cli"
	LayerRuntime     ProvenanceLayer = "runtime"
)

type ProvenanceOrigin struct {
	Layer               ProvenanceLayer
	Name                string
	RequestedSource     string
	CanonicalSource     string
	AuthoredVersion     int
	Version             int
	CLIArgumentIndex    int
	HasCLIArgumentIndex bool
	ConfigScopeID       ConfigScopeID
	HasConfigScopeID    bool
}

type ProvenanceRecord struct {
	Path        string
	Winner      ProvenanceOrigin
	Overwritten []ProvenanceOrigin // low-to-high prior winners
	Tombstone   bool
	Sensitive   bool
}

type Provenance struct {
	records map[string]ProvenanceRecord
}

func newProvenance() Provenance {
	return Provenance{records: make(map[string]ProvenanceRecord)}
}

func (p Provenance) Lookup(path string) (ProvenanceRecord, bool) {
	record, ok := p.records[path]
	if !ok {
		return ProvenanceRecord{}, false
	}
	return cloneProvenanceRecord(record), true
}

func (p Provenance) Records() []ProvenanceRecord {
	paths := make([]string, 0, len(p.records))
	for path := range p.records {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make([]ProvenanceRecord, 0, len(paths))
	for _, path := range paths {
		out = append(out, cloneProvenanceRecord(p.records[path]))
	}
	return out
}

func (p Provenance) set(path string, origin ProvenanceOrigin, tombstone, sensitive bool) {
	record := ProvenanceRecord{Path: path, Winner: origin, Tombstone: tombstone, Sensitive: sensitive}
	if previous, ok := p.records[path]; ok {
		record.Overwritten = append(record.Overwritten, previous.Overwritten...)
		record.Overwritten = append(record.Overwritten, previous.Winner)
	}
	p.records[path] = record
}

func (p Provenance) tombstonePrefixExcept(path string, origin ProvenanceOrigin, sensitive bool, exclude map[string]struct{}) {
	prefixes := []string{path + ".", path + "["}
	for existing := range p.records {
		if _, skip := exclude[existing]; skip {
			continue
		}
		if existing == path || strings.HasPrefix(existing, prefixes[0]) || strings.HasPrefix(existing, prefixes[1]) {
			record := p.records[existing]
			p.set(existing, origin, true, sensitive || record.Sensitive)
		}
	}
}

func cloneProvenanceRecord(record ProvenanceRecord) ProvenanceRecord {
	record.Overwritten = append([]ProvenanceOrigin(nil), record.Overwritten...)
	return record
}
