// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package internal // import "go.opentelemetry.io/collector/exporter/exporterhelper/internal"

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/component"
)

type QueueConsumers[T any] struct {
	queue        Queue[T]
	numConsumers int
	consumeFunc  func(context.Context, T) error
	stopWG       sync.WaitGroup
}

func NewQueueConsumers[T any](q Queue[T], numConsumers int, consumeFunc func(context.Context, T) error) *QueueConsumers[T] {
	return &QueueConsumers[T]{
		queue:        q,
		numConsumers: numConsumers,
		consumeFunc:  consumeFunc,
		stopWG:       sync.WaitGroup{},
	}
}

// Start ensures that queue and all consumers are started.
func (qc *QueueConsumers[T]) Start(ctx context.Context, host component.Host) error {
	if err := qc.queue.Start(ctx, host); err != nil {
		return err
	}

	var startWG sync.WaitGroup
	for i := 0; i < qc.numConsumers; i++ {
		qc.stopWG.Add(1)
		startWG.Add(1)
		go func() {
			startWG.Done()
			defer qc.stopWG.Done()
			for {
				ok := qc.queue.Consume(qc.consumeFunc)
				if !ok {
					return
				}
			}
		}()
	}
	startWG.Wait()

	return nil
}

// Shutdown ensures that queue and all consumers are stopped.
func (qc *QueueConsumers[T]) Shutdown(ctx context.Context) error {
	if err := qc.queue.Shutdown(ctx); err != nil {
		return err
	}
	qc.stopWG.Wait()
	return nil
}
