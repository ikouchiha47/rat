package config

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/user"
	"strings"

	"rat/internal/interfaces"
)

// CLIConfig implements the Configuration interface with CLI flag support
type CLIConfig struct {
	username    string
	room        string
	port        int
	logLevel    string
	serviceName string
	serviceType string
	verbose     bool
	logFile     string
}

// NewCLIConfig creates a new configuration from CLI flags and interactive prompts
func NewCLIConfig() interfaces.Configuration {
	cfg := &CLIConfig{
		room:        "general",
		port:        0, // 0 means random available port
		logLevel:    "INFO",
		serviceName: "LocalChat",
		serviceType: "_localchat._tcp",
	}

	// Parse CLI flags
	cfg.parseFlags()

	// If username not provided via flags, prompt interactively
	if cfg.username == "" {
		cfg.username = cfg.promptUsername()
	}

	// Load from environment variables (as fallback)
	cfg.loadFromEnv()
	
	// Auto-generate log file name if verbose mode is disabled but no file specified
	if !cfg.verbose && cfg.logFile == "" {
		cfg.logFile = fmt.Sprintf("localchat_%s.log", cfg.username)
	}

	return cfg
}

// parseFlags parses command line flags
func (c *CLIConfig) parseFlags() {
	flag.StringVar(&c.username, "username", "", "Your username (required)")
	flag.StringVar(&c.username, "u", "", "Your username (shorthand)")
	flag.StringVar(&c.room, "room", "general", "Chat room to join")
	flag.StringVar(&c.room, "r", "general", "Chat room to join (shorthand)")
	flag.IntVar(&c.port, "port", 0, "Port to listen on (0 = random)")
	flag.StringVar(&c.logLevel, "log", "INFO", "Log level (DEBUG, INFO, WARN, ERROR)")
	flag.StringVar(&c.serviceName, "service", "LocalChat", "mDNS service name")
	flag.BoolVar(&c.verbose, "verbose", false, "Verbose mode - logs to terminal instead of file")
	flag.BoolVar(&c.verbose, "v", false, "Verbose mode (shorthand)")
	flag.StringVar(&c.logFile, "logfile", "", "Log file path (auto-generated if not specified)")
	flag.Parse()
}

// promptUsername prompts the user for a username interactively
func (c *CLIConfig) promptUsername() string {
	fmt.Println("🚀 Local Chat Application")
	fmt.Println("========================")
	fmt.Println()

	// Get default username
	defaultUsername := getSystemUsername()
	
	// Create reader for stdin
	reader := bufio.NewReader(os.Stdin)
	
	for {
		fmt.Printf("Enter your username [%s]: ", defaultUsername)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}
		
		username := strings.TrimSpace(input)
		if username == "" {
			username = defaultUsername
		}
		
		// Validate username
		if err := c.validateUsername(username); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid username: %v\n", err)
			continue
		}
		
		return username
	}
}

// validateUsername validates the username
func (c *CLIConfig) validateUsername(username string) error {
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	
	if len(username) > 20 {
		return fmt.Errorf("username too long (max 20 characters)")
	}
	
	if strings.ContainsAny(username, " \t\n\r") {
		return fmt.Errorf("username cannot contain spaces or special characters")
	}
	
	return nil
}

// loadFromEnv loads configuration from environment variables (as fallback)
func (c *CLIConfig) loadFromEnv() {
	// Only load if not already set via CLI flags
	if c.username == "" {
		if username := os.Getenv("CHAT_USERNAME"); username != "" {
			c.username = username
		}
	}
	
	if c.room == "general" {
		if room := os.Getenv("CHAT_ROOM"); room != "" {
			c.room = room
		}
	}
	
	if c.port == 0 {
		if portStr := os.Getenv("CHAT_PORT"); portStr != "" {
			// Parse port from environment
			// Note: Port parsing is handled by CLI flags, env vars are fallback
		}
	}
	
	if c.logLevel == "INFO" {
		if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
			c.logLevel = logLevel
		}
	}
	
	if c.serviceName == "LocalChat" {
		if serviceName := os.Getenv("CHAT_SERVICE_NAME"); serviceName != "" {
			c.serviceName = serviceName
		}
	}
}

// GetUsername returns the username
func (c *CLIConfig) GetUsername() string {
	return c.username
}

// GetRoom returns the room name
func (c *CLIConfig) GetRoom() string {
	return c.room
}

// GetPort returns the port number
func (c *CLIConfig) GetPort() int {
	return c.port
}

// GetLogLevel returns the log level
func (c *CLIConfig) GetLogLevel() string {
	return c.logLevel
}

// GetServiceName returns the service name for mDNS
func (c *CLIConfig) GetServiceName() string {
	return c.serviceName
}

// GetServiceType returns the service type for mDNS
func (c *CLIConfig) GetServiceType() string {
	return c.serviceType
}

// GetQuiet returns whether quiet mode is enabled (inverse of verbose)
func (c *CLIConfig) GetQuiet() bool {
	return !c.verbose
}

// GetLogFile returns the log file path
func (c *CLIConfig) GetLogFile() string {
	return c.logFile
}

// getSystemUsername returns the system username or a default
func getSystemUsername() string {
	if currentUser, err := user.Current(); err == nil {
		return currentUser.Username
	}
	
	if username := os.Getenv("USER"); username != "" {
		return username
	}
	
	if username := os.Getenv("USERNAME"); username != "" {
		return username
	}
	
	return "user"
}

// PrintUsage prints usage information
func PrintUsage() {
	fmt.Println("Local Chat Application")
	fmt.Println("Usage: localchat [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -username, -u string    Your username (required if not provided interactively)")
	fmt.Println("  -room, -r string        Chat room to join (default: general)")
	fmt.Println("  -port int               Port to listen on (0 = random available)")
	fmt.Println("  -log string             Log level: DEBUG, INFO, WARN, ERROR (default: INFO)")
	fmt.Println("  -service string         mDNS service name (default: LocalChat)")
	fmt.Println("  -verbose, -v            Verbose mode - logs to terminal instead of file")
	fmt.Println("  -logfile string         Log file path (auto-generated if not specified)")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  CHAT_USERNAME           Your username")
	fmt.Println("  CHAT_ROOM               Chat room to join")
	fmt.Println("  CHAT_PORT               Port to listen on")
	fmt.Println("  LOG_LEVEL               Log level")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  localchat -username alice -room dev")
	fmt.Println("  localchat -u bob -r gaming")
	fmt.Println("  localchat -u alice                      # Logs to localchat_alice.log (default)")
	fmt.Println("  localchat -u alice --verbose            # Logs to terminal")
	fmt.Println("  localchat") // Will prompt for username
}
