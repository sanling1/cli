package surface

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// EntryKind identifies the type of surface entry.
type EntryKind string

const (
	KindCmd  EntryKind = "CMD"
	KindFlag EntryKind = "FLAG"
	KindSub  EntryKind = "SUB"
	KindArg  EntryKind = "ARG"
)

// Entry represents a single element in the CLI surface.
type Entry struct {
	Kind     EntryKind
	Path     string // Full command path (e.g., "basecamp projects list")
	Name     string // Flag or subcommand name, or arg name for ARG entries
	FlagType string // Flag type (e.g., "string", "bool") — only for FLAG entries
	Position int    // Zero-based arg position — only for ARG entries
	Required bool   // Whether the arg is required (<name>) vs optional ([name]) — only for ARG entries
	Variadic bool   // Whether the arg accepts multiple values (<name>...) — only for ARG entries
}

// String returns the canonical string representation of the entry.
// Format: "CMD path", "FLAG path --name type=flagtype", "SUB path name",
// "ARG path 00 <name>", "ARG path 01 [name]..."
func (e Entry) String() string {
	switch e.Kind {
	case KindCmd:
		return fmt.Sprintf("CMD %s", e.Path)
	case KindFlag:
		return fmt.Sprintf("FLAG %s --%s type=%s", e.Path, e.Name, e.FlagType)
	case KindSub:
		return fmt.Sprintf("SUB %s %s", e.Path, e.Name)
	case KindArg:
		open, close := byte('<'), byte('>')
		if !e.Required {
			open, close = '[', ']'
		}
		suffix := ""
		if e.Variadic {
			suffix = "..."
		}
		return fmt.Sprintf("ARG %s %02d %c%s%c%s", e.Path, e.Position, open, e.Name, close, suffix)
	default:
		return fmt.Sprintf("%s %s %s", e.Kind, e.Path, e.Name)
	}
}

// Snapshot walks a Cobra command tree and returns all surface entries.
func Snapshot(cmd *cobra.Command) []Entry {
	var entries []Entry
	walkCommand(cmd, cmd.Name(), &entries)
	return entries
}

// SnapshotString returns a sorted, newline-joined string of all surface entries.
func SnapshotString(cmd *cobra.Command) string {
	entries := Snapshot(cmd)
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.String()
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// argPattern matches bracket-delimited tokens in Cobra Use strings:
// <required>, [optional], <name>..., [name]...
//
// Note: this intentionally mirrors the regex in basecamp-cli's internal/cli/args.go.
// It does NOT support arg_schema annotations — only Use-string parsing.
// If a command uses arg_schema, the surface entry will reflect the Use string,
// which may differ from the annotation-based schema.
var argPattern = regexp.MustCompile(`[<\[]([^>\]]+)[>\]](\.\.\.)?`)

// parseArgs extracts positional argument entries from a command's Use string.
// Returns nil for non-runnable commands (pure group commands with no RunE).
func parseArgs(cmd *cobra.Command, path string) []Entry {
	if !cmd.Runnable() {
		return nil
	}

	use := cmd.Use
	use = strings.ReplaceAll(use, " [flags]", "")
	use = strings.ReplaceAll(use, " [action]", "")

	idx := strings.IndexByte(use, ' ')
	if idx < 0 {
		return nil
	}
	use = use[idx+1:]

	matches := argPattern.FindAllStringSubmatch(use, -1)
	if len(matches) == 0 {
		return nil
	}

	args := make([]Entry, 0, len(matches))
	for i, m := range matches {
		name := m[1]
		full := m[0]
		args = append(args, Entry{
			Kind:     KindArg,
			Path:     path,
			Name:     name,
			Position: i,
			Required: full[0] == '<',
			Variadic: m[2] == "...",
		})
	}
	return args
}

func walkCommand(cmd *cobra.Command, path string, entries *[]Entry) {
	// Emit CMD entry
	*entries = append(*entries, Entry{Kind: KindCmd, Path: path})

	// Emit ARG entries from the command's Use string
	*entries = append(*entries, parseArgs(cmd, path)...)

	// Initialize Cobra's lazy-added flags (help, version) that are only
	// set up during Execute() by default. Without this, walking the tree
	// without executing misses these inherited flags.
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()

	// Collect and sort all flags visible at this command level:
	// local flags, persistent flags on this command, and inherited persistent flags.
	var flags []Entry
	seen := make(map[string]bool)
	addFlag := func(f *pflag.Flag) {
		if seen[f.Name] || f.Hidden {
			return
		}
		seen[f.Name] = true
		flags = append(flags, Entry{
			Kind:     KindFlag,
			Path:     path,
			Name:     f.Name,
			FlagType: f.Value.Type(),
		})
	}
	cmd.Flags().VisitAll(addFlag)
	cmd.PersistentFlags().VisitAll(addFlag)
	if cmd.HasParent() {
		cmd.InheritedFlags().VisitAll(addFlag)
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
	*entries = append(*entries, flags...)

	// Collect and sort subcommands
	subs := cmd.Commands()
	sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })

	for _, sub := range subs {
		if sub.Hidden {
			continue
		}

		// Primary name
		*entries = append(*entries, Entry{Kind: KindSub, Path: path, Name: sub.Name()})
		walkCommand(sub, path+" "+sub.Name(), entries)

		// Alias paths — walk the same command tree under each alias name
		for _, alias := range sub.Aliases {
			*entries = append(*entries, Entry{Kind: KindSub, Path: path, Name: alias})
			walkCommand(sub, path+" "+alias, entries)
		}
	}
}
