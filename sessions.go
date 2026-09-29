package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Session struct {
	Name string
	Exec []string
}

// loadSessions 扫描 wayland-sessions 下的 .desktop 文件。
func loadSessions() []Session {
	dirs := []string{
		"/usr/share/wayland-sessions",
		"/usr/local/share/wayland-sessions",
	}
	var out []Session
	for _, d := range dirs {
		files, _ := filepath.Glob(filepath.Join(d, "*.desktop"))
		sort.Strings(files)
		for _, f := range files {
			if s, ok := parseDesktop(f); ok {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 {
		out = append(out, Session{Name: "Shell", Exec: []string{"/bin/sh"}})
	}
	return out
}

func parseDesktop(path string) (Session, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, false
	}
	defer f.Close()

	var s Session
	inEntry := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "[Desktop Entry]":
			inEntry = true
		case strings.HasPrefix(line, "["):
			inEntry = false
		case !inEntry:
		default:
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch k {
			case "Name":
				s.Name = v
			case "Exec":
				s.Exec = strings.Fields(v)
			case "Hidden", "NoDisplay":
				if v == "true" {
					return Session{}, false
				}
			}
		}
	}
	return s, s.Name != "" && len(s.Exec) > 0
}

// 记住上次登录的用户和会话。
type state struct {
	User    string `json:"user"`
	Session string `json:"session"`
}

func loadState(path string) state {
	var st state
	b, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	_ = json.Unmarshal(b, &st)
	return st
}

func saveState(path string, st state) {
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, b, 0o644)
}
