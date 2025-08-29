package main

import (
	"os"
	"fmt"

	"github.com/kairos-io/kairos-sdk/bus"
	"github.com/mudler/go-pluggable"
)

func main() {
	factory := pluggable.NewPluginFactory()

	// Register the build-install event handler
	factory.Add(bus.InitProviderInstall, func(e *pluggable.Event) pluggable.EventResponse {
		// Simple test provider that just acknowledges the event
		fmt.Printf("Test provider: received build-install event\n")
		
		// Always respond with success for this test provider
		return pluggable.EventResponse{
			State: bus.EventResponseSuccess,
			Data:  "Test provider installed successfully",
		}
	})

	// Start the plugin factory (this is the correct way for a plugin)
	err := factory.Run(pluggable.EventType(bus.InitProviderInstall), os.Stdin, os.Stdout)
	if err != nil {
		fmt.Printf("Error running plugin factory: %v\n", err)
		os.Exit(1)
	}
}