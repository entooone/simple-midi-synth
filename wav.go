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
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"regexp"
)

func semitoneFromNote(note string) (int, error) {
	// matches occurrence of A through G
	// followed by positive or negative integer
	// followed by 0 to 2 occurrences of flat or sharp
	const re = `^([A-G])(\-?\d+)(b{0,2}|#{0,2})$`

	// if semitone is unrecognized, assume REST
	matched, _ := regexp.MatchString(re, note)
	if !matched {
		return 0, errors.New("invalid note")
	}

	// parse substrings of note
	s := regexp.MustCompile(re).FindAllStringSubmatch(note, 1)[0]
	tone, octave, accidental := s[1], s[2], s[3]

	var (
		tones = map[string]int{
			"C": 0, "D": 2, "E": 4, "F": 5, "G": 7, "A": 9, "B": 11,
		}
		octaves = map[string]int{
			"-1": 0, "0": 1, "1": 2, "2": 3, "3": 4, "4": 5, "5": 6, "6": 7, "7": 8, "8": 9, "9": 10, "10": 11,
		}
		accidentals = map[string]int{
			"bb": -2, "b": -1, "": 0, "#": 1, "##": 2,
		}
	)

	if _, ok := tones[tone]; !ok {
		return 0, errors.New("invalid tone in note")
	}

	if _, ok := octaves[octave]; !ok {
		return 0, errors.New("invalid octave in note")
	}

	if _, ok := accidentals[accidental]; !ok {
		return 0, errors.New("invalid accidental in note")
	}

	return tones[tone] + octaves[octave]*12 + accidentals[accidental], nil
}

func noteFromSemitone(semitone int) (string, error) {
	var (
		octaves = []int{
			-1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		}
		tones = []string{
			"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B",
		}
	)

	octaveIndex := int(math.Floor(float64(semitone) / 12))
	toneIndex := semitone - octaveIndex*12

	if octaveIndex >= len(octaves) {
		return "REST", errors.New("invalid octave")
	}

	if toneIndex >= len(tones) {
		return "REST", errors.New("invalid tone")
	}

	tone := []rune(tones[toneIndex])
	octave := octaves[octaveIndex]

	// tone followed by octave followed by accidental
	if len(tone) == 1 {
		return fmt.Sprintf("%c%d", tone[0], octave), nil
	}
	return fmt.Sprintf("%c%d%c", tone[0], octave, tone[1]), nil
}

// frequencyFromSemitone converts semitone index to frequency in Hz
func frequencyFromSemitone(semitone int) float32 {
	// A4 is 440 Hz, 12 semitones per octave
	return float32(440 * math.Pow(2, float64(semitone-69)/12))
}

type wavData struct {
	header        []byte
	data          []float32
	pointer       uint
	numChannels   uint16
	sampleRate    uint32
	bitsPerSample int
	chunkSize     uint32
	subChunk2Size uint32
}

func newWAV(numChannels uint16, sampleRate uint32, bitsPerSample int, littleEndian bool, data []byte) (*wavData, error) {
	if !littleEndian {
		return nil, errors.New("big endian is not supported")
	}

	// WAV header is always 44 bytes
	header := []byte{
		0x52, 0x49, 0x46, 0x46, // chunk id ("RIFF")
		0x00, 0x00, 0x00, 0x00, // chunk size
		0x57, 0x41, 0x56, 0x45, // format ("WAVE")
		0x66, 0x6D, 0x74, 0x20, // subchunk1 id ("fmt")
		0x10, 0x00, 0x00, 0x00, // subchunk1 size
		0x01, 0x00, // audio format
		0x01, 0x00, // num channels
		0x44, 0xAC, 0x00, 0x00, // sample rate
		0x88, 0x58, 0x01, 0x00, // byte rate
		0x02, 0x00, // block align
		0x10, 0x00, // bits per sample
		0x64, 0x61, 0x74, 0x61, // subchunk2 id ("data")
		0x00, 0x00, 0x00, 0x00, // subchunk2 size
	}

	return &wavData{
		header:        header,
		data:          nil,
		pointer:       0,
		numChannels:   numChannels,
		sampleRate:    sampleRate,
		bitsPerSample: bitsPerSample,
	}, nil
}

// seek sets time (in seconds) of pointer zero-fills by default
func (w *wavData) seek(time float32, fill bool) {
	sample := int(math.Round(float64(w.sampleRate) * float64(time)))

	w.pointer = uint(w.numChannels) * uint(sample)

	// if fill {
	// 	// zero-fill seek
	// 	for uint(len(w.data)) < w.pointer {
	// 		w.data = append(w.data, 0)
	// 	}
	// } else {
	// 	w.pointer = uint(len(w.data))
	// }
}

