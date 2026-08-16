# completion — Generate shell completion script

Print a bash/zsh completion script for `mu` subcommands and flags. The scripts
dynamically parse `mu <path> --help` output, so they stay accurate as commands change.

```bash
# bash — add to ~/.bashrc
source <(mu completion bash)

# zsh — add to ~/.zshrc
source <(mu completion zsh)
```
