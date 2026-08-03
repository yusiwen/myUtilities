package completion

import (
	"fmt"
)

type Options struct {
	Shell string `arg:"" help:"Target shell." enum:"bash,zsh"`
}

func (o *Options) Run() error {
	switch o.Shell {
	case "bash":
		fmt.Print(bashScript)
	case "zsh":
		fmt.Print(zshScript)
	}
	return nil
}

const bashScript = `# bash completion for mu — put this in ~/.bashrc or source it:
#   source <(mu completion bash)

_mu_subcommands() {
	local path="$1" help_output in_commands=false line name
	help_output=$(mu $path --help 2>/dev/null)
	[[ -z "$help_output" ]] && return 1

	while IFS= read -r line; do
		if ! $in_commands; then
			[[ "$line" = "Commands:" ]] && in_commands=true
			continue
		fi
		[[ -z "$line" ]] && break
		[[ "$line" != "  "* ]] && break
		line="${line##  }"
		name="${line%%  *}"
		[[ "$name" = "$line" ]] && continue
		[[ -z "$name" ]] && continue
		if [[ -n "$path" ]]; then
			name="${name#$path }"
		fi
		[[ -n "$name" ]] && echo "$name"
	done <<< "$help_output"
}

_mu_flags() {
	local path="$1" help_output in_flags=false
	local flag_re='--[a-z0-9-]+'
	help_output=$(mu $path --help 2>/dev/null)
	[[ -z "$help_output" ]] && return 1

	while IFS= read -r line; do
		if ! $in_flags; then
			[[ "$line" = "Flags:" ]] && in_flags=true
			continue
		fi
		[[ -z "$line" ]] && break
		if [[ "$line" =~ $flag_re ]]; then
			echo "${BASH_REMATCH[0]}"
		fi
	done <<< "$help_output"
}

_mu_completion() {
	local cur prev words cword
	_init_completion || return

	local cmd_path=""
	local i=1
	while [[ $i -lt $cword ]]; do
		if [[ "${words[$i]}" != -* ]]; then
			cmd_path="${cmd_path} ${words[$i]}"
		fi
		((i++))
	done
	cmd_path="${cmd_path# }"

	if [[ "$cur" == -* ]]; then
		COMPREPLY=($(compgen -W "$(_mu_flags "$cmd_path")" -- "$cur"))
		return
	fi

	local subcommands
	subcommands=$(_mu_subcommands "$cmd_path")
	if [[ -n "$subcommands" ]]; then
		COMPREPLY=($(compgen -W "$subcommands" -- "$cur"))
		return
	fi

	COMPREPLY=($(compgen -W "$(_mu_flags "$cmd_path")" -- "$cur"))
}

complete -F _mu_completion mu
`

const zshScript = `#compdef mu
# zsh completion for mu — source it to enable:
#   source <(mu completion zsh)

_mu_subcommands() {
	local cmd="$1" in_commands=false line name
	for line in ${(@f)"$(mu ${=cmd} --help 2>/dev/null)"}; do
		if ! $in_commands; then
			[[ "$line" = "Commands:" ]] && in_commands=true
			continue
		fi
		[[ -z "$line" ]] && break
		[[ "$line" != "  "* ]] && break
		line="${line##  }"
		name="${line%%  *}"
		[[ "$name" = "$line" ]] && continue
		[[ -z "$name" ]] && continue
		if [[ -n "$cmd" ]]; then
			name="${name#$cmd }"
		fi
		[[ -n "$name" ]] && echo "$name"
	done
}

_mu_flags() {
	local cmd="$1" in_flags=false line
	local flag_re='--[a-z0-9-]+'
	for line in ${(@f)"$(mu ${=cmd} --help 2>/dev/null)"}; do
		if ! $in_flags; then
			[[ "$line" = "Flags:" ]] && in_flags=true
			continue
		fi
		[[ -z "$line" ]] && break
		if [[ "$line" =~ $flag_re ]]; then
			echo "$MATCH"
		fi
	done
}

_mu() {
	local cmd_path=""
	for ((i = 2; i < CURRENT; i++)); do
		if [[ "${words[$i]}" != -* ]]; then
			cmd_path="${cmd_path} ${words[$i]}"
		fi
	done
	cmd_path="${cmd_path# }"

	local -a subcommands
	subcommands=($(_mu_subcommands "$cmd_path"))

	if (( ${#subcommands} > 0 )); then
		_describe -t commands 'subcommand' subcommands
		return
	fi

	local -a flags
	flags=($(_mu_flags "$cmd_path"))
	if (( ${#flags} > 0 )); then
		_describe -t flags 'flag' flags
	fi
}

compdef _mu mu
`
