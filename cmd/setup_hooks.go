package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var setupHooksCmd = &cobra.Command{
	Use:   "setup-hooks",
	Short: "Configure Claude Code hooks for message notifications",
	Long: `Configures Claude Code hooks to check for messages when Claude is idle.

This adds an idle_prompt hook that runs 'clauder check-messages' when Claude
has been waiting for input for 60+ seconds. If unread messages exist, they
are fed back to Claude so it can respond.

The hook is added to ~/.claude/settings.json.`,
	RunE: runSetupHooks,
}

func init() {
	rootCmd.AddCommand(setupHooksCmd)
}

// HookCommand represents a hook command configuration
type HookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// HookMatcher represents a hook with matcher and commands
type HookMatcher struct {
	Matcher string        `json:"matcher"`
	Hooks   []HookCommand `json:"hooks"`
}

// ClaudeSettings represents the Claude Code settings.json structure
type ClaudeSettings struct {
	Hooks map[string][]HookMatcher `json:"hooks,omitempty"`
	// Preserve other fields
	Other map[string]json.RawMessage `json:"-"`
}

func (c *ClaudeSettings) UnmarshalJSON(data []byte) error {
	// First unmarshal into a generic map to preserve unknown fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	c.Other = make(map[string]json.RawMessage)
	for k, v := range raw {
		if k == "hooks" {
			if err := json.Unmarshal(v, &c.Hooks); err != nil {
				return err
			}
		} else {
			c.Other[k] = v
		}
	}
	return nil
}

func (c ClaudeSettings) MarshalJSON() ([]byte, error) {
	// Build a map with all fields
	result := make(map[string]interface{})
	for k, v := range c.Other {
		var val interface{}
		if err := json.Unmarshal(v, &val); err != nil {
			return nil, err
		}
		result[k] = val
	}
	if c.Hooks != nil {
		result["hooks"] = c.Hooks
	}
	return json.MarshalIndent(result, "", "  ")
}

func runSetupHooks(cmd *cobra.Command, args []string) error {
	// Find clauder binary path
	binaryPath, err := getBinaryPath()
	if err != nil {
		return fmt.Errorf("failed to find clauder binary: %w", err)
	}

	// Claude Code settings file location
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")

	// Ensure .claude directory exists
	claudeDir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	// Read existing settings or create new
	var settings ClaudeSettings
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("failed to parse existing settings: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read settings file: %w", err)
	}

	// Initialize hooks map if needed
	if settings.Hooks == nil {
		settings.Hooks = make(map[string][]HookMatcher)
	}

	// Create the clauder check-messages hook
	checkMessagesHook := HookCommand{
		Type:    "command",
		Command: fmt.Sprintf("%s check-messages", binaryPath),
		Timeout: 10,
	}

	// Check if idle_prompt hook already exists for clauder
	notificationHooks := settings.Hooks["Notification"]
	alreadyExists := false
	for _, h := range notificationHooks {
		if h.Matcher == "idle_prompt" {
			for _, cmd := range h.Hooks {
				if cmd.Command == checkMessagesHook.Command ||
				   (len(cmd.Command) > 0 && filepath.Base(cmd.Command) == "clauder check-messages") {
					alreadyExists = true
					break
				}
			}
		}
	}

	if alreadyExists {
		fmt.Println("Hook already configured!")
		fmt.Printf("Settings file: %s\n", settingsPath)
		return nil
	}

	// Add or update the idle_prompt hook
	found := false
	for i, h := range notificationHooks {
		if h.Matcher == "idle_prompt" {
			// Add to existing idle_prompt hooks
			notificationHooks[i].Hooks = append(h.Hooks, checkMessagesHook)
			found = true
			break
		}
	}

	if !found {
		// Create new idle_prompt matcher
		newMatcher := HookMatcher{
			Matcher: "idle_prompt",
			Hooks:   []HookCommand{checkMessagesHook},
		}
		notificationHooks = append(notificationHooks, newMatcher)
	}

	settings.Hooks["Notification"] = notificationHooks

	// Write updated settings
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write settings: %w", err)
	}

	fmt.Println("✅ Hook configured successfully!")
	fmt.Printf("Settings file: %s\n", settingsPath)
	fmt.Println()
	fmt.Println("When Claude is idle for 60+ seconds, it will now check for")
	fmt.Println("messages from other instances and receive them automatically.")
	fmt.Println()
	fmt.Println("Note: You may need to restart Claude Code for changes to take effect.")
	fmt.Println("Also, hooks require review in /hooks menu before they become active.")

	return nil
}
