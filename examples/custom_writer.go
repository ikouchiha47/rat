package main

import (
	"fmt"
	"os"

	"rat/internal/models"
	"rat/internal/service"
)

// Example of how to use custom message writers

func main() {
	// Example 1: File writer
	file, err := os.Create("evicted_messages.jsonl")
	if err != nil {
		panic(err)
	}
	
	fileWriter := service.NewFileWriter(file)
	
	// Example 2: Custom database writer
	dbWriter := &DatabaseWriter{connectionString: "postgres://..."}
	
	// Example 3: Custom logging writer
	logWriter := &LogWriter{}
	
	fmt.Println("File writer:", fileWriter)
	fmt.Println("DB writer:", dbWriter)
	fmt.Println("Log writer:", logWriter)
	
	// In your chat service setup:
	// chatService.SetMessageWriter(fileWriter)  // Write evicted messages to file
	// chatService.SetMessageWriter(dbWriter)    // Write evicted messages to database
	// chatService.SetMessageWriter(logWriter)   // Write evicted messages to logs
	// chatService.SetMessageWriter(nil)         // Default: write to /dev/null (oblivion)
}

// DatabaseWriter example - writes messages to a database
type DatabaseWriter struct {
	connectionString string
}

func (w *DatabaseWriter) WriteMessage(message *models.Message) error {
	// Implement database write logic here
	fmt.Printf("Writing to DB: %s from %s\n", message.Content, message.From)
	return nil
}

func (w *DatabaseWriter) Close() error {
	// Close database connection
	return nil
}

// LogWriter example - writes messages to application logs
type LogWriter struct{}

func (w *LogWriter) WriteMessage(message *models.Message) error {
	// Write to application logs
	fmt.Printf("LOG: Evicted message: %s from %s at %s\n", 
		message.Content, message.From, message.Timestamp.Format("15:04:05"))
	return nil
}

func (w *LogWriter) Close() error {
	return nil
}
