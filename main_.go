
package main

import (
	"fmt"
	"log"

	"github.com/hypebeast/go-osc/osc"
)

func main() {
	// Create a dispatcher (router for OSC addresses)
	dispatcher := osc.NewStandardDispatcher()

	// Add handler for our avatar parameter
	dispatcher.AddMsgHandler("/avatar/parameters/TouchedTail", func(msg *osc.Message) {
		if len(msg.Arguments) > 0 {
			if val, ok := msg.Arguments[0].(float32); ok {
				if val > 0.5 {
					fmt.Println("Touch detected! Trigger haptics ON")
				} else {
					fmt.Println("Touch released! Haptics OFF")
				}
			}
		}
	})

	// Start server
	server := &osc.Server{
		Addr:       "127.0.0.1:9000",
		Dispatcher: dispatcher,
	}

	log.Printf("Listening for OSC on %s...\n", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
