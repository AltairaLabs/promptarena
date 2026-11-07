package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"

	"github.com/AltairaLabs/promptarena/v2/packc/compiler"

	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

const (
	minArgsForCommand  = 2
	minArgsWithPackArg = 3
)

func main() {
	if len(os.Args) < minArgsForCommand {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "compile":
		compileCommand()
	case "compile-prompt":
		compilePromptCommand()
	case "validate":
		validateCommand()
	case "inspect":
		inspectCommand()
	case "completion":
		completionCommand()
	case "version":
		fmt.Printf("packc %s\n", packcVersion())
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`packc - PromptKit Pack Compiler

Usage:
  packc compile [options]         Compile ALL prompts from arena.yaml into a single pack
  packc compile-prompt [options]  Compile a single prompt to pack format
  packc validate <pack-file>...   Validate one or more pack files
  packc inspect <pack-file>       Display pack information
  packc version                   Show version
  packc help                      Show this help
  packc completion <shell>        Generate shell completion script (bash, zsh, fish, powershell)

Compile Command (main):
  packc compile [-c <arena.yaml>] [-o <pack-file>] [--id <pack-id>] [--pack-version <semver>]

  Options:
    -c, --config        Path to arena.yaml file (default: config.arena.yaml)
    -o, --output        Output pack file path (default: {id}.pack.json)
        --id            Pack ID (default: current folder name, sanitized)
        --pack-version  Pack version, e.g. 1.2.0 (default: v1.0.0)

Compile Prompt Command (single prompt):
  packc compile-prompt -p <yaml-file> -o <json-file> [--pack-version <semver>]

  Options:
    -p, --prompt        Path to prompt YAML file (required)
    -o, --output        Output pack file path (required)
        --pack-version  Pack version (default: the prompt's spec.version)

Examples:
  # Compile using all defaults (config.arena.yaml, folder name as id)
  packc compile

  # Compile with custom config
  packc compile -c arena.yaml

  # Compile with all options specified
  packc compile -c arena.yaml -o packs/customer-support-pack.json --id customer-support --pack-version 1.2.0

  # Compile single prompt
  packc compile-prompt -p prompts/support.yaml -o packs/support.json

  # Validate a pack, or several
  packc validate packs/support.json
  packc validate packs/*.pack.json

  # Inspect a pack
  packc inspect packs/support.json

  # Generate shell completions
  packc completion bash >> ~/.bashrc
  packc completion zsh >> ~/.zshrc`)
}

// compileFlags holds the parsed flags for the compile command.
type compileFlags struct {
	configFile  string
	outputFile  string
	packID      string
	packVersion string
}

// getDefaultPackID returns a default pack ID based on the current directory name.
func getDefaultPackID() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "pack"
	}
	return compiler.SanitizePackID(filepath.Base(cwd))
}

// packVersionUsage is the help text for --pack-version on both compile commands.
const packVersionUsage = "Pack version, a semantic version such as 1.2.0 (default: v1.0.0 for compile, " +
	"the prompt's spec.version for compile-prompt)"

// parseCompileFlags parses and validates compile command flags.
func parseCompileFlags() compileFlags {
	fs := pflag.NewFlagSet("compile", pflag.ExitOnError)
	configFile := fs.StringP("config", "c", "config.arena.yaml", "Path to arena.yaml file")
	outputFile := fs.StringP("output", "o", "", "Output pack file path (default: {id}.pack.json)")
	packID := fs.String("id", "", "Pack ID (default: current folder name, sanitized)")
	packVersion := fs.String("pack-version", "", packVersionUsage)

	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	// Apply smart defaults
	resolvedID := *packID
	if resolvedID == "" {
		resolvedID = getDefaultPackID()
	}

	resolvedOutput := *outputFile
	if resolvedOutput == "" {
		resolvedOutput = resolvedID + ".pack.json"
	}

	return compileFlags{
		configFile:  *configFile,
		outputFile:  resolvedOutput,
		packID:      resolvedID,
		packVersion: *packVersion,
	}
}

