package codegen

import (
	"fmt"
	"math"
	"strings"
)

// Disassemble produces a human-readable listing of bytecode instructions.
// If strings is non-nil, string pool indices are resolved to their values.
func Disassemble(code []byte, stringPool []string) string {
	var sb strings.Builder
	offset := 0
	for offset < len(code) {
		instr, size, err := DecodeInstruction(code, offset)
		if err != nil {
			fmt.Fprintf(&sb, "%04X  ERROR: %v\n", offset, err)
			break
		}
		fmt.Fprintf(&sb, "%04X  %s", offset, formatInstruction(instr, stringPool))
		sb.WriteByte('\n')
		offset += size
	}
	return sb.String()
}

// DisassembleFunc disassembles a single function's code section.
func DisassembleFunc(prog *CompiledProgram, fnIdx int) string {
	if fnIdx < 0 || fnIdx >= len(prog.Functions) {
		return "<invalid function index>\n"
	}
	fn := prog.Functions[fnIdx]
	start := int(fn.CodeOffset)
	end := start + int(fn.CodeLength)
	if end > len(prog.Code) {
		end = len(prog.Code)
	}

	var sb strings.Builder
	fnName := ""
	if int(fn.NameIdx) < len(prog.StringPool) {
		fnName = prog.StringPool[fn.NameIdx]
	}
	fmt.Fprintf(&sb, "; function %s (arity=%d, locals=%d, max_stack=%d)\n",
		fnName, fn.Arity, fn.LocalsCount, fn.MaxStack)

	code := prog.Code[start:end]
	offset := 0
	for offset < len(code) {
		instr, size, err := DecodeInstruction(code, offset)
		if err != nil {
			fmt.Fprintf(&sb, "  %04X  ERROR: %v\n", offset, err)
			break
		}
		fmt.Fprintf(&sb, "  %04X  %s\n", offset, formatInstruction(instr, prog.StringPool))
		offset += size
	}
	return sb.String()
}

// DisassembleProgram produces a full listing of all functions.
func DisassembleProgram(prog *CompiledProgram) string {
	var sb strings.Builder
	sb.WriteString("; RYX bytecode\n")
	fmt.Fprintf(&sb, "; strings: %d, types: %d, functions: %d\n",
		len(prog.StringPool), len(prog.TypePool), len(prog.Functions))
	fmt.Fprintf(&sb, "; entry: function #%d\n\n", prog.MainIndex)

	for i := range prog.Functions {
		sb.WriteString(DisassembleFunc(prog, i))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

func formatInstruction(instr Instruction, stringPool []string) string {
	info, ok := LookupOpcode(instr.Op)
	if !ok {
		return fmt.Sprintf("UNKNOWN(0x%02X)", byte(instr.Op))
	}

	switch info.Operands {
	case OpNone:
		return info.Name

	case OpI64:
		if len(instr.Operands) > 0 {
			return fmt.Sprintf("%-20s %d", info.Name, instr.Operands[0])
		}

	case OpF64:
		if len(instr.Operands) > 0 {
			f := math.Float64frombits(uint64(instr.Operands[0]))
			return fmt.Sprintf("%-20s %g", info.Name, f)
		}

	case OpU32:
		if len(instr.Operands) > 0 {
			v := uint32(instr.Operands[0])
			if instr.Op == OpConstChar {
				return fmt.Sprintf("%-20s U+%04X '%c'", info.Name, v, rune(v))
			}
			return fmt.Sprintf("%-20s %d", info.Name, v)
		}

	case OpU16:
		if len(instr.Operands) > 0 {
			v := uint16(instr.Operands[0])
			extra := resolveStringRef(instr.Op, v, stringPool)
			if extra != "" {
				return fmt.Sprintf("%-20s %d  ; %s", info.Name, v, extra)
			}
			return fmt.Sprintf("%-20s %d", info.Name, v)
		}

	case OpI16:
		if len(instr.Operands) > 0 {
			return fmt.Sprintf("%-20s %+d", info.Name, int16(instr.Operands[0]))
		}

	case OpU16U16:
		if len(instr.Operands) >= 2 {
			return fmt.Sprintf("%-20s %d, %d", info.Name, instr.Operands[0], instr.Operands[1])
		}

	case OpU16U16U16:
		if len(instr.Operands) >= 3 {
			return fmt.Sprintf("%-20s %d, %d, %d", info.Name, instr.Operands[0], instr.Operands[1], instr.Operands[2])
		}

	case OpU16U16Src:
		if len(instr.Operands) >= 2 {
			return fmt.Sprintf("%-20s line %d, col %d", info.Name, instr.Operands[0], instr.Operands[1])
		}

	case OpJumpTab:
		if len(instr.Operands) > 0 {
			count := instr.Operands[0]
			offsets := make([]string, 0, count)
			for i := 1; i <= int(count) && i < len(instr.Operands); i++ {
				offsets = append(offsets, fmt.Sprintf("%+d", int16(instr.Operands[i])))
			}
			return fmt.Sprintf("%-20s [%s]", info.Name, strings.Join(offsets, ", "))
		}
	}

	return info.Name
}

// resolveStringRef returns a comment showing the string value for opcodes that index the string pool.
func resolveStringRef(op Opcode, idx uint16, pool []string) string {
	switch op {
	case OpConstString, OpLoadGlobal, OpStoreGlobal:
		if int(idx) < len(pool) {
			return fmt.Sprintf("%q", pool[idx])
		}
	}
	return ""
}
