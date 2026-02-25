package creator

import (
	"os"

	"gopkg.in/yaml.v3"
)

type filePayload struct {
	Creators []Entry `yaml:"creators" json:"creators"`
}

func LoadEntries(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var payload filePayload
	if err := yaml.Unmarshal(data, &payload); err == nil && len(payload.Creators) > 0 {
		return payload.Creators, nil
	}

	var list []Entry
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}
