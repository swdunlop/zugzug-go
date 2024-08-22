// Copyright (c) 2023, Scott W. Dunlop
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

// Package indent provides a wrapper inserts an indent.  This wrapper is largely safe for concurrency but it may
// interleave lines that span writes.
package indent

import (
	"bytes"
	"io"
	"sync"
)

// Writer returns a writer that transforms UTF-8 writes by inserting the specified indent before the first byte
// sent and again before the first byte sent after each newline.
func Writer(w io.Writer, indent string) io.Writer {
	// as a special case, if w is a writer, we share its state.
	if prev, ok := w.(*writer); ok {
		buf := make([]byte, 0, len(prev.indent)+len(indent))
		buf = append(buf, prev.indent...)
		buf = append(buf, indent...)
		return &writer{prev.sink, buf}
	}
	return &writer{&sink{io: w}, []byte(indent)}
}

func mergeIndents(prev []byte, indent string) []byte {
	next := make([]byte, len(prev)+len(indent))
	copy(next[copy(next, prev):], indent)
	return next
}

type sink struct {
	sync.Mutex
	io       io.Writer
	indented bool
}

type writer struct {
	*sink
	indent []byte
}

func (wr *writer) Write(p []byte) (int, error) {
	originalSz := len(p)
	if originalSz == 0 {
		return 0, nil
	}
	wr.sink.Lock()
	defer wr.sink.Unlock()

	isz := len(wr.indent)
	sz := originalSz + isz
	for _, ch := range p {
		if ch == '\n' {
			sz += isz
		}
	}

	buf := make([]byte, 0, sz)
	if !wr.indented {
		// when indented is false, we must indent before outputting the first byte
		buf = append(buf, wr.indent...)
		wr.indented = true
	}

	for {
		i := bytes.IndexByte(p, '\n')
		if i == -1 {
			buf = append(buf, p...)
			break
		}
		i++
		p, buf = p[i:], append(buf, p[:i]...)
		if len(p) < 1 {
			// reset indented, so we will indent on the next byte.
			wr.indented = false
			break
		}
		buf = append(buf, wr.indent...)
	}

	_, err := wr.io.Write(buf)
	if err == nil {
		return originalSz, nil
	}
	return 0, err
}
