package cmd

import (
	"os"

	"github.com/parsabordbar/ctx3/mcp"

	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run ctx3 as an MCP server (stdio) exposing its analysis as live tools",
	Long: `Serve ctx3's read-only code-analysis facts over the Model Context Protocol so
a coding agent can query them live, mid-task, instead of reading frozen skill
snapshots. Speaks newline-delimited JSON-RPC 2.0 over stdin/stdout.

Tools exposed, all read-only:
  ctx3_context    project overview: counts, deps, languages, README preview
  ctx3_map        symbol index: where a symbol is defined, what a package exports
  ctx3_functions  detailed signatures: receiver, params, results, doc line
  ctx3_deps       internal import graph and circular imports
  ctx3_flow       call graph and entry points
  ctx3_impact     blast radius of changing a symbol
  ctx3_db         detected databases and reconstructed schema
  ctx3_tree       directory tree, depth-limited
  ctx3_pack       one subtree packed as a document (tree + file contents)
  ctx3_git        branch, uncommitted work, recent commits, hot files

Results default to a compact text rendering and are truncated past maxBytes, so
a query against a large repo returns something usable rather than flooding the
agent's context.

Register with Claude Code:
  claude mcp add ctx3 -- ctx3 mcp

Or add to .mcp.json:
  {"mcpServers": {"ctx3": {"command": "ctx3", "args": ["mcp"]}}}

The server runs until stdin closes; it is meant to be launched by an MCP client,
not run interactively.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcp.NewServer(os.Stdin, os.Stdout).Serve()
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
