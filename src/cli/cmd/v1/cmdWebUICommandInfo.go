package cmd

import (
	"reflect"
	"strings"
)

// CommandInfo holds metadata about a command for the REST API
type CommandInfo struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"displayName,omitempty"`
	Path        string          `json:"path"`
	Description string          `json:"description"`
	Icon        string          `json:"icon,omitempty"`
	Hidden      bool            `json:"hidden,omitempty"`
	WebHidden   bool            `json:"webHidden,omitempty"`
	SimpleMode  bool            `json:"simpleMode"`
	HasChildren bool            `json:"hasChildren"`
	InvWebForce bool            `json:"invWebForce,omitempty"`
	Children    []*CommandInfo  `json:"children,omitempty"`
	Parameters  []ParameterInfo `json:"parameters,omitempty"`
	// Internal: reflect type for execution
	reflectType reflect.Type `json:"-"`
}

// ParameterInfo holds metadata about a command parameter
type ParameterInfo struct {
	Name          string   `json:"name"`
	DisplayName   string   `json:"displayName,omitempty"`
	FieldName     string   `json:"fieldName"`
	Short         string   `json:"short,omitempty"`
	Long          string   `json:"long,omitempty"`
	Description   string   `json:"description,omitempty"`
	Type          string   `json:"type"`
	Default       string   `json:"default,omitempty"`
	Required      bool     `json:"required,omitempty"`
	WebType       string   `json:"webType,omitempty"`
	Choices       []string `json:"choices,omitempty"`
	ChoiceLabels  []string `json:"choiceLabels,omitempty"`
	ChoicesMethod string   `json:"choicesMethod,omitempty"`
	Hidden        bool     `json:"hidden,omitempty"`
	WebHidden     bool     `json:"webHidden,omitempty"`
	SimpleMode    bool     `json:"simpleMode"`
	Group         string   `json:"group,omitempty"`
	Namespace     string   `json:"namespace,omitempty"`
	NoDefault     bool     `json:"noDefault,omitempty"`
	IsSlice       bool     `json:"isSlice,omitempty"`
	IsPositional  bool     `json:"isPositional,omitempty"`
	IsFile        bool     `json:"isFile,omitempty"`
	Optional      bool     `json:"optional,omitempty"`
}

// FindByPath finds a command by its path (e.g., "cluster/create")
func (c *CommandInfo) FindByPath(path string) *CommandInfo {
	if path == "" {
		return c
	}

	parts := strings.Split(path, "/")
	current := c

	for _, part := range parts {
		found := false
		for _, child := range current.Children {
			if child.Name == part {
				current = child
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	return current
}
