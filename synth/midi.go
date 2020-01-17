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
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
)

type midiStream struct {
	data              []byte
	byteOffset        int
	lastEventTypeByte byte
}

func newMIDIStream(reader io.Reader) (*midiStream, error) {
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return &midiStream{
		data:              data,
		byteOffset:        0,
		lastEventTypeByte: 0x00,
	}, nil
}

func (m *midiStream) readString(byteLength int) string {
	byteOffset := m.byteOffset

	var str string
	for i := 0; i < byteLength; i++ {
		str += fmt.Sprintf("%c", m.data[byteOffset+i])
	}
	m.byteOffset += byteLength
	return str
}

func (m *midiStream) readUint32() uint32 {
	byteOffset := m.byteOffset

	value := (uint32(m.data[byteOffset]) << 24) |
		(uint32(m.data[byteOffset+1]) << 16) |
		(uint32(m.data[byteOffset+2]) << 8) |
		uint32(m.data[byteOffset+3])

	m.byteOffset += 4

	return value
}

func (m *midiStream) readUint24() uint32 {
	byteOffset := m.byteOffset

	value := (uint32(m.data[byteOffset]) << 16) |
		(uint32(m.data[byteOffset+1]) << 8) |
		uint32(m.data[byteOffset+2])

	m.byteOffset += 3

	return value
}

func (m *midiStream) readUint16() uint16 {
	byteOffset := m.byteOffset

	value := (uint16(m.data[byteOffset]) << 8) |
		uint16(m.data[byteOffset+1])

	m.byteOffset += 2

	return value
}

func (m *midiStream) readUint8() uint8 {
	byteOffset := m.byteOffset

	value := m.data[byteOffset]

	m.byteOffset++

	return value
}

func (m *midiStream) readVarUint() uint {
	var (
		value uint
		ui8   byte
	)
	ui8 = m.readUint8()
	value = (value << 7) + (uint(ui8) & 0x7f)
	for (ui8 & 0x80) == 0x80 {
		ui8 = m.readUint8()
		value = (value << 7) + (uint(ui8) & 0x7f)
	}

	return value
}

func (m *midiStream) skip(byteLength int) {
	m.byteOffset += byteLength
}

type midiChunk struct {
	id     string
	length int
	data   []byte
}

func (m *midiStream) readChunk() *midiChunk {
	id := m.readString(4)
	length := int(m.readUint32())
	byteOffset := m.byteOffset

	m.byteOffset += length

	data := m.data[byteOffset:m.byteOffset]

	return &midiChunk{
		id:     id,
		length: length,
		data:   data,
	}
}

type midiEvent struct {
	delta     uint
	eventType string
	subType   string
	value     map[string]string
	channel   byte
}

func (m *midiStream) readEvent() *midiEvent {
	delta := m.readVarUint()
	eventTypeByte := m.readUint8()
	var (
		eventType string
		subType   string
		channel   byte
		value     = make(map[string]string)
	)
	// system event
	if (eventTypeByte & 0xf0) == 0xf0 {
		switch eventTypeByte {
		// meta event
		case 0xff:
			eventType = "meta"

			subTypeByte := m.readUint8()
			length := int(m.readVarUint())

			switch subTypeByte {
			case 0x00:
				subType = "sequenceNumber"
				if length == 2 {
					value["value"] = fmt.Sprintf("%d", m.readUint16())
				} else {
					m.skip(length)
				}
			case 0x01:
				subType = "text"
				value["value"] = m.readString(length)
			case 0x02:
				subType = "copyrightNotice"
				value["value"] = m.readString(length)
			case 0x03:
				subType = "trackName"
				value["value"] = m.readString(length)
			case 0x04:
				subType = "instrumentName"
				value["value"] = m.readString(length)
			case 0x05:
				subType = "lyrics"
				value["value"] = m.readString(length)
			case 0x06:
				subType = "marker"
				value["value"] = m.readString(length)
			case 0x07:
				subType = "cuePoint"
				value["value"] = m.readString(length)
			case 0x20:
				subType = "midiChannelPrefix"
				if length == 1 {
					value["value"] = fmt.Sprintf("%d", m.readUint8())
				} else {
					m.skip(length)
				}
			case 0x2f:
				subType = "endOfTrack"
				if length > 0 {
					m.skip(length)
				}
			case 0x51:
				subType = "setTempo"
				if length == 3 {
					value["value"] = fmt.Sprintf("%d", m.readUint24())
				} else {
					m.skip(length)
				}
			case 0x54:
				subType = "smpteOffset"
				if length == 5 {
					hourByte := m.readUint8()
					value["frameRate"] = fmt.Sprintf("%f", []float32{24, 25, 29.97, 30}[hourByte>>6])
					value["hour"] = fmt.Sprintf("%d", hourByte&0x3f)
					value["minute"] = fmt.Sprintf("%d", m.readUint8())
					value["second"] = fmt.Sprintf("%d", m.readUint8())
					value["frame"] = fmt.Sprintf("%d", m.readUint8())
					value["subFrame"] = fmt.Sprintf("%d", m.readUint8())
				} else {
					m.skip(length)
				}
			case 0x58:
				subType = "timeSignature"
				if length == 4 {
					value["numerator"] = fmt.Sprintf("%d", m.readUint8())
					value["denominator"] = fmt.Sprintf("%d", m.readUint8())
					value["metronome"] = fmt.Sprintf("%d", 1<<int(m.readUint8()))
					value["thirtyseconds"] = fmt.Sprintf("%d", m.readUint8())
				} else {
					m.skip(length)
				}
			case 0x59:
				subType = "keySignature"
				if length == 2 {
					value["key"] = fmt.Sprintf("%d", m.readUint8())
					value["scale"] = fmt.Sprintf("%d", m.readUint8())
				} else {
					m.skip(length)
				}
			case 0x7f:
				subType = "sequencerSpecific"
				value["value"] = m.readString(length)
			default:
				subType = "unknown"
				value["value"] = m.readString(length)
			}
		// sysex event
		case 0xf0:
			eventType = "sysEx"
			length := int(m.readVarUint())
			value["value"] = m.readString(length)
		case 0xf7:
			eventType = "dividedSysEx"
			length := int(m.readVarUint())
			value["value"] = m.readString(length)
		default:
			eventType = "unknown"
			length := int(m.readVarUint())
			value["value"] = m.readString(length)
		}
		// channel event
	} else {
		var param byte

		// if the high bit  is low
		// use running event type mode
		if (eventTypeByte & 0x80) == 0x00 {
			param = eventTypeByte
			eventTypeByte = m.lastEventTypeByte
		} else {
			param = m.readUint8()
			m.lastEventTypeByte = eventTypeByte
		}

		channelEventType := eventTypeByte >> 4

		channel = eventTypeByte & 0x0f
		eventType = "channel"

		switch channelEventType {
		case 0x08:
			subType = "noteOff"
			value["noteNumber"] = fmt.Sprintf("%d", param)
			value["velocity"] = fmt.Sprintf("%d", m.readUint8())
		case 0x09:
			value["noteNumber"] = fmt.Sprintf("%d", param)
			value["velocity"] = fmt.Sprintf("%d", m.readUint8())

			// some midi implementations use a noteOn
			// event with 0 velocity to denote noteOff
			if v, _ := strconv.Atoi(value["velocity"]); v == 0 {
				subType = "noteOff"
			} else {
				subType = "noteOn"
			}
		case 0x0a:
			subType = "noteAftertouch"
			value["noteNumber"] = fmt.Sprintf("%d", param)
			value["amount"] = fmt.Sprintf("%d", m.readUint8())
		case 0x0b:
			subType = "controller"
			value["controllerNumber"] = fmt.Sprintf("%d", param)
			value["controllerValue"] = fmt.Sprintf("%d", m.readUint8())
		case 0x0c:
			subType = "programChange"
			value["value"] = fmt.Sprintf("%d", param)
		case 0x0d:
			subType = "channelAftertouch"
			value["value"] = fmt.Sprintf("%d", param)
		case 0x0e:
			subType = "pitchBend"
			value["value"] = fmt.Sprintf("%d", uint(param)+uint(m.readUint8())<<7)
		default:
			subType = "unknown"
			value["value"] = fmt.Sprintf("%d", (uint(param)<<8)+uint(m.readUint8()))
		}
	}
	return &midiEvent{
		delta:     delta,
		eventType: eventType,
		subType:   subType,
		value:     value,
		channel:   channel,
	}
}
