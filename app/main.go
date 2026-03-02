package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	var (
		demo       = flag.Bool("demo", false, "Run in demo mode")
		configPath = flag.String("config", "", "Path to config file")
		provider   = flag.String("provider", "", "LLM provider (openai, openrouter)")
		model      = flag.String("model", "", "Model name")
		apiKey     = flag.String("api-key", "", "API key for the provider")
	)
	flag.Parse()

	app, err := Bootstrap(BootstrapConfig{
		Demo:       *demo,
		ConfigPath: *configPath,
		Provider:   *provider,
		Model:      *model,
		APIKey:     *apiKey,
	})
	if err != nil {
		log.Fatalf("Failed to bootstrap: %v", err)
	}

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
