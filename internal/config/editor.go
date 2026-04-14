package config

import (
	"context"
	"os"
	"time"
)

type Document struct {
	Path    string
	Content string
}

type SaveResult struct {
	Changed          bool
	RestartScheduled bool
	Path             string
}

type Editor struct {
	path           string
	requestRestart func()
}

func NewEditor(path string, requestRestart func()) *Editor {
	return &Editor{
		path:           path,
		requestRestart: requestRestart,
	}
}

func (e *Editor) Load(_ context.Context) (Document, error) {
	data, err := os.ReadFile(e.path)
	if err != nil {
		return Document{}, err
	}
	return Document{
		Path:    e.path,
		Content: string(data),
	}, nil
}

func (e *Editor) Save(_ context.Context, content string) (SaveResult, error) {
	current, err := os.ReadFile(e.path)
	if err != nil {
		return SaveResult{}, err
	}
	if string(current) == content {
		return SaveResult{
			Changed:          false,
			RestartScheduled: false,
			Path:             e.path,
		}, nil
	}

	if _, err := Parse([]byte(content)); err != nil {
		return SaveResult{}, err
	}

	mode := os.FileMode(0o644)
	if info, err := os.Stat(e.path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(e.path, []byte(content), mode); err != nil {
		return SaveResult{}, err
	}

	if e.requestRestart != nil {
		go func() {
			time.Sleep(300 * time.Millisecond)
			e.requestRestart()
		}()
	}

	return SaveResult{
		Changed:          true,
		RestartScheduled: e.requestRestart != nil,
		Path:             e.path,
	}, nil
}
