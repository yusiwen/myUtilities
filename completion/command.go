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

_mu_commands() {
	mu --help 2>/dev/null | sed -n '/^Commands:/,$p' | grep '  ' | awk '{print $1}' | grep -v '^$'
}

_mu_flags() {
	local cmd=$1
	mu "$cmd" --help 2>/dev/null | sed -n '/^Flags:/,$p' | grep -Eo -- '--[a-z0-9-]+' | sort -u
}

_mu_completion() {
	local cur prev words cword
	_init_completion || return

	COMPREPLY=()

	if [[ $cword -eq 1 ]]; then
		COMPREPLY=($(compgen -W "$(_mu_commands)" -- "$cur"))
		return
	fi

	if [[ $cword -ge 2 ]]; then
		local cmd="${words[1]}"
		if [[ -n $cmd ]]; then
			COMPREPLY=($(compgen -W "$(_mu_flags "$cmd")" -- "$cur"))
		fi
	fi
}

complete -F _mu_completion mu
`

const zshScript = `#compdef mu
# zsh completion for mu — source it to enable:
#   source <(mu completion zsh)

_mu_commands() {
	mu --help 2>/dev/null | sed -n '/^Commands:/,$p' | grep '  ' | awk '{print $1}' | grep -v '^$'
}

_mu_flags() {
	local cmd=$1
	mu "$cmd" --help 2>/dev/null | sed -n '/^Flags:/,$p' | grep -Eo -- '--[a-z0-9-]+' | sort -u
}

_mu() {
	local -a commands flags
	commands=($(_mu_commands))

	if (( CURRENT == 2 )); then
		_describe -t commands 'mu command' commands
		return
	fi

	local cmd=${words[2]}
	if [[ -n $cmd ]]; then
		flags=($(_mu_flags "$cmd"))
		_describe -t flags 'flag' flags
	fi
}

compdef _mu mu

_mu "$@"
`