func compileCommand() {
	flags := parseCompileFlags()

	opts := []compiler.Option{
		compiler.WithPackID(flags.packID),
		compiler.WithCompilerVersion(compilerVersionString()),
	}
	if flags.packVersion != "" {
		opts = append(opts, compiler.WithPackVersion(flags.packVersion))
	}

	result, err := compiler.Compile(flags.configFile, opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compilation failed: %v\n", err)
		os.Exit(1)
	}

	// Print any warnings
	for _, w := range result.Warnings {
		fmt.Printf("⚠ %s\n", w)
	}

	// Write the output file
	if err := writePackFile(flags.outputFile, result.JSON); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write pack file: %v\n", err)
		os.Exit(1)
	}

	printPackSummary(result.Pack, flags.outputFile)
}

// compilerVersionString is the compilation.compiled_with value packc stamps.
func compilerVersionString() string {
	return "packc-" + packcVersion()
}

func compilePromptCommand() {
	fs := pflag.NewFlagSet("compile-prompt", pflag.ExitOnError)
	promptFile := fs.StringP("prompt", "p", "", "Path to prompt YAML file")
	outputFile := fs.StringP("output", "o", "", "Output pack file path")
	packVersion := fs.String("pack-version", "", packVersionUsage)

	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	if *promptFile == "" || *outputFile == "" {
		fmt.Fprintln(os.Stderr, "Error: --prompt and --output are required")
		fmt.Fprintln(os.Stderr, "Usage: packc compile-prompt -p <yaml-file> -o <json-file>")
		fs.PrintDefaults()
		os.Exit(1)
	}

	opts := []compiler.Option{compiler.WithCompilerVersion(compilerVersionString())}
	if *packVersion != "" {
		opts = append(opts, compiler.WithPackVersion(*packVersion))
	}

	fmt.Printf("Compiling prompt %s to pack...\n", *promptFile)
	result := compiler.CompilePromptFile(*promptFile, opts...)
	for _, w := range result.Warnings {
		fmt.Printf("⚠ %s\n", w)
	}
	if err := result.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Compilation failed: %v\n", err)
		os.Exit(1)
	}

	if err := writePackFile(*outputFile, result.JSON); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write pack file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Pack compiled successfully: %s\n", *outputFile)
}

// packFileArgs returns the pack-file arguments to validate or inspect, or nil
// after printing usage when they asked for help.
func packFileArgs(usage string) []string {
	args := os.Args[2:]
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println(usage)
			return nil
		}
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: pack file path required")
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}
	return args
}

const validateUsage = `Usage: packc validate <pack-file> [<pack-file>...]

Validates each pack file against the PromptPack schema and checks its structure.
Exits 1 if any pack has an error.`

const inspectUsage = `Usage: packc inspect <pack-file>

Displays a pack's prompts, tools, evals, skills, workflow, agents and metadata.`

func validateCommand() {
	packFiles := packFileArgs(validateUsage)
	if packFiles == nil {
		return
	}

	failed := 0
	for i, packFile := range packFiles {
		if i > 0 {
			fmt.Println()
		}
		if !validatePackFile(packFile) {
			failed++
		}
	}

	if len(packFiles) > 1 {
		fmt.Printf("\n%d of %d packs valid\n", len(packFiles)-failed, len(packFiles))
	}
	if failed > 0 {
		os.Exit(1)
	}
}

// validatePackFile validates one pack and prints the result, reporting whether
// it is valid.
func validatePackFile(packFile string) bool {
	fmt.Printf("Validating pack: %s\n", packFile)

	result := compiler.ValidatePackFile(packFile)

	schemaFailed := false
	for _, p := range result.Errors {
		if p.Stage == compiler.StagePackSchema {
			schemaFailed = true
		}
	}
	if result.Pack != nil && !schemaFailed {
		fmt.Println("✓ Schema validation passed")
	}

	for _, w := range result.Warnings {
		fmt.Printf("⚠ %s\n", w)
	}

	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "✗ Pack validation failed: %s\n", packFile)
		for _, p := range result.Errors {
			fmt.Fprintf(os.Stderr, warningFormat, p)
		}
		return false
	}

	fmt.Println("✓ Pack structure is valid")
	return true
}

