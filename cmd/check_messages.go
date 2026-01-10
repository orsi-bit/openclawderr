package cmd

import (
	"fmt"
	"os"

	"github.com/maorbril/clauder/internal/store"
	"github.com/spf13/cobra"
)

var checkMessagesCmd = &cobra.Command{
	Use:   "check-messages",
	Short: "Check for unread messages (for use in hooks)",
	Long: `Check for unread messages for instances running in the current directory.

This command is designed to be used in Claude Code hooks (e.g., idle_prompt).
If unread messages exist, it outputs them to stderr and exits with code 2,
which causes Claude to receive the message content.

Example hook configuration:
{
  "hooks": {
    "Notification": [
      {
        "matcher": "idle_prompt",
        "hooks": [
          {
            "type": "command",
            "command": "clauder check-messages"
          }
        ]
      }
    ]
  }
}`,
	RunE: runCheckMessages,
}

func init() {
	rootCmd.AddCommand(checkMessagesCmd)
}

func runCheckMessages(cmd *cobra.Command, args []string) error {
	dataDir := getDataDir()
	s, err := store.NewSQLiteStore(dataDir)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Find instances in current directory
	instances, err := s.GetInstances()
	if err != nil {
		return fmt.Errorf("failed to get instances: %w", err)
	}

	var localInstances []store.Instance
	for _, inst := range instances {
		if inst.Directory == workDir {
			localInstances = append(localInstances, inst)
		}
	}

	if len(localInstances) == 0 {
		// No instances in this directory, nothing to check
		return nil
	}

	// Check messages for all local instances
	var allMessages []store.Message
	for _, inst := range localInstances {
		messages, err := s.GetMessages(inst.ID, true) // unread only
		if err != nil {
			continue
		}
		allMessages = append(allMessages, messages...)
	}

	if len(allMessages) == 0 {
		// No messages, exit cleanly
		return nil
	}

	// Output messages to stderr (for Claude to receive via exit code 2)
	fmt.Fprintf(os.Stderr, "📬 You have %d unread message(s) from other Claude instances:\n\n", len(allMessages))
	for _, m := range allMessages {
		// Look up sender instance to get directory info
		sender, _ := s.GetInstance(m.FromInstance)
		senderInfo := m.FromInstance
		if len(senderInfo) > 8 {
			senderInfo = senderInfo[:8] + "..."
		}
		if sender != nil {
			shortID := m.FromInstance
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			senderInfo = fmt.Sprintf("%s (in %s)", shortID, sender.Directory)
		}
		fmt.Fprintf(os.Stderr, "From: %s\n", senderInfo)
		fmt.Fprintf(os.Stderr, "Time: %s\n", m.CreatedAt.Format("15:04:05"))
		fmt.Fprintf(os.Stderr, "Message: %s\n\n", m.Content)
	}
	fmt.Fprintf(os.Stderr, "Use `get_messages` tool to mark as read and respond.\n")

	// Exit with code 2 to signal Claude should process this
	os.Exit(2)
	return nil
}
