// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package connector

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/internal/testdata"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

type mutatingMetricsSink struct {
	*consumertest.MetricsSink
}

func (mts *mutatingMetricsSink) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func TestMetricsRouterMultiplexing(t *testing.T) {
	var max = 20
	for numIDs := 1; numIDs < max; numIDs++ {
		for numCons := 1; numCons < max; numCons++ {
			for numMetrics := 1; numMetrics < max; numMetrics++ {
				t.Run(
					fmt.Sprintf("%d-ids/%d-cons/%d-logs", numIDs, numCons, numMetrics),
					fuzzMetrics(numIDs, numCons, numMetrics),
				)
			}
		}
	}
}

func fuzzMetrics(numIDs, numCons, numMetrics int) func(*testing.T) {
	return func(t *testing.T) {
		allIDs := make([]component.ID, 0, numCons)
		allCons := make([]consumer.Metrics, 0, numCons)
		allConsMap := make(map[component.ID]consumer.Metrics)

		// If any consumer is mutating, the router must report mutating
		for i := 0; i < numCons; i++ {
			allIDs = append(allIDs, component.NewIDWithName("sink", strconv.Itoa(numCons)))
			// Random chance for each consumer to be mutating
			if (numCons+numMetrics+i)%4 == 0 {
				allCons = append(allCons, &mutatingMetricsSink{MetricsSink: new(consumertest.MetricsSink)})
			} else {
				allCons = append(allCons, new(consumertest.MetricsSink))
			}
			allConsMap[allIDs[i]] = allCons[i]
		}

		r := NewMetricsRouter(allConsMap)
		md := testdata.GenerateMetrics(1)

		// Keep track of how many logs each consumer should receive.
		// This will be validated after every call to RouteMetrics.
		expected := make(map[component.ID]int, numCons)

		for i := 0; i < numMetrics; i++ {
			// Build a random set of ids (no duplicates)
			randCons := make(map[component.ID]bool, numIDs)
			for j := 0; j < numIDs; j++ {
				// This number should be pretty random and less than numCons
				conNum := (numCons + numIDs + i + j) % numCons
				randCons[allIDs[conNum]] = true
			}

			// Convert to slice, update expectations
			conIDs := make([]component.ID, 0, len(randCons))
			for id := range randCons {
				conIDs = append(conIDs, id)
				expected[id]++
			}

			// Route to list of consumers
			fanout, err := r.Consumer(conIDs...)
			assert.NoError(t, err)
			assert.NoError(t, fanout.ConsumeMetrics(context.Background(), md))

			// Validate expectations for all consumers
			for id := range expected {
				metrics := []pmetric.Metrics{}
				switch con := allConsMap[id].(type) {
				case *consumertest.MetricsSink:
					metrics = con.AllMetrics()
				case *mutatingMetricsSink:
					metrics = con.AllMetrics()
				}
				assert.Len(t, metrics, expected[id])
				for n := 0; n < len(metrics); n++ {
					assert.EqualValues(t, md, metrics[n])
				}
			}
		}
	}
}

func TestMetricsRouterConsumers(t *testing.T) {
	ctx := context.Background()
	md := testdata.GenerateMetrics(1)

	fooID := component.NewID("foo")
	barID := component.NewID("bar")

	foo := new(consumertest.MetricsSink)
	bar := new(consumertest.MetricsSink)
	r := NewMetricsRouter(map[component.ID]consumer.Metrics{fooID: foo, barID: bar})

	rcs := r.PipelineIDs()
	assert.Len(t, rcs, 2)
	assert.ElementsMatch(t, []component.ID{fooID, barID}, rcs)

	assert.Len(t, foo.AllMetrics(), 0)
	assert.Len(t, bar.AllMetrics(), 0)

	both, err := r.Consumer(fooID, barID)
	assert.NotNil(t, both)
	assert.NoError(t, err)

	assert.NoError(t, both.ConsumeMetrics(ctx, md))
	assert.Len(t, foo.AllMetrics(), 1)
	assert.Len(t, bar.AllMetrics(), 1)

	fooOnly, err := r.Consumer(fooID)
	assert.NotNil(t, fooOnly)
	assert.NoError(t, err)

	assert.NoError(t, fooOnly.ConsumeMetrics(ctx, md))
	assert.Len(t, foo.AllMetrics(), 2)
	assert.Len(t, bar.AllMetrics(), 1)

	barOnly, err := r.Consumer(barID)
	assert.NotNil(t, barOnly)
	assert.NoError(t, err)

	assert.NoError(t, barOnly.ConsumeMetrics(ctx, md))
	assert.Len(t, foo.AllMetrics(), 2)
	assert.Len(t, bar.AllMetrics(), 2)

	none, err := r.Consumer()
	assert.Nil(t, none)
	assert.Error(t, err)

	fake, err := r.Consumer(component.NewID("fake"))
	assert.Nil(t, fake)
	assert.Error(t, err)
}
