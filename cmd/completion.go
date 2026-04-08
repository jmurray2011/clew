package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for clew.

To load completions:

Bash:
  $ source <(clew completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ clew completion bash > /etc/bash_completion.d/clew
  # macOS:
  $ clew completion bash > $(brew --prefix)/etc/bash_completion.d/clew

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ clew completion zsh > "${fpath[1]}/_clew"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ clew completion fish | source

  # To load completions for each session, execute once:
  $ clew completion fish > ~/.config/fish/completions/clew.fish

PowerShell:
  PS> clew completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> clew completion powershell > clew.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
