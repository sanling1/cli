package surface

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func noop(_ *cobra.Command, _ []string) error { return nil }

func newTestRoot() *cobra.Command {
	root := &cobra.Command{
		Use: "mycli",
	}
	root.PersistentFlags().Bool("json", false, "JSON output")
	root.PersistentFlags().Bool("verbose", false, "Verbose output")

	projects := &cobra.Command{Use: "projects", Short: "Manage projects", Aliases: []string{"project"}}
	projects.Flags().Int("limit", 50, "Limit results")

	list := &cobra.Command{Use: "list", Short: "List projects"}
	list.Flags().Bool("all", false, "Show all")

	show := &cobra.Command{Use: "show <id>", Short: "Show project", RunE: noop}
	show.Flags().String("format", "table", "Output format")

	create := &cobra.Command{Use: "create <name> [description]", Short: "Create project", RunE: noop}

	projects.AddCommand(list, show, create)

	assign := &cobra.Command{Use: "assign <id|url>...", Short: "Assign", RunE: noop}

	root.AddCommand(projects, assign)

	hidden := &cobra.Command{Use: "internal", Hidden: true}
	root.AddCommand(hidden)

	return root
}

func TestSnapshot(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	// Should have entries for all visible commands
	var cmds []string
	for _, e := range entries {
		if e.Kind == KindCmd {
			cmds = append(cmds, e.Path)
		}
	}

	assert.Contains(t, cmds, "mycli")
	assert.Contains(t, cmds, "mycli projects")
	assert.Contains(t, cmds, "mycli projects list")
	assert.Contains(t, cmds, "mycli projects show")
	assert.Contains(t, cmds, "mycli projects create")
}

func TestSnapshotFlags(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	var flags []string
	for _, e := range entries {
		if e.Kind == KindFlag {
			flags = append(flags, e.String())
		}
	}

	assert.Contains(t, flags, "FLAG mycli --json type=bool")
	assert.Contains(t, flags, "FLAG mycli --verbose type=bool")
	assert.Contains(t, flags, "FLAG mycli projects --limit type=int")
	assert.Contains(t, flags, "FLAG mycli projects list --all type=bool")
}

func TestSnapshotSubcommands(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	var subs []string
	for _, e := range entries {
		if e.Kind == KindSub {
			subs = append(subs, e.String())
		}
	}

	assert.Contains(t, subs, "SUB mycli projects")
	assert.Contains(t, subs, "SUB mycli projects list")
	assert.Contains(t, subs, "SUB mycli projects show")
	assert.Contains(t, subs, "SUB mycli projects create")
}

func TestSnapshotHiddenExcluded(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	for _, e := range entries {
		assert.NotContains(t, e.Path, "internal", "hidden commands should be excluded")
	}
}

func TestSnapshotString(t *testing.T) {
	root := newTestRoot()
	s := SnapshotString(root)

	assert.NotEmpty(t, s)

	// Should be sorted
	lines := splitLines(s)
	for i := 1; i < len(lines); i++ {
		assert.True(t, lines[i-1] <= lines[i], "lines should be sorted: %q > %q", lines[i-1], lines[i])
	}
}

func TestSnapshotArgs(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	var args []string
	for _, e := range entries {
		if e.Kind == KindArg {
			args = append(args, e.String())
		}
	}

	// show <id> — single required arg
	assert.Contains(t, args, "ARG mycli projects show 00 <id>")

	// create <name> [description] — required + optional
	assert.Contains(t, args, "ARG mycli projects create 00 <name>")
	assert.Contains(t, args, "ARG mycli projects create 01 [description]")
}

func TestSnapshotArgVariadic(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	var args []string
	for _, e := range entries {
		if e.Kind == KindArg {
			args = append(args, e.String())
		}
	}

	// assign <id|url>... — required variadic
	assert.Contains(t, args, "ARG mycli assign 00 <id|url>...")
}

func TestSnapshotArgNonRunnable(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	// "projects" is a group command (no RunE) — should have no ARG entries
	for _, e := range entries {
		if e.Kind == KindArg && e.Path == "mycli projects" {
			t.Errorf("non-runnable group command should not emit ARG entries, got: %s", e.String())
		}
	}
}