// writeNote writes the specified note to the sound data
// for amount of time in seconds
// at given normalized amplitude
// to channels listed (or all by default)
// adds to existing data by default
// and does not reset write index after operation by default
func (w *wavData) writeNote(note string, time float32, amplitude float32, channels []int, blend bool, reset bool, relativeDuration int) {
	var (
		numChannels = w.numChannels
		sampleRate  = w.sampleRate

		// to prevent sound artifacts
		fadeSeconds float32 = 0.001

		// calculating properties of given note
		semitone, _ = semitoneFromNote(note)
		frequency   = float32(frequencyFromSemitone(semitone)) * math.Pi * 2 / float32(sampleRate)

		// amount of blocks to be written
		blocksOut = int(math.Round(float64(sampleRate) * float64(time)))
		// reduces sound artifacts by fading at last fadeSeconds
		nonZero = float32(blocksOut) - float32(sampleRate)*fadeSeconds
		// fade interval in samples
		fade = float32(sampleRate)*fadeSeconds + 1

		// index of start and stop samples
		start = int(w.pointer)
		stop  = len(w.data)

		// determines amount of blocks to be updated
		// blocksIn = minInt(int(math.Floor(float64(stop-start)/float64(numChannels))), blocksOut)

		// k = cached index of data
		// d = sample data value
		k int
		d float32
	)

	// by default write to all channels
	if len(channels) == 0 {
		for i := 0; i < int(numChannels); i++ {
			channels = append(channels, i)
		}
	}

	skipChannels := make([]bool, numChannels)
	for i := 0; i < len(skipChannels); i++ {
		skipChannels[i] = channels[i] == -1
	}

	// update existing data
	for i := 0; i < blocksOut; i++ {
		// iterate through specified channels
		for j := 0; j < len(channels); j++ {
			k = start + i*int(numChannels) + channels[j]
			d = 0

			if frequency > 0 {
				d = amplitude * float32(math.Sin(float64(frequency)*float64(i)))
				if float32(i) < fade {
					d *= float32(i) / fade
				} else if float32(i) > nonZero {
					d *= float32(blocksOut-i+1) / fade
				}
			}

			if blend {
				w.data[k] = d + w.data[k]
			} else {
				w.data[k] = d
			}
		}
	}

	// append data
	// for i := blocksIn; i < blocksOut; i++ {
	// 	// iterate through all channels
	// 	for j := 0; j < int(numChannels); j++ {
	// 		d = 0

	// 		// only write non-zero data to specified channels
	// 		if frequency > 0 || !skipChannels[j] {
	// 			d = amplitude * float32(math.Sin(float64(frequency)*float64(i)))
	// 			if float32(i) < fade {
	// 				d *= float32(i) / fade
	// 			} else if float32(i) > nonZero {
	// 				d *= float32(blocksOut-i+1) / fade
	// 			}
	// 		}

	// 		w.data = append(w.data, d)
	// 	}
	// }

	end := maxInt(start+blocksOut*int(numChannels), stop) * (w.bitsPerSample >> 3)
	w.chunkSize = uint32(end + len(w.header) - 8)
	w.subChunk2Size = uint32(end)

	binary.LittleEndian.PutUint32(w.header[4:8], w.chunkSize)
	binary.LittleEndian.PutUint32(w.header[40:44], w.subChunk2Size)

	if !reset {
		w.pointer = uint(start + blocksOut*int(numChannels))
	}
}

// writeProgression adds specified notes in series
// (or asynchronously if offset property is specified in a note)
// each playing for time * relativeDuration seconds
// followed by a time * (1 - relativeDuration) second rest
func (w *wavData) writeProgression(notes []*progression, amplitude float32, channels []int, blend bool, reset bool, relativeDuration int) {
	start := w.pointer

	var max uint
	for i := 0; i < len(notes); i++ {
		var (
			time = notes[i].time
			off  = notes[i].offset
		)
		sample := int(math.Round(float64(w.sampleRate) * float64(off+time)))
		val := uint(w.numChannels) * uint(sample+1)

		if max < val {
			max = val
		}
	}
	w.data = make([]float32, max)

	for i := 0; i < len(notes); i++ {
		var (
			note = notes[i].note
			time = notes[i].time
			amp  = notes[i].amplitude
			off  = notes[i].offset
		)

		// for asynchronous progression
		w.seek(off, true)

		w.writeNote(note, time, amp*amplitude, channels, blend, false, 1)
	}

	if reset {
		w.pointer = start
	}
}

func (w *wavData) typeData() *bytes.Buffer {
	bytesPerSample := w.bitsPerSample >> 3
	size := w.subChunk2Size
	samples := int(size) / bytesPerSample
	buf := make([]byte, size)

	// convert signed normalized sound data to typed integer data
	// i.e. [-1, 1] -> [INT_MIN, INT_MAX]
	amplitude := float32(math.Pow(2, float64(w.bitsPerSample-1)) - 1)

	switch bytesPerSample {
	case 1:
		for i := 0; i < samples; i++ {
			buf[i*2] = uint8(w.data[i]*amplitude+0x80) & 0xff
		}
	case 2:
		for i := 0; i < samples; i++ {
			// [INT16_MIN, INT16_MAX] -> [0, UINT16_MAX]
			d := uint16(w.data[i]*amplitude+0x10000) & 0xffff

			// unwrap inner loop
			buf[i*2] = uint8(d & 0xff)
			buf[i*2+1] = uint8(d >> 8)
		}
	case 3:
		for i := 0; i < samples; i++ {
			d := uint32(w.data[i]*amplitude+0x1000000) & 0xFFFFFF
			buf[i*3] = uint8(d & 0xff)
			buf[i*3+1] = uint8((d >> 8) & 0xff)
			buf[i*3+2] = uint8(d >> 16)
		}
	case 4:
		for i := 0; i < samples; i++ {
			d := uint32(w.data[i]*amplitude+0x100000000) & 0xFFFFFFFF
			buf[i*4] = uint8(d & 0xff)
			buf[i*4+1] = uint8((d >> 8) & 0xff)
			buf[i*4+2] = uint8((d >> 16) & 0xff)
			buf[i*4+3] = uint8(d >> 24)
		}
	}

	return bytes.NewBuffer(buf)
}

func (w *wavData) toBuffer() *bytes.Buffer {
	data := w.typeData().Bytes()
	buf := make([]byte, 0, len(w.header)+len(data))
	buf = append(buf, w.header...)
	buf = append(buf, data...)
	return bytes.NewBuffer(buf)
}