func inspectCommand() {
	packFiles := packFileArgs(inspectUsage)
	if packFiles == nil {
		return
	}
	if len(packFiles) > 1 {
		fmt.Fprintln(os.Stderr, "Error: inspect takes one pack file")
		fmt.Fprintln(os.Stderr, inspectUsage)
		os.Exit(1)
	}
	packFile := packFiles[0]

	pack, err := prompt.LoadPack(packFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading pack: %v\n", err)
		os.Exit(1)
	}
	if pack.ID == "" && len(pack.Prompts) == 0 {
		fmt.Fprintf(os.Stderr, "Error: %s is not a PromptPack (no id and no prompts)\n", packFile)
		os.Exit(1)
	}

	printPackInfo(pack)
	printTemplateEngine(pack)
	printPrompts(pack)
	printTools(pack)
	printFragments(pack)
	printEvals(pack)
	printSkills(pack)
	printWorkflow(pack)
	printCompositions(pack)
	printAgents(pack)
	printMetadata(pack)
	printCompilationInfo(pack)

	fmt.Println()
}

func completionCommand() {
	if len(os.Args) < minArgsWithPackArg {
		fmt.Fprintln(os.Stderr, "Error: shell type required (bash, zsh, fish, powershell)")
		fmt.Fprintln(os.Stderr, "Usage: packc completion <shell>")
		os.Exit(1)
	}

	shell := os.Args[2]

	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	case "powershell":
		fmt.Print(powershellCompletion)
	default:
		fmt.Fprintf(os.Stderr, "Unknown shell: %s\n", shell)
		fmt.Fprintln(os.Stderr, "Supported shells: bash, zsh, fish, powershell")
		os.Exit(1)
	}
}

const bashCompletion = `# packc bash completion
_packc_completions() {
    local cur prev commands
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    commands="compile compile-prompt validate inspect version help completion"

    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
        return 0
    fi

    case "${prev}" in
        compile)
            COMPREPLY=($(compgen -W "-c --config -o --output --id --pack-version" -- "${cur}"))
            ;;
        compile-prompt)
            COMPREPLY=($(compgen -W "-p --prompt -o --output --pack-version" -- "${cur}"))
            ;;
        -c|--config)
            COMPREPLY=($(compgen -f -X '!*.yaml' -- "${cur}"))
            ;;
        -o|--output)
            COMPREPLY=($(compgen -f -X '!*.json' -- "${cur}"))
            ;;
        -p|--prompt)
            COMPREPLY=($(compgen -f -X '!*.yaml' -- "${cur}"))
            ;;
        validate|inspect)
            COMPREPLY=($(compgen -f -X '!*.json' -- "${cur}"))
            ;;
        completion)
            COMPREPLY=($(compgen -W "bash zsh fish powershell" -- "${cur}"))
            ;;
    esac
}
complete -F _packc_completions packc
`

