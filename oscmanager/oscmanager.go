package oscmanager

import (
	"fmt"
	"log"

	"github.com/hypebeast/go-osc/osc"
)

// OSCManager holds the OSC server and a channel for touch events
type OSCManager struct {
	Addr      string
	TouchChan chan float32
	server    *osc.Server
}

// New creates a new OSCManager
func New(addr string, touchChan chan float32) *OSCManager {
	return &OSCManager{
		Addr:      addr,
		TouchChan: touchChan,
	}
}

// Run starts the OSC server and listens for TailTouch messages
func (o *OSCManager) Run() {
	dispatcher := osc.NewStandardDispatcher()

	// Wildcard handler (optional for debugging)
	// dispatcher.AddMsgHandler("*", func(msg *osc.Message) {
	// 	fmt.Printf("Received OSC message: %s, Arguments: %v\n", msg.Address, msg.Arguments)
	// })

	// TailTouch handler
	dispatcher.AddMsgHandler("/avatar/parameters/TailTouch", func(msg *osc.Message) {
		fmt.Printf("Received OSC message: %s\n", msg.Address)
		fmt.Printf("Arguments: %v\n", msg.Arguments)

		if len(msg.Arguments) > 0 {
			if val, ok := msg.Arguments[0].(float32); ok {
				select {
				case o.TouchChan <- val:
					// sent successfully
				default:
					// channel full: remove old value then insert new one
					<-o.TouchChan
					o.TouchChan <- val
				}
			}
		}
	})

	o.server = &osc.Server{
		Addr:       o.Addr,
		Dispatcher: dispatcher,
	}

	log.Printf("Listening for OSC on %s...\n", o.server.Addr)
	if err := o.server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
