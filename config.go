package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// dotenv contains the configuration loaded from .env. It is deliberately
// kept separate from the process environment so an exported OS variable
// cannot silently override the values in the project's .env file.
var dotenv map[string]string

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, rawValue, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return fmt.Errorf("invalid entry in %s at line %d", path, lineNumber)
		}

		value := strings.TrimSpace(rawValue)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') ||
			(value[0] == '"' && value[len(value)-1] == '"')) {
			if value[0] == '"' {
				value, err = strconv.Unquote(value)
				if err != nil {
					return fmt.Errorf("invalid quoted value in %s at line %d: %w", path, lineNumber, err)
				}
			} else {
				value = value[1 : len(value)-1]
			}
		} else if comment := strings.Index(value, " #"); comment >= 0 {
			value = strings.TrimSpace(value[:comment])
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	dotenv = values
	return nil
}

func envOrDefault(name, fallback string) string {
	if value := dotenv[name]; value != "" {
		return value
	}
	return fallback
}