func TestSnapshotAliases(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	var cmds, subs []string
	for _, e := range entries {
		switch e.Kind {
		case KindCmd:
			cmds = append(cmds, e.Path)
		case KindSub:
			subs = append(subs, e.String())
		}
	}

	// "project" is an alias for "projects"
	assert.Contains(t, cmds, "mycli project")
	assert.Contains(t, cmds, "mycli project show")
	assert.Contains(t, cmds, "mycli project create")
	assert.Contains(t, subs, "SUB mycli project")
	assert.Contains(t, subs, "SUB mycli project show")
}

func TestSnapshotAliasArgs(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	var args []string
	for _, e := range entries {
		if e.Kind == KindArg {
			args = append(args, e.String())
		}
	}

	// ARG entries under the alias path must match the primary path
	assert.Contains(t, args, "ARG mycli project show 00 <id>")
	assert.Contains(t, args, "ARG mycli project create 00 <name>")
	assert.Contains(t, args, "ARG mycli project create 01 [description]")
}

func TestSnapshotAliasFlags(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	var flags []string
	for _, e := range entries {
		if e.Kind == KindFlag {
			flags = append(flags, e.String())
		}
	}

	// Inherited flags appear under alias paths too
	assert.Contains(t, flags, "FLAG mycli project --json type=bool")
	assert.Contains(t, flags, "FLAG mycli project --limit type=int")
	assert.Contains(t, flags, "FLAG mycli project show --format type=string")
}

func TestSnapshotArgMalformedOptional(t *testing.T) {
	// Use: "attach <file> [<file2> ...]" has a nested bracket pattern.
	// The regex captures <file2 as the name inside the outer [...] wrapper,
	// producing [<file2] — this matches the shell script's output.
	// The bug is in the Use string, not the parser. We preserve for parity.
	root := &cobra.Command{Use: "mycli"}
	attach := &cobra.Command{Use: "attach <file> [<file2> ...]", RunE: noop}
	root.AddCommand(attach)

	entries := Snapshot(root)
	var args []string
	for _, e := range entries {
		if e.Kind == KindArg {
			args = append(args, e.String())
		}
	}

	assert.Contains(t, args, "ARG mycli attach 00 <file>")
	assert.Contains(t, args, "ARG mycli attach 01 [<file2]")
}

func TestDiffIdentical(t *testing.T) {
	root := newTestRoot()
	entries := Snapshot(root)

	result := Diff(entries, entries)
	assert.Empty(t, result.Added)
	assert.Empty(t, result.Removed)
	assert.False(t, result.HasBreakingChanges())
}

func TestDiffAdditions(t *testing.T) {
	root1 := newTestRoot()
	old := Snapshot(root1)

	root2 := newTestRoot()
	root2.AddCommand(&cobra.Command{Use: "newcmd", Short: "New command"})
	new := Snapshot(root2)

	result := Diff(old, new)
	assert.NotEmpty(t, result.Added)
	assert.Empty(t, result.Removed)
	assert.False(t, result.HasBreakingChanges())

	// Check specific addition
	var addedCmds []string
	for _, e := range result.Added {
		if e.Kind == KindCmd {
			addedCmds = append(addedCmds, e.Path)
		}
	}
	assert.Contains(t, addedCmds, "mycli newcmd")
}

func TestDiffRemovals(t *testing.T) {
	root1 := newTestRoot()
	root1.AddCommand(&cobra.Command{Use: "oldcmd"})
	old := Snapshot(root1)

	root2 := newTestRoot()
	new := Snapshot(root2)

	result := Diff(old, new)
	assert.NotEmpty(t, result.Removed)
	assert.True(t, result.HasBreakingChanges())
}

func TestEntryString(t *testing.T) {
	tests := []struct {
		entry    Entry
		expected string
	}{
		{Entry{Kind: KindCmd, Path: "mycli"}, "CMD mycli"},
		{Entry{Kind: KindFlag, Path: "mycli", Name: "json", FlagType: "bool"}, "FLAG mycli --json type=bool"},
		{Entry{Kind: KindSub, Path: "mycli", Name: "projects"}, "SUB mycli projects"},
		{Entry{Kind: KindArg, Path: "mycli projects create", Name: "name", Position: 0, Required: true}, "ARG mycli projects create 00 <name>"},
		{Entry{Kind: KindArg, Path: "mycli projects create", Name: "description", Position: 1, Required: false}, "ARG mycli projects create 01 [description]"},
		{Entry{Kind: KindArg, Path: "mycli assign", Name: "id|url", Position: 0, Required: true, Variadic: true}, "ARG mycli assign 00 <id|url>..."},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.entry.String())
		})
	}
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
