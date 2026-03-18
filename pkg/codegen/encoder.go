package codegen

import (
	"encoding/binary"
	"io"
)

// Binary format constants.
var Magic = [4]byte{0x52, 0x59, 0x58, 0x00} // "RYX\0"

const FormatVersion uint16 = 1

// ---------------------------------------------------------------------------
// Encoder
// ---------------------------------------------------------------------------

// Encode writes a CompiledProgram to w in the RYX binary format.
func Encode(w io.Writer, prog *CompiledProgram) error {
	e := &encoder{w: w}

	// Header
	e.writeBytes(Magic[:])
	e.writeU16(FormatVersion)
	e.writeU16(0) // flags (reserved)

	// String pool
	e.writeU32(uint32(len(prog.StringPool)))
	for _, s := range prog.StringPool {
		b := []byte(s)
		e.writeU32(uint32(len(b)))
		e.writeBytes(b)
	}

	// Type pool
	e.writeU32(uint32(len(prog.TypePool)))
	for _, td := range prog.TypePool {
		e.writeByte(byte(td.Tag))
		e.writeU32(td.NameIdx)
		e.writeU16(uint16(len(td.Fields)))
		for _, f := range td.Fields {
			e.writeU32(f)
		}
	}

	// Function table
	e.writeU32(uint32(len(prog.Functions)))
	for _, fn := range prog.Functions {
		e.writeU32(fn.NameIdx)
		e.writeU16(fn.Arity)
		e.writeU16(fn.LocalsCount)
		e.writeU16(fn.UpvalueCount)
		e.writeU16(fn.MaxStack)
		e.writeU32(fn.CodeOffset)
		e.writeU32(fn.CodeLength)
		e.writeU32(fn.SourceMapOffset)
	}

	// Code section
	e.writeU32(uint32(len(prog.Code)))
	e.writeBytes(prog.Code)

	// Source map
	e.writeU32(uint32(len(prog.SourceMap)))
	for _, sm := range prog.SourceMap {
		e.writeU32(sm.BytecodeOffset)
		e.writeU32(sm.Line)
		e.writeU16(sm.Col)
	}

	// Entry point
	e.writeU32(prog.MainIndex)

	return e.err
}

// ---------------------------------------------------------------------------
// encoder helpers (accumulate first error)
// ---------------------------------------------------------------------------

type encoder struct {
	w   io.Writer
	err error
}

func (e *encoder) writeBytes(b []byte) {
	if e.err != nil {
		return
	}
	_, e.err = e.w.Write(b)
}

func (e *encoder) writeByte(b byte) {
	e.writeBytes([]byte{b})
}

func (e *encoder) writeU16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	e.writeBytes(b[:])
}

func (e *encoder) writeU32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	e.writeBytes(b[:])
}
