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

package synth

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/entooone/simple-midi-synth/internal/time"
	"io"
	"sort"
	"strconv"
)

type noteValue struct {
	offset   float32
	velocity int
}

type noteEvent struct {
	velocity int
	delta    uint
	note     bool
}

type progression struct {
	note      string
	time      float32
	amplitude float32
	offset    float32
}

// MIDIToWAV convert MIDI into WAV
func MIDIToWAV(reader io.Reader) (*bytes.Buffer, error) {
	midiStream, err := newMIDIStream(reader)
	if err != nil {
		return nil, err
	}
	header := midiStream.readChunk()

	if header.id != "MThd" || header.length != 6 {
		return nil, errors.New("invalid header")
	}

	headerStream, err := newMIDIStream(bytes.NewReader(header.data))
	if err != nil {
		return nil, err
	}
	headerStream.readUint16() // format type
	trackCount := int(headerStream.readUint16())
	timeDivision := int(headerStream.readUint16())
	tracks := make([][]*midiEvent, 0)
	prog := make([]*progression, 0)
	events := make([]*noteEvent, 0)
	var maxAmplitude float32
	for i := 0; i < trackCount; i++ {
		trackChunk := midiStream.readChunk()

		if trackChunk.id != "MTrk" {
			continue
		}

		trackStream, err := newMIDIStream(bytes.NewReader(trackChunk.data))
		if err != nil {
			return nil, err
		}
		track := make([]*midiEvent, 0)
		keep := true

		for keep && trackStream.byteOffset < trackChunk.length {
			event := trackStream.readEvent()
			track = append(track, event)
		}

		if keep {
			tracks = append(tracks, track)
		}
	}

	if (timeDivision >> 15) == 0 {
		timer := time.NewTimer(timeDivision)

		// set up timer with setTempo events
		for i, delta := 0, 0; i < len(tracks[0]); i++ {
			event := tracks[0][i]
			delta += int(event.delta)

			if event.subType == "setTempo" {
				v, _ := strconv.Atoi(event.value["value"])
				timer.AddCriticalPoint(delta, v)
				delta = 0
			}
		}

		// generate note data
		for i := 0; i < len(tracks); i++ {
			track := tracks[i]
			var delta uint
			m := make(map[int][]*noteValue)

			for j := 0; j < len(track); j++ {
				event := track[j]
				delta += event.delta

				if event.eventType == "channel" {
					semitone, _ := strconv.Atoi(event.value["noteNumber"])

					if event.subType == "noteOn" {
						v, _ := strconv.Atoi(event.value["velocity"])
						note := &noteValue{
							velocity: v,
							offset:   timer.Time(int(delta)),
						}

						// use stack for simultaneous identical notes
						if _, ok := m[semitone]; ok {
							m[semitone] = append(m[semitone], note)
						} else {
							m[semitone] = []*noteValue{note}
						}

						// to determine maximum total velocity for normalizing volume
						events = append(events, &noteEvent{
							velocity: note.velocity,
							delta:    delta,
							note:     true,
						})
					} else if event.subType == "noteOff" {
						if _, ok := m[semitone]; !ok {
							return nil, fmt.Errorf("invalid semitone (%d)", semitone)
						}
						note := m[semitone][len(m[semitone])-1]
						m[semitone] = m[semitone][:len(m[semitone])-1]
						n, _ := noteFromSemitone(semitone)
						prog = append(prog, &progression{
							note:      n,
							time:      timer.Time(int(delta)) - note.offset,
							amplitude: float32(note.velocity) / 128,
							offset:    note.offset,
						})

						events = append(events, &noteEvent{
							velocity: note.velocity,
							delta:    delta,
							note:     false,
						})
					}
				}
			}
		}

		sort.Slice(events, func(i, j int) bool {
			return (events[i].delta < events[j].delta) || ((events[i].delta == events[j].delta) && ((events[i].note != events[j].note) && events[j].note))
		})

		var (
			maxVelocity = 1
			velocity    = 1
			maxChord    = 0
			chord       = 0
		)

		for _, event := range events {
			if event.note {
				velocity += event.velocity
				chord++

				if velocity > maxVelocity {
					maxVelocity = velocity
				}

				if chord > maxChord {
					maxChord = chord
				}
			} else {
				velocity -= event.velocity
				chord--
			}
		}

		// scaling factor for amplitude
		maxAmplitude = 128 / float32(maxVelocity)
	} else {
		// use frames per second
		// not yet implemented

		return nil, errors.New("unsupported format")
	}

	wav, _ := newWAV(1, 44100, 16, true, make([]byte, 0))
	if err != nil {
		return nil, err
	}

	wav.writeProgression(prog, maxAmplitude, []int{0}, true, true, 1)

	return wav.toBuffer(), nil
}
