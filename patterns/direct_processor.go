package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"solace.dev/go/messaging"
	"solace.dev/go/messaging/pkg/solace"
	"solace.dev/go/messaging/pkg/solace/config"
	"solace.dev/go/messaging/pkg/solace/message"
	"solace.dev/go/messaging/pkg/solace/resource"
)

// Message Handler
func MessageHandler(message message.InboundMessage) {
	var message_body string
	if payload, ok := message.GetPayloadAsString(); ok {
		message_body = payload
	} else if payload, ok := message.GetPayloadAsBytes(); ok {
		message_body = string(payload)
	}

	received_topic := message.GetDestinationName()
	// Generate processed topic
	slice := strings.Split(received_topic, "/")
	processed_topic := strings.Join(slice[:len(slice)-1][:], "/") + "/output"

	// Process the message
	// For example, change the body of the message to uppercased
	processed_msg := strings.ToUpper(message_body)
	fmt.Printf("Received a message: %s on topic %s\n", message_body, received_topic)
	fmt.Printf("Uppercasing to %s and publishing on %s", processed_msg, processed_topic)

	// Publish the message here
}

func ReconnectionHandler(e solace.ServiceEvent) {
	e.GetTimestamp()
	e.GetBrokerURI()
	err := e.GetCause()
	if err != nil {
		fmt.Println(err)
	}
}

func getEnv(key, def string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return def
}

func main() {

	// Define Topic Prefix
	TOPIC_PREFIX := "solace/samples"

	// Configuration parameters
	brokerConfig := config.ServicePropertyMap{
		config.TransportLayerPropertyHost:                getEnv("SOLACE_HOST", "tcp://localhost:55555,tcp://localhost:55554"),
		config.ServicePropertyVPNName:                    getEnv("SOLACE_VPN", "default"),
		config.AuthenticationPropertySchemeBasicPassword: getEnv("SOLACE_PASSWORD", "default"),
		config.AuthenticationPropertySchemeBasicUserName: getEnv("SOLACE_USERNAME", "default"),
	}

	messagingService, err := messaging.NewMessagingServiceBuilder().FromConfigurationProvider(brokerConfig).Build()

	if err != nil {
		panic(err)
	}

	messagingService.AddReconnectionListener(ReconnectionHandler)

	// Connect to the messaging serice
	if err := messagingService.Connect(); err != nil {
		panic(err)
	}

	fmt.Println("Connected to the broker? ", messagingService.IsConnected())

	// Define Topic Subscriptions
	subscription_topic := resource.TopicSubscriptionOf(TOPIC_PREFIX + "/direct/processor/input")

	// Build a Direct message receivers with given topics
	directReceiver, err := messagingService.CreateDirectMessageReceiverBuilder().
		WithSubscriptions(subscription_topic).
		Build()

	fmt.Println("Subscribed to: ", subscription_topic.GetName())

	if err != nil {
		panic(err)
	}

	// Start Direct Message Receiver
	if err := directReceiver.Start(); err != nil {
		panic(err)
	}

	fmt.Println("Direct Receiver running? ", directReceiver.IsRunning())

	// Build a direct publish to use for publishing the processed message
	directPublisher, builderErr := messagingService.CreateDirectMessagePublisherBuilder().Build()
	if builderErr != nil {
		panic(builderErr)
	}

	startErr := directPublisher.Start()
	if startErr != nil {
		panic(startErr)
	}
	messageBuilder := messagingService.MessageBuilder()

	// Message Handler
	var messageHandler solace.MessageHandler = func(message message.InboundMessage) {
		var message_body string
		if payload, ok := message.GetPayloadAsString(); ok {
			message_body = payload
		} else if payload, ok := message.GetPayloadAsBytes(); ok {
			message_body = string(payload)
		}

		received_topic := message.GetDestinationName()
		// Generate processed topic
		slice := strings.Split(received_topic, "/")
		processed_topic := strings.Join(slice[:len(slice)-1][:], "/") + "/output"

		// Process the message
		// For example, change the body of the message to uppercased
		processed_msg := strings.ToUpper(message_body)
		fmt.Printf("Received a message: %s on topic %s\n", message_body, received_topic)
		fmt.Printf("Uppercasing to %s and publishing on %s\n\n", processed_msg, processed_topic)

		out_message, err := messageBuilder.BuildWithStringPayload(processed_msg)
		if err != nil {
			panic(err)
		}

		// Publish on dynamic topic with dynamic body
		publishErr := directPublisher.Publish(out_message, resource.TopicOf(processed_topic))
		if publishErr != nil {
			panic(publishErr)
		}
	}

	// Register Message callback handler to the Message Receiver
	if regErr := directReceiver.ReceiveAsync(messageHandler); regErr != nil {
		panic(regErr)
	}

	fmt.Println("\n===Interrupt (CTR+C) to handle graceful terminaltion of the subscriber===\n")

	// cleanup after the main calling function has finished execution
	defer func() {
		// Terminate the Direct Receiver
		directReceiver.Terminate(1 * time.Second)
		fmt.Println("\nDirect Receiver Terminated? ", directReceiver.IsTerminated())
		// Disconnect the Message Service
		messagingService.Disconnect()
		fmt.Println("Messaging Service Disconnected? ", !messagingService.IsConnected())
	}()

	// Run forever until an interrupt signal is received
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	// Block until a interrupt signal is received.
	<-c
}
