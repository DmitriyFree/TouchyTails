package oscmanager

import (
	"fmt"
	"log"
	"strings"

	"github.com/hypebeast/go-osc/osc"
)

type OSCMessage struct {
	Name  string
	Value float32
}

// OSCManager holds the OSC server and a channel for touch events
type OSCManager struct {
	Addr    string
	oscChan chan OSCMessage
	server  *osc.Server
}

// New creates a new OSCManager
func New(addr string, oscChan chan OSCMessage) *OSCManager {
	return &OSCManager{
		Addr:    addr,
		oscChan: oscChan,
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

		prefix := "/avatar/parameters/"
		name := strings.TrimPrefix(msg.Address, prefix)

		if len(msg.Arguments) > 0 {
			if val, ok := msg.Arguments[0].(float32); ok {
				select {
				case o.oscChan <- OSCMessage{
					Name:  name,
					Value: val,
				}:
					// sent successfully
				default:
					// channel full: remove old value then insert new one
					<-o.oscChan
					o.oscChan <- OSCMessage{
						Name:  name,
						Value: val,
					}
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
