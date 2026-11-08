package main

import "github.com/spf13/cobra"

// mockProviderFlagHelp is the --mock-provider help shared by run, chat and
// serve. EnableMockProviderMode swaps the LLM provider registry and the
// self-play roles' providers; the TTS and STT registries are separate and keep
// their real providers. Only TTS has a mock type.
const mockProviderFlagHelp = "Replace the LLM and self-play role providers with mocks. " +
	"TTS and STT providers are not replaced; for keyless voices, declare a type: mock, role: tts provider"

// mockProviderModeLog is what serve logs once --mock-provider is in effect.
const mockProviderModeLog = "Mock provider mode: LLM and self-play role providers replaced with mocks; " +
	"TTS and STT providers unchanged"

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Chat interactively with an agent defined in an Arena config",
	RunE:  runChat,
}

func init() {
	chatCmd.Flags().String("config", "config.arena.yaml", "Path to the Arena config file")
	chatCmd.Flags().Bool("mock-provider", false, mockProviderFlagHelp)
	chatCmd.Flags().String("mock-config", "", "Mock response file (used with --mock-provider)")
	chatCmd.Flags().Bool("voice", false, "Hands-free voice mode (requires PortAudio installed)")
	chatCmd.Flags().String("voice-stt", "", "STT provider id for voice over text agents (VAD mode)")
	chatCmd.Flags().String("voice-output-voice", "", "TTS voice id the agent speaks in (VAD mode)")
	chatCmd.Flags().Bool("echo-guard", false,
		"Gate mic while the agent speaks (for laptop speakers; default off assumes headphones)")
	chatCmd.Flags().Bool("barge-in", false,
		"Allow interrupting the agent mid-reply (opt-in; needs headphones/AEC, off by default)")
	chatCmd.Flags().BoolP("verbose", "v", false, "Enable verbose debug logging (raw provider events, transcription)")
	rootCmd.AddCommand(chatCmd)
}
