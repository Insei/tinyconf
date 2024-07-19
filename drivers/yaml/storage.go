package yaml

import (
	"fmt"
	"io"
	"os"

	"github.com/insei/tinyconf"
	"gopkg.in/yaml.v3"
)

type storageImpl struct {
	filePath    string
	yamlMap     map[string]any
	initialized bool
}

type storage interface {
	load() (map[string]any, error)
}

type readerCloser interface {
	io.Reader
	io.Closer
}

func (s *storageImpl) getMap() map[string]any {
	return s.yamlMap
}

func (s *storageImpl) getReaderCloser() (readerCloser, error) {
	f, err := os.Open(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("error while open file: %s", err)
	}
	return f, nil
}

func (s *storageImpl) parseYAML(r readerCloser) error {
	defer r.Close()
	s.yamlMap = make(map[string]any)
	decoder := yaml.NewDecoder(r)
	err := decoder.Decode(&s.yamlMap)
	if err != nil {
		return fmt.Errorf("failed to decode yaml: %s", err)
	}
	return nil
}

func (s *storageImpl) load() (map[string]any, error) {
	if s.yamlMap == nil && s.initialized {
		return nil, fmt.Errorf("%w: value not found in yaml config", tinyconf.ErrValueNotFound)
	}
	s.initialized = true
	rc, err := s.getReaderCloser()
	if err != nil {
		return nil, err
	}
	return s.yamlMap, s.parseYAML(rc)
}
