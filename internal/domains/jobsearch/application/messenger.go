package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
)

type Delivery struct {
	Name      string
	Messenger Messenger
}

type MultiMessenger struct {
	deliveries []Delivery
	logger     *log.Logger
}

func NewMultiMessenger(deliveries []Delivery, logger *log.Logger) *MultiMessenger {
	active := make([]Delivery, 0, len(deliveries))
	for _, delivery := range deliveries {
		if delivery.Messenger != nil {
			active = append(active, delivery)
		}
	}
	return &MultiMessenger{deliveries: active, logger: logger}
}

func (m *MultiMessenger) Send(ctx context.Context, message string) error {
	var wait sync.WaitGroup
	errorsByDelivery := make(chan error, len(m.deliveries))
	for _, delivery := range m.deliveries {
		wait.Add(1)
		go func(delivery Delivery) {
			defer wait.Done()
			if err := delivery.Messenger.Send(ctx, message); err != nil {
				if m.logger != nil {
					m.logger.Printf("jobsearch: %s delivery: %v", delivery.Name, err)
				}
				errorsByDelivery <- fmt.Errorf("%s: %w", delivery.Name, err)
			}
		}(delivery)
	}
	wait.Wait()
	close(errorsByDelivery)
	var deliveryErrors []error
	for err := range errorsByDelivery {
		deliveryErrors = append(deliveryErrors, err)
	}
	return errors.Join(deliveryErrors...)
}
