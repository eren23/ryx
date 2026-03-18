package codegen

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Decode reads a CompiledProgram from r in the RYX binary format.
func Decode(r io.Reader) (*CompiledProgram, error) {
	d := &decoder{r: r}

	// Header
	var magic [4]byte
	d.readFull(magic[:])
	if d.err != nil {
		return nil, fmt.Errorf("decode: reading magic: %w", d.err)
	}
	if magic != Magic {
		return nil, fmt.Errorf("decode: invalid magic %v, expected RYX\\0", magic)
	}
	version := d.readU16()
	if version != FormatVersion {
		return nil, fmt.Errorf("decode: unsupported version %d", version)
	}
	_ = d.readU16() // flags (reserved)

	// String pool
	strCount := d.readU32()
	strings := make([]string, strCount)
	for i := uint32(0); i < strCount; i++ {
		sLen := d.readU32()
		buf := make([]byte, sLen)
		d.readFull(buf)
		strings[i] = string(buf)
	}

	// Type pool
	typeCount := d.readU32()
	typePool := make([]TypeDescriptor, typeCount)
	for i := uint32(0); i < typeCount; i++ {
		tag := TypeTag(d.readByte())
		nameIdx := d.readU32()
		fieldCount := d.readU16()
		fields := make([]uint32, fieldCount)
		for j := uint16(0); j < fieldCount; j++ {
			fields[j] = d.readU32()
		}
		typePool[i] = TypeDescriptor{Tag: tag, NameIdx: nameIdx, Fields: fields}
	}

	// Function table
	fnCount := d.readU32()
	funcs := make([]CompiledFunc, fnCount)
	for i := uint32(0); i < fnCount; i++ {
		funcs[i] = CompiledFunc{
			NameIdx:         d.readU32(),
			Arity:           d.readU16(),
			LocalsCount:     d.readU16(),
			UpvalueCount:    d.readU16(),
			MaxStack:        d.readU16(),
			CodeOffset:      d.readU32(),
			CodeLength:      d.readU32(),
			SourceMapOffset: d.readU32(),
		}
	}

	// Code section
	codeLen := d.readU32()
	code := make([]byte, codeLen)
	d.readFull(code)

	// Source map
	smCount := d.readU32()
	sourceMap := make([]SourceMapEntry, smCount)
	for i := uint32(0); i < smCount; i++ {
		sourceMap[i] = SourceMapEntry{
			BytecodeOffset: d.readU32(),
			Line:           d.readU32(),
			Col:            d.readU16(),
		}
	}

	// Entry point
	mainIdx := d.readU32()

	if d.err != nil {
		return nil, fmt.Errorf("decode: %w", d.err)
	}

	return &CompiledProgram{
		StringPool: strings,
		TypePool:   typePool,
		Functions:  funcs,
		Code:       code,
		SourceMap:  sourceMap,
		MainIndex:  mainIdx,
	}, nil
}

// ---------------------------------------------------------------------------
// decoder helpers
// ---------------------------------------------------------------------------

type decoder struct {
	r   io.Reader
	err error
}

func (d *decoder) readFull(buf []byte) {
	if d.err != nil {
		return
	}
	_, d.err = io.ReadFull(d.r, buf)
}

func (d *decoder) readByte() byte {
	var b [1]byte
	d.readFull(b[:])
	return b[0]
}

func (d *decoder) readU16() uint16 {
	var b [2]byte
	d.readFull(b[:])
	return binary.LittleEndian.Uint16(b[:])
}

func (d *decoder) readU32() uint32 {
	var b [4]byte
	d.readFull(b[:])
	return binary.LittleEndian.Uint32(b[:])
}
