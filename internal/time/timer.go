// Copyright 2020 entooone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package time

type criticalPoint struct {
	delta               int
	microsecondsPerBeat int
}

// Timer calculate time from delta ticks when MIDI file has several "setTempo" events
type Timer struct {
	ticksPerBeat   int
	criticalPoints []criticalPoint
}

// NewTimer has delta to represent ticks since last time change
func NewTimer(ticksPerBeat int) *Timer {
	return &Timer{
		ticksPerBeat:   ticksPerBeat,
		criticalPoints: make([]criticalPoint, 0),
	}
}

const (
	microsecondsPerSecond = 1000000

	// midi standard initializes file with this value
	microsecondsPerBeatDefault = 500000
)

// AddCriticalPoint add criticalPoint to timer
func (t *Timer) AddCriticalPoint(delta, microsecondsPerBeat int) {
	t.criticalPoints = append(t.criticalPoints, criticalPoint{
		delta:               delta,
		microsecondsPerBeat: microsecondsPerBeat,
	})
}

// Time gets time from timer
func (t *Timer) Time(delta int) float32 {
	var time float32
	microsecondsPerBeat := microsecondsPerBeatDefault
	var cp criticalPoint

	// iterate through time changes while decrementing delta ticks to 0
	for i := 0; i < len(t.criticalPoints) && delta > 0; i++ {
		cp = t.criticalPoints[i]

		// incrementally calculate the time passed for each range of timing
		if delta >= cp.delta {
			time += float32(cp.delta*microsecondsPerBeat) / float32(t.ticksPerBeat) / microsecondsPerSecond
			delta -= cp.delta
		} else {
			time += float32(delta*microsecondsPerBeat) / float32(t.ticksPerBeat) / microsecondsPerSecond
			delta = 0
		}

		microsecondsPerBeat = cp.microsecondsPerBeat
	}

	time += float32(delta*microsecondsPerBeat) / float32(t.ticksPerBeat) / microsecondsPerSecond

	return time
}