const zshCompletion = `#compdef packc

_packc() {
    local -a commands
    commands=(
        'compile:Compile ALL prompts from arena.yaml into a single pack'
        'compile-prompt:Compile a single prompt to pack format'
        'validate:Validate a pack file'
        'inspect:Display pack information'
        'version:Show version'
        'help:Show help'
        'completion:Generate shell completion script'
    )

    _arguments -C \
        '1: :->command' \
        '*: :->args'

    case $state in
        command)
            _describe 'command' commands
            ;;
        args)
            case $words[2] in
                compile)
                    _arguments \
                        '-c[Path to arena.yaml file]:config file:_files -g "*.yaml"' \
                        '--config[Path to arena.yaml file]:config file:_files -g "*.yaml"' \
                        '-o[Output pack file path]:output file:_files -g "*.json"' \
                        '--output[Output pack file path]:output file:_files -g "*.json"' \
                        '--id[Pack ID]:pack id:' \
                        '--pack-version[Pack version]:pack version:'
                    ;;
                compile-prompt)
                    _arguments \
                        '-p[Path to prompt YAML file]:prompt file:_files -g "*.yaml"' \
                        '--prompt[Path to prompt YAML file]:prompt file:_files -g "*.yaml"' \
                        '-o[Output pack file path]:output file:_files -g "*.json"' \
                        '--output[Output pack file path]:output file:_files -g "*.json"' \
                        '--pack-version[Pack version]:pack version:'
                    ;;
                validate|inspect)
                    _arguments '*:pack file:_files -g "*.json"'
                    ;;
                completion)
                    _arguments '1:shell:(bash zsh fish powershell)'
                    ;;
            esac
            ;;
    esac
}

_packc "$@"
`

const fishCompletion = `# packc fish completion
complete -c packc -f

# Commands
complete -c packc -n '__fish_use_subcommand' -a 'compile' -d 'Compile ALL prompts from arena.yaml into a single pack'
complete -c packc -n '__fish_use_subcommand' -a 'compile-prompt' -d 'Compile a single prompt to pack format'
complete -c packc -n '__fish_use_subcommand' -a 'validate' -d 'Validate a pack file'
complete -c packc -n '__fish_use_subcommand' -a 'inspect' -d 'Display pack information'
complete -c packc -n '__fish_use_subcommand' -a 'version' -d 'Show version'
complete -c packc -n '__fish_use_subcommand' -a 'help' -d 'Show help'
complete -c packc -n '__fish_use_subcommand' -a 'completion' -d 'Generate shell completion script'

# compile options
complete -c packc -n '__fish_seen_subcommand_from compile' -s c -l config -d 'Path to arena.yaml file' -r -F
complete -c packc -n '__fish_seen_subcommand_from compile' -s o -l output -d 'Output pack file path' -r -F
complete -c packc -n '__fish_seen_subcommand_from compile' -l id -d 'Pack ID' -r
complete -c packc -n '__fish_seen_subcommand_from compile' -l pack-version -d 'Pack version' -r

# compile-prompt options
complete -c packc -n '__fish_seen_subcommand_from compile-prompt' -s p -l prompt -d 'Path to prompt YAML file' -r -F
complete -c packc -n '__fish_seen_subcommand_from compile-prompt' -s o -l output -d 'Output pack file path' -r -F
complete -c packc -n '__fish_seen_subcommand_from compile-prompt' -l pack-version -d 'Pack version' -r

# completion shells
complete -c packc -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish powershell'
`

const powershellCompletion = `# packc PowerShell completion
Register-ArgumentCompleter -Native -CommandName packc -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $commands = @{
        'compile' = 'Compile ALL prompts from arena.yaml into a single pack'
        'compile-prompt' = 'Compile a single prompt to pack format'
        'validate' = 'Validate a pack file'
        'inspect' = 'Display pack information'
        'version' = 'Show version'
        'help' = 'Show help'
        'completion' = 'Generate shell completion script'
    }

    $elements = $commandAst.CommandElements
    $command = $null
    if ($elements.Count -gt 1) {
        $command = $elements[1].Extent.Text
    }

    if ($elements.Count -eq 1 -or ($elements.Count -eq 2 -and $wordToComplete -eq $command)) {
        $commands.Keys | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $commands[$_])
        }
        return
    }

    switch ($command) {
        'compile' {
            $opts = @('-c', '--config', '-o', '--output', '--id', '--pack-version')
            $opts | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterName', $_)
            }
        }
        'compile-prompt' {
            $opts = @('-p', '--prompt', '-o', '--output', '--pack-version')
            $opts | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterName', $_)
            }
        }
        'completion' {
            @('bash', 'zsh', 'fish', 'powershell') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
    }
}
`
