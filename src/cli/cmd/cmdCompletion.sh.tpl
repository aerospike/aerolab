_aerolab() {
    # All arguments except the first one
	%s
    args=("${COMP_WORDS[@]:1:$COMP_CWORD}")

    # Only split on newlines
    local IFS=$'\n'

    # Call completion (note that the first element of COMP_WORDS is
    # the executable itself)
    COMPREPLY=($(GO_FLAGS_COMPLETION=1 ${COMP_WORDS[0]} "${args[@]}"))
    return 0
}

%s
complete -F _aerolab aerolab
complete -F _aerolab aerolab-macos
complete -F _aerolab aerolab-linux
