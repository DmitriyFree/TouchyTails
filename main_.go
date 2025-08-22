package main

import (
	"fmt"
	"log"

	"github.com/hypebeast/go-osc/osc"
)

func main() {
	// Create a dispatcher (router for OSC addresses)
	dispatcher := osc.NewStandardDispatcher()

	// Add handler for our avatar parameter ///avatar/parameters/TouchedTail
	/*dispatcher.AddMsgHandler("*", func(msg *osc.Message) {
		fmt.Printf("Received OSC message: %s\n", msg.Address)
		//fmt.Printf("Arguments: %v\n", msg.Arguments)
	})*/
	// Add handler for our avatar parameter //
	dispatcher.AddMsgHandler("/avatar/parameters/TailTouch", func(msg *osc.Message) {
		fmt.Printf("Received OSC message: %s\n", msg.Address)
		fmt.Printf("Arguments: %v\n", msg.Arguments)
	})

	// Start server
	server := &osc.Server{
		Addr:       "127.0.0.1:9001",
		Dispatcher: dispatcher,
	}

	log.Printf("Listening for OSC on %s...\n", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
