package rules

import (
	"fmt"
	"sync"
)

type Validator func(map[string]any) error

var (
	mu         sync.RWMutex
	validators = map[string]Validator{}
)

func Register(name string, validator Validator) error {
	if name == "" || validator == nil {
		return fmt.Errorf("rule name and validator are required")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := validators[name]; exists {
		return fmt.Errorf("rule %q is already registered", name)
	}
	validators[name] = validator
	return nil
}

func Validate(name string, payload map[string]any) error {
	mu.RLock()
	validator, ok := validators[name]
	mu.RUnlock()
	if !ok {
		return fmt.Errorf("rule %q is not registered", name)
	}
	return validator(payload)
}
