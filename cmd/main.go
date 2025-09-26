package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"rat/internal/config"
	"rat/internal/discovery"
	"rat/internal/logger"
	"rat/internal/service"
	"rat/internal/transport"
	"rat/internal/ui"
)

func main() {
	// Handle help flag
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help") {
		config.PrintUsage()
		os.Exit(0)
	}
	
	// Create configuration with CLI flags and interactive prompts
	cfg := config.NewCLIConfig()
	
	// Create logger with configured level and file support
	log, err := logger.NewWithFile(
		logger.LogLevel(cfg.GetLogLevel()), 
		cfg.GetQuiet(), 
		cfg.GetLogFile(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	
	// Create services
	discovery := discovery.NewMDNSDiscoveryService(cfg, log)
	transport := transport.NewTCPMessageTransport(cfg, log)
	chatService := service.NewLocalChatService(cfg, log, discovery, transport)
	
	// Create UI model
	model := ui.NewModel(chatService)
	
	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Start chat service in background
	go func() {
		if err := chatService.Start(ctx); err != nil {
			log.Error("Failed to start chat service", "error", err)
			os.Exit(1)
		}
	}()
	
	// Start Bubble Tea program
	p := tea.NewProgram(model, tea.WithAltScreen())
	
	// Set the program reference in the model for event handling
	model.SetProgram(p)
	
	// Handle shutdown gracefully
	go func() {
		<-sigChan
		log.Info("Received shutdown signal")
		
		// Stop the chat service
		if err := chatService.Stop(); err != nil {
			log.Error("Error stopping chat service", "error", err)
		}
		
		// Quit the program
		p.Quit()
	}()
	
	// Run the program
	if err := p.Start(); err != nil {
		log.Error("Error running program", "error", err)
		os.Exit(1)
	}
	
	log.Info("Application shutdown complete")
}

