package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kairos-io/kairos-sdk/bus"
	"github.com/mudler/go-pluggable"
)

func main() {
	factory := pluggable.NewPluginFactory()

	// Register the build-install event handler
	factory.Add(bus.InitProviderInstall, func(e *pluggable.Event) pluggable.EventResponse {
		fmt.Printf("Test provider: received build-install event with payload: %s\n", e.Data)
		
		return pluggable.EventResponse{
			State: bus.EventResponseSuccess,
			Data:  "Test provider installed successfully",
		}
	})

	// Register the info event handler to respond with version info
	factory.Add(bus.InitProviderInfo, func(e *pluggable.Event) pluggable.EventResponse {
		fmt.Printf("Test provider: received info event with payload: %s\n", e.Data)
		
		// Create version info payload
		versionInfo := bus.ProviderInstalledVersionPayload{
			Provider: "test-provider",
			Version:  "v1.0.0-test",
		}
		
		data, err := json.Marshal(versionInfo)
		if err != nil {
			return pluggable.EventResponse{
				State: bus.EventResponseNotApplicable,
				Error: fmt.Sprintf("Failed to marshal version info: %v", err),
			}
		}
		
		return pluggable.EventResponse{
			State: bus.EventResponseSuccess,
			Data:  string(data),
		}
	})

	// The plugin framework will call this based on stdin input
	factory.Run(pluggable.EventType(""), os.Stdin, os.Stdout)
}