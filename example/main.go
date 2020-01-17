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

// +build example

package main

import (
	"flag"
	"fmt"
	"github.com/entooone/simple-midi-synth/synth"
	"io"
	"log"
	"os"
	"path/filepath"
)

func fileNameWithoutExt(path string) string {
	return filepath.Base(path[:len(path)-len(filepath.Ext(path))])
}

func init() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run -tags=example github.com/entooone/simple-midi-synth/example midifile")
		os.Exit(2)
	}
	flag.Parse()
}

func main() {
	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
	}

	filepath := args[0]
	midfile, err := os.Open(filepath)
	if err != nil {
		log.Fatal(err)
	}
	defer midfile.Close()

	buf, err := synth.MIDIToWAV(midfile)
	if err != nil {
		log.Fatal(err)
	}

	filename := fmt.Sprintf("%s.wav", fileNameWithoutExt(filepath))

	wavfile, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer wavfile.Close()

	io.Copy(wavfile, buf)
}
