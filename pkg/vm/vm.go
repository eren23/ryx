package vm

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/ryx-lang/ryx/pkg/codegen"
)

// ---------------------------------------------------------------------------
// VM
// ---------------------------------------------------------------------------

// VM executes a compiled Ryx program.
type VM struct {
	Program   *codegen.CompiledProgram
	Heap      *Heap
	Scheduler *Scheduler
	Globals   []Value

	Stdout io.Writer // default os.Stdout

	// Source location tracking (updated by OpSourceLoc).
	lastLine uint32
	lastCol  uint16
}

// NewVM creates a VM ready to execute the given program.
func NewVM(prog *codegen.CompiledProgram) *VM {
	globals := make([]Value, len(prog.Functions))
	for i := range prog.Functions {
		globals[i] = FuncVal(uint32(i))
	}
	return &VM{
		Program:   prog,
		Heap:      NewHeap(),
		Scheduler: NewScheduler(DefaultTimeslice),
		Globals:   globals,
		Stdout:    os.Stdout,
	}
}

// FuncName returns the name of the function at the given index.
func (vm *VM) FuncName(idx uint32) string {
	if int(idx) >= len(vm.Program.Functions) {
		return "<unknown>"
	}
	nameIdx := vm.Program.Functions[idx].NameIdx
	if int(nameIdx) < len(vm.Program.StringPool) {
		return vm.Program.StringPool[nameIdx]
	}
	return fmt.Sprintf("<func_%d>", idx)
}

// Run executes the program starting from the main function.
func (vm *VM) Run() error {
	mainIdx := vm.Program.MainIndex
	if int(mainIdx) >= len(vm.Program.Functions) {
		return &RuntimeError{Message: "main function not found"}
	}

	mainFn := vm.Program.Functions[mainIdx]
	fiber := vm.Scheduler.NewFiber()
	fiber.PushFrame(CallFrame{
		FuncIdx: mainIdx,
		IP:      int(mainFn.CodeOffset),
		BP:      0,
	})
	fiber.SP = int(mainFn.LocalsCount)
	vm.Scheduler.Ready(fiber)

	return vm.runLoop()
}

// runLoop is the main scheduler loop.
func (vm *VM) runLoop() error {
	for {
		fiber := vm.Scheduler.Next()
		if fiber == nil {
			// No runnable fibers.
			if vm.Scheduler.DeadlockDetected() {
				return errDeadlock()
			}
			return nil
		}

		err := vm.execute(fiber)
		if err != nil {
			// Attach stack trace.
			if re, ok := err.(*RuntimeError); ok && len(re.StackTrace) == 0 {
				re.StackTrace = fiber.BuildStackTrace(vm)
			}
			return err
		}

		switch fiber.State {
		case FiberRunning:
			// Timeslice expired — requeue.
			vm.Scheduler.Ready(fiber)
		case FiberDead:
			vm.Scheduler.MarkDead(fiber)
		case FiberBlocked:
			// Already tracked by channel; stays in allFibers.
		}
	}
}

// ---------------------------------------------------------------------------
// Inline operand readers
// ---------------------------------------------------------------------------

func readU16(code []byte, off int) uint16 {
	return binary.LittleEndian.Uint16(code[off : off+2])
}

func readI16(code []byte, off int) int16 {
	return int16(binary.LittleEndian.Uint16(code[off : off+2]))
}

func readU32(code []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(code[off : off+4])
}

func readI64(code []byte, off int) int64 {
	return int64(binary.LittleEndian.Uint64(code[off : off+8]))
}

func readU64(code []byte, off int) uint64 {
	return binary.LittleEndian.Uint64(code[off : off+8])
}

// ---------------------------------------------------------------------------
// Dispatch loop
// ---------------------------------------------------------------------------

func (vm *VM) execute(fiber *Fiber) error {
	code := vm.Program.Code
	instrLeft := vm.Scheduler.Timeslice

	for instrLeft > 0 && fiber.State == FiberRunning {
		instrLeft--

		frame := fiber.CurrentFrame()
		ip := frame.IP

		if ip >= len(code) {
			fiber.State = FiberDead
			return nil
		}

		op := codegen.Opcode(code[ip])

		switch op {

		// =================================================================
		// Stack operations
		// =================================================================

		case codegen.OpConstInt:
			v := readI64(code, ip+1)
			fiber.Push(IntVal(v))
			frame.IP = ip + 9

		case codegen.OpConstFloat:
			bits := readU64(code, ip+1)
			fiber.Push(Value{TagFloat, bits})
			frame.IP = ip + 9

		case codegen.OpConstTrue:
			fiber.Push(BoolVal(true))
			frame.IP = ip + 1

		case codegen.OpConstFalse:
			fiber.Push(BoolVal(false))
			frame.IP = ip + 1

		case codegen.OpConstUnit:
			fiber.Push(UnitVal())
			frame.IP = ip + 1

		case codegen.OpConstString:
			idx := readU16(code, ip+1)
			s := vm.Program.StringPool[idx]
			s = unquoteString(s)
			heapIdx := vm.Heap.AllocString(s)
			fiber.Push(ObjVal(heapIdx))
			frame.IP = ip + 3

		case codegen.OpConstChar:
			ch := readU32(code, ip+1)
			fiber.Push(CharVal(rune(ch)))
			frame.IP = ip + 5

		case codegen.OpPop:
			fiber.Pop()
			frame.IP = ip + 1

		case codegen.OpDup:
			fiber.Push(fiber.Peek())
			frame.IP = ip + 1

		case codegen.OpSwap:
			a := fiber.Pop()
			b := fiber.Pop()
			fiber.Push(a)
			fiber.Push(b)
			frame.IP = ip + 1

		// =================================================================
		// Variable access
		// =================================================================

		case codegen.OpLoadLocal:
			slot := readU16(code, ip+1)
			fiber.Push(fiber.Stack[frame.BP+int(slot)])
			frame.IP = ip + 3

		case codegen.OpStoreLocal:
			slot := readU16(code, ip+1)
			fiber.Stack[frame.BP+int(slot)] = fiber.Pop()
			frame.IP = ip + 3

		case codegen.OpLoadUpvalue:
			idx := readU16(code, ip+1)
			if frame.Closure == nil {
				return errTypeMismatch("closure", "function", vm.lastLine, vm.lastCol)
			}
			fiber.Push(frame.Closure.Upvalues[idx].Get())
			frame.IP = ip + 3

		case codegen.OpStoreUpvalue:
			idx := readU16(code, ip+1)
			if frame.Closure == nil {
				return errTypeMismatch("closure", "function", vm.lastLine, vm.lastCol)
			}
			frame.Closure.Upvalues[idx].Set(fiber.Pop())
			frame.IP = ip + 3

		case codegen.OpLoadGlobal:
			idx := readU16(code, ip+1)
			if int(idx) < len(vm.Globals) {
				fiber.Push(vm.Globals[idx])
			} else {
				fiber.Push(UnitVal())
			}
			frame.IP = ip + 3

		case codegen.OpStoreGlobal:
			idx := readU16(code, ip+1)
			for int(idx) >= len(vm.Globals) {
				vm.Globals = append(vm.Globals, UnitVal())
			}
			vm.Globals[idx] = fiber.Pop()
			frame.IP = ip + 3

		// =================================================================
		// Integer arithmetic
		// =================================================================

		case codegen.OpAddInt:
			b := fiber.Pop().AsInt()
			a := fiber.Pop().AsInt()
			fiber.Push(IntVal(a + b))
			frame.IP = ip + 1

		case codegen.OpSubInt:
			b := fiber.Pop().AsInt()
			a := fiber.Pop().AsInt()
			fiber.Push(IntVal(a - b))
			frame.IP = ip + 1

		case codegen.OpMulInt:
			b := fiber.Pop().AsInt()
			a := fiber.Pop().AsInt()
			fiber.Push(IntVal(a * b))
			frame.IP = ip + 1

		case codegen.OpDivInt:
			b := fiber.Pop().AsInt()
			a := fiber.Pop().AsInt()
			if b == 0 {
				return errDivByZero(vm.lastLine, vm.lastCol)
			}
			fiber.Push(IntVal(a / b))
			frame.IP = ip + 1

		case codegen.OpModInt:
			b := fiber.Pop().AsInt()
			a := fiber.Pop().AsInt()
			if b == 0 {
				return errDivByZero(vm.lastLine, vm.lastCol)
			}
			fiber.Push(IntVal(a % b))
			frame.IP = ip + 1

		case codegen.OpNegInt:
			a := fiber.Pop().AsInt()
			fiber.Push(IntVal(-a))
			frame.IP = ip + 1

		// =================================================================
		// Float arithmetic
		// =================================================================

		case codegen.OpAddFloat:
			b := fiber.Pop().AsFloat()
			a := fiber.Pop().AsFloat()
			fiber.Push(FloatVal(a + b))
			frame.IP = ip + 1

		case codegen.OpSubFloat:
			b := fiber.Pop().AsFloat()
			a := fiber.Pop().AsFloat()
			fiber.Push(FloatVal(a - b))
			frame.IP = ip + 1

		case codegen.OpMulFloat:
			b := fiber.Pop().AsFloat()
			a := fiber.Pop().AsFloat()
			fiber.Push(FloatVal(a * b))
			frame.IP = ip + 1

		case codegen.OpDivFloat:
			b := fiber.Pop().AsFloat()
			a := fiber.Pop().AsFloat()
			if b == 0 {
				return errDivByZero(vm.lastLine, vm.lastCol)
			}
			fiber.Push(FloatVal(a / b))
			frame.IP = ip + 1

		case codegen.OpModFloat:
			b := fiber.Pop().AsFloat()
			a := fiber.Pop().AsFloat()
			fiber.Push(FloatVal(math.Mod(a, b)))
			frame.IP = ip + 1

		case codegen.OpNegFloat:
			a := fiber.Pop().AsFloat()
			fiber.Push(FloatVal(-a))
			frame.IP = ip + 1

		// =================================================================
		// String
		// =================================================================

		case codegen.OpConcatString:
			bv := fiber.Pop()
			av := fiber.Pop()
			sa := vm.resolveString(av)
			sb := vm.resolveString(bv)
			idx := vm.Heap.AllocString(sa + sb)
			fiber.Push(ObjVal(idx))
			frame.IP = ip + 1

		// =================================================================
		// Comparison
		// =================================================================

		case codegen.OpEq:
			b := fiber.Pop()
			a := fiber.Pop()
			fiber.Push(BoolVal(a.Equal(b, vm.Heap)))
			frame.IP = ip + 1

		case codegen.OpNeq:
			b := fiber.Pop()
			a := fiber.Pop()
			fiber.Push(BoolVal(!a.Equal(b, vm.Heap)))
			frame.IP = ip + 1

		case codegen.OpLtInt:
			b := fiber.Pop().AsInt()
			a := fiber.Pop().AsInt()
			fiber.Push(BoolVal(a < b))
			frame.IP = ip + 1

		case codegen.OpLtFloat:
			b := fiber.Pop().AsFloat()
			a := fiber.Pop().AsFloat()
			fiber.Push(BoolVal(a < b))
			frame.IP = ip + 1

		case codegen.OpGtInt:
			b := fiber.Pop().AsInt()
			a := fiber.Pop().AsInt()
			fiber.Push(BoolVal(a > b))
			frame.IP = ip + 1

		case codegen.OpGtFloat:
			b := fiber.Pop().AsFloat()
			a := fiber.Pop().AsFloat()
			fiber.Push(BoolVal(a > b))
			frame.IP = ip + 1

		case codegen.OpLeqInt:
			b := fiber.Pop().AsInt()
			a := fiber.Pop().AsInt()
			fiber.Push(BoolVal(a <= b))
			frame.IP = ip + 1

		case codegen.OpLeqFloat:
			b := fiber.Pop().AsFloat()
			a := fiber.Pop().AsFloat()
			fiber.Push(BoolVal(a <= b))
			frame.IP = ip + 1

		case codegen.OpGeqInt:
			b := fiber.Pop().AsInt()
			a := fiber.Pop().AsInt()
			fiber.Push(BoolVal(a >= b))
			frame.IP = ip + 1

		case codegen.OpGeqFloat:
			b := fiber.Pop().AsFloat()
			a := fiber.Pop().AsFloat()
			fiber.Push(BoolVal(a >= b))
			frame.IP = ip + 1

		// =================================================================
		// Logical
		// =================================================================

		case codegen.OpNot:
			a := fiber.Pop()
			fiber.Push(BoolVal(!a.IsTruthy()))
			frame.IP = ip + 1

		// =================================================================
		// Control flow
		// =================================================================

		case codegen.OpJump:
			offset := readI16(code, ip+1)
			frame.IP = ip + 3 + int(offset)

		case codegen.OpJumpIfTrue:
			offset := readI16(code, ip+1)
			cond := fiber.Pop()
			if cond.IsTruthy() {
				frame.IP = ip + 3 + int(offset)
			} else {
				frame.IP = ip + 3
			}

		case codegen.OpJumpIfFalse:
			offset := readI16(code, ip+1)
			cond := fiber.Pop()
			if !cond.IsTruthy() {
				frame.IP = ip + 3 + int(offset)
			} else {
				frame.IP = ip + 3
			}

		case codegen.OpJumpTable:
			count := readU16(code, ip+1)
			idx := fiber.Pop().AsInt()
			baseIP := ip + 3 + int(count)*2 // past the table
			if idx >= 0 && int(idx) < int(count) {
				off := readI16(code, ip+3+int(idx)*2)
				frame.IP = baseIP + int(off)
			} else {
				frame.IP = baseIP
			}

		// =================================================================
		// Functions
		// =================================================================

		case codegen.OpCall:
			argCount := int(readU16(code, ip+1))
			frame.IP = ip + 3 // save return IP

			funcVal := fiber.Stack[fiber.SP-argCount-1]
			var funcIdx uint32
			var closure *ClosureObj

			switch funcVal.Tag {
			case TagFunc:
				funcIdx = funcVal.AsFunc()
			case TagObj:
				obj := vm.Heap.Get(funcVal.AsObj())
				cl, ok := obj.Data.(*ClosureObj)
				if !ok {
					return errTypeMismatch("callable", "object", vm.lastLine, vm.lastCol)
				}
				funcIdx = uint32(cl.FuncIdx)
				closure = cl
			default:
				return errTypeMismatch("callable", tagName(funcVal.Tag), vm.lastLine, vm.lastCol)
			}

			if int(funcIdx) >= len(vm.Program.Functions) {
				return &RuntimeError{
					Message: fmt.Sprintf("invalid function index %d", funcIdx),
					Line:    vm.lastLine, Col: vm.lastCol,
				}
			}
			fn := vm.Program.Functions[funcIdx]
			newBP := fiber.SP - argCount

			if !fiber.PushFrame(CallFrame{
				FuncIdx: funcIdx,
				IP:      int(fn.CodeOffset),
				BP:      newBP,
				Closure: closure,
			}) {
				return errCallDepthExceeded(vm.lastLine, vm.lastCol)
			}

			// Zero-fill remaining locals beyond args.
			needed := newBP + int(fn.LocalsCount)
			if needed > MaxStackSize {
				return errStackOverflow(vm.lastLine, vm.lastCol)
			}
			for fiber.SP < needed {
				fiber.Stack[fiber.SP] = UnitVal()
				fiber.SP++
			}

		case codegen.OpCallMethod:
			nameIdx := readU16(code, ip+1)
			argCount := int(readU16(code, ip+3))
			frame.IP = ip + 5
			// Method calls: for now treat the same as regular calls,
			// looking up the method name in globals.
			_ = nameIdx
			funcVal := fiber.Stack[fiber.SP-argCount-1]
			var funcIdx uint32
			var closure *ClosureObj

			switch funcVal.Tag {
			case TagFunc:
				funcIdx = funcVal.AsFunc()
			case TagObj:
				obj := vm.Heap.Get(funcVal.AsObj())
				cl, ok := obj.Data.(*ClosureObj)
				if !ok {
					return errTypeMismatch("callable", "object", vm.lastLine, vm.lastCol)
				}
				funcIdx = uint32(cl.FuncIdx)
				closure = cl
			default:
				return errTypeMismatch("callable", tagName(funcVal.Tag), vm.lastLine, vm.lastCol)
			}

			fn := vm.Program.Functions[funcIdx]
			newBP := fiber.SP - argCount

			if !fiber.PushFrame(CallFrame{
				FuncIdx: funcIdx,
				IP:      int(fn.CodeOffset),
				BP:      newBP,
				Closure: closure,
			}) {
				return errCallDepthExceeded(vm.lastLine, vm.lastCol)
			}

			needed := newBP + int(fn.LocalsCount)
			if needed > MaxStackSize {
				return errStackOverflow(vm.lastLine, vm.lastCol)
			}
			for fiber.SP < needed {
				fiber.Stack[fiber.SP] = UnitVal()
				fiber.SP++
			}

		case codegen.OpTailCall:
			argCount := int(readU16(code, ip+1))

			funcVal := fiber.Stack[fiber.SP-argCount-1]
			var funcIdx uint32
			var closure *ClosureObj

			switch funcVal.Tag {
			case TagFunc:
				funcIdx = funcVal.AsFunc()
			case TagObj:
				obj := vm.Heap.Get(funcVal.AsObj())
				cl, ok := obj.Data.(*ClosureObj)
				if !ok {
					return errTypeMismatch("callable", "object", vm.lastLine, vm.lastCol)
				}
				funcIdx = uint32(cl.FuncIdx)
				closure = cl
			default:
				return errTypeMismatch("callable", tagName(funcVal.Tag), vm.lastLine, vm.lastCol)
			}

			fn := vm.Program.Functions[funcIdx]

			// Close upvalues in the current frame.
			fiber.CloseUpvalues(frame.BP)

			// Copy args into the current frame's local slots.
			srcStart := fiber.SP - argCount
			for i := 0; i < argCount; i++ {
				fiber.Stack[frame.BP+i] = fiber.Stack[srcStart+i]
			}

			// Zero-fill remaining locals.
			for i := argCount; i < int(fn.LocalsCount); i++ {
				fiber.Stack[frame.BP+i] = UnitVal()
			}

			fiber.SP = frame.BP + int(fn.LocalsCount)
			frame.FuncIdx = funcIdx
			frame.IP = int(fn.CodeOffset)
			frame.Closure = closure

		case codegen.OpReturn:
			retVal := fiber.Pop()

			// Close upvalues in the current frame's range.
			fiber.CloseUpvalues(frame.BP)

			fiber.PopFrame()

			if fiber.FrameCount == 0 {
				// Top-level return: fiber is done.
				fiber.Push(retVal)
				fiber.State = FiberDead
				return nil
			}

			// Restore caller state. The func_val was at BP-1 in the caller's view.
			fiber.SP = frame.BP - 1
			fiber.Push(retVal)

		case codegen.OpMakeClosure:
			fnIdx := readU16(code, ip+1)
			upvalCount := int(readU16(code, ip+3))
			frame.IP = ip + 5

			upvals := make([]*UpvalueCell, upvalCount)
			for i := upvalCount - 1; i >= 0; i-- {
				v := fiber.Pop()
				upvals[i] = &UpvalueCell{Closed: v}
			}

			idx := vm.Heap.AllocClosure(fnIdx, upvals)
			fiber.Push(ObjVal(idx))

		// =================================================================
		// Data structures
		// =================================================================

		case codegen.OpMakeArray:
			count := int(readU16(code, ip+1))
			frame.IP = ip + 3
			elems := make([]Value, count)
			for i := count - 1; i >= 0; i-- {
				elems[i] = fiber.Pop()
			}
			idx := vm.Heap.AllocArray(elems)
			fiber.Push(ObjVal(idx))

		case codegen.OpMakeTuple:
			count := int(readU16(code, ip+1))
			frame.IP = ip + 3
			elems := make([]Value, count)
			for i := count - 1; i >= 0; i-- {
				elems[i] = fiber.Pop()
			}
			idx := vm.Heap.AllocTuple(elems)
			fiber.Push(ObjVal(idx))

		case codegen.OpMakeStruct:
			typeIdx := readU16(code, ip+1)
			fieldCount := int(readU16(code, ip+3))
			frame.IP = ip + 5
			fields := make([]Value, fieldCount)
			for i := fieldCount - 1; i >= 0; i-- {
				fields[i] = fiber.Pop()
			}
			idx := vm.Heap.AllocStruct(typeIdx, fields)
			fiber.Push(ObjVal(idx))

		case codegen.OpMakeEnum:
			typeIdx := readU16(code, ip+1)
			variantIdx := readU16(code, ip+3)
			fieldCount := int(readU16(code, ip+5))
			frame.IP = ip + 7
			fields := make([]Value, fieldCount)
			for i := fieldCount - 1; i >= 0; i-- {
				fields[i] = fiber.Pop()
			}
			idx := vm.Heap.AllocEnum(typeIdx, variantIdx, fields)
			fiber.Push(ObjVal(idx))

		case codegen.OpIndexGet:
			idxVal := fiber.Pop()
			objVal := fiber.Pop()
			frame.IP = ip + 1

			obj := vm.Heap.Get(objVal.AsObj())
			index := int(idxVal.AsInt())

			switch o := obj.Data.(type) {
			case *ArrayObj:
				if index < 0 || index >= len(o.Elements) {
					return errIndexOutOfBounds(index, len(o.Elements), vm.lastLine, vm.lastCol)
				}
				fiber.Push(o.Elements[index])
			case *TupleObj:
				if index < 0 || index >= len(o.Elements) {
					return errIndexOutOfBounds(index, len(o.Elements), vm.lastLine, vm.lastCol)
				}
				fiber.Push(o.Elements[index])
			case *StringObj:
				runes := []rune(o.Value)
				if index < 0 || index >= len(runes) {
					return errIndexOutOfBounds(index, len(runes), vm.lastLine, vm.lastCol)
				}
				fiber.Push(CharVal(runes[index]))
			default:
				return errTypeMismatch("indexable", "object", vm.lastLine, vm.lastCol)
			}

		case codegen.OpIndexSet:
			val := fiber.Pop()
			idxVal := fiber.Pop()
			objVal := fiber.Pop()
			frame.IP = ip + 1

			obj := vm.Heap.Get(objVal.AsObj())
			index := int(idxVal.AsInt())

			switch o := obj.Data.(type) {
			case *ArrayObj:
				if index < 0 || index >= len(o.Elements) {
					return errIndexOutOfBounds(index, len(o.Elements), vm.lastLine, vm.lastCol)
				}
				o.Elements[index] = val
			default:
				return errTypeMismatch("mutable indexable", "object", vm.lastLine, vm.lastCol)
			}

		case codegen.OpFieldGet:
			fieldIdx := int(readU16(code, ip+1))
			objVal := fiber.Pop()
			frame.IP = ip + 3

			obj := vm.Heap.Get(objVal.AsObj())
			switch o := obj.Data.(type) {
			case *StructObj:
				if fieldIdx >= len(o.Fields) {
					return errIndexOutOfBounds(fieldIdx, len(o.Fields), vm.lastLine, vm.lastCol)
				}
				fiber.Push(o.Fields[fieldIdx])
			case *EnumObj:
				if fieldIdx >= len(o.Fields) {
					return errIndexOutOfBounds(fieldIdx, len(o.Fields), vm.lastLine, vm.lastCol)
				}
				fiber.Push(o.Fields[fieldIdx])
			case *TupleObj:
				if fieldIdx >= len(o.Elements) {
					return errIndexOutOfBounds(fieldIdx, len(o.Elements), vm.lastLine, vm.lastCol)
				}
				fiber.Push(o.Elements[fieldIdx])
			default:
				return errTypeMismatch("struct/enum", "object", vm.lastLine, vm.lastCol)
			}

		case codegen.OpFieldSet:
			fieldIdx := int(readU16(code, ip+1))
			val := fiber.Pop()
			objVal := fiber.Pop()
			frame.IP = ip + 3

			obj := vm.Heap.Get(objVal.AsObj())
			switch o := obj.Data.(type) {
			case *StructObj:
				if fieldIdx >= len(o.Fields) {
					return errIndexOutOfBounds(fieldIdx, len(o.Fields), vm.lastLine, vm.lastCol)
				}
				o.Fields[fieldIdx] = val
			default:
				return errTypeMismatch("mutable struct", "object", vm.lastLine, vm.lastCol)
			}

		// =================================================================
		// Heap / GC
		// =================================================================

		case codegen.OpAllocArray:
			count := int(readU16(code, ip+1))
			frame.IP = ip + 3
			elems := make([]Value, count)
			idx := vm.Heap.AllocArray(elems)
			fiber.Push(ObjVal(idx))

		case codegen.OpAllocClosure:
			// Allocate an empty closure placeholder (used by GC machinery).
			idx := vm.Heap.AllocClosure(0, nil)
			fiber.Push(ObjVal(idx))
			frame.IP = ip + 1

		// =================================================================
		// Pattern matching
		// =================================================================

		case codegen.OpTagCheck:
			variantIdx := readU16(code, ip+1)
			frame.IP = ip + 3
			v := fiber.Peek()
			if v.Tag == TagObj {
				obj := vm.Heap.Get(v.AsObj())
				if e, ok := obj.Data.(*EnumObj); ok {
					fiber.Push(BoolVal(e.VariantIdx == variantIdx))
				} else {
					fiber.Push(BoolVal(false))
				}
			} else {
				fiber.Push(BoolVal(false))
			}

		case codegen.OpDestructure:
			count := int(readU16(code, ip+1))
			frame.IP = ip + 3
			v := fiber.Pop()
			if v.Tag == TagObj {
				obj := vm.Heap.Get(v.AsObj())
				switch o := obj.Data.(type) {
				case *EnumObj:
					for i := 0; i < count && i < len(o.Fields); i++ {
						fiber.Push(o.Fields[i])
					}
				case *StructObj:
					for i := 0; i < count && i < len(o.Fields); i++ {
						fiber.Push(o.Fields[i])
					}
				case *TupleObj:
					for i := 0; i < count && i < len(o.Elements); i++ {
						fiber.Push(o.Elements[i])
					}
				}
			}

		// =================================================================
		// Concurrency
		// =================================================================

		case codegen.OpChannelCreate:
			cap := int(readU16(code, ip+1))
			frame.IP = ip + 3
			idx := vm.Heap.AllocChannel(cap)
			fiber.Push(ObjVal(idx))

		case codegen.OpChannelSend:
			val := fiber.Pop()
			chVal := fiber.Pop()
			frame.IP = ip + 1

			obj := vm.Heap.Get(chVal.AsObj())
			ch := obj.Data.(*ChannelObj)

			result := ChannelTrySend(ch, val, fiber, vm.Scheduler)
			switch result {
			case SendOK:
				// continue
			case SendBlocked:
				return nil // yield to scheduler
			case SendClosed:
				return errSendOnClosed(vm.lastLine, vm.lastCol)
			}

		case codegen.OpChannelRecv:
			chVal := fiber.Pop()
			frame.IP = ip + 1

			obj := vm.Heap.Get(chVal.AsObj())
			ch := obj.Data.(*ChannelObj)

			val, result := ChannelTryRecv(ch, fiber, vm.Scheduler)
			switch result {
			case RecvOK:
				fiber.Push(val)
			case RecvBlocked:
				return nil // yield to scheduler
			case RecvClosed:
				fiber.Push(val)
			}

		case codegen.OpChannelClose:
			chVal := fiber.Pop()
			frame.IP = ip + 1

			obj := vm.Heap.Get(chVal.AsObj())
			ch := obj.Data.(*ChannelObj)
			ChannelClose(ch, vm.Scheduler)

		case codegen.OpSpawn:
			fnIdx := uint32(readU16(code, ip+1))
			frame.IP = ip + 3

			fn := vm.Program.Functions[fnIdx]
			argCount := int(fn.Arity)

			newFiber := vm.Scheduler.NewFiber()
			newFiber.PushFrame(CallFrame{
				FuncIdx: fnIdx,
				IP:      int(fn.CodeOffset),
				BP:      0,
			})

			// Copy args from current fiber's stack to the new fiber.
			for i := argCount - 1; i >= 0; i-- {
				v := fiber.Pop()
				newFiber.Stack[i] = v
			}
			newFiber.SP = int(fn.LocalsCount)
			vm.Scheduler.Ready(newFiber)

			fiber.Push(UnitVal()) // spawn returns unit

		// =================================================================
		// Built-ins
		// =================================================================

		case codegen.OpPrint:
			v := fiber.Pop()
			fmt.Fprint(vm.Stdout, StringValue(v, vm.Heap))
			frame.IP = ip + 1

		case codegen.OpPrintln:
			v := fiber.Pop()
			fmt.Fprintln(vm.Stdout, StringValue(v, vm.Heap))
			frame.IP = ip + 1

		case codegen.OpIntToFloat:
			v := fiber.Pop().AsInt()
			fiber.Push(FloatVal(float64(v)))
			frame.IP = ip + 1

		case codegen.OpFloatToInt:
			v := fiber.Pop().AsFloat()
			fiber.Push(IntVal(int64(v)))
			frame.IP = ip + 1

		case codegen.OpIntToString:
			v := fiber.Pop().AsInt()
			s := strconv.FormatInt(v, 10)
			idx := vm.Heap.AllocString(s)
			fiber.Push(ObjVal(idx))
			frame.IP = ip + 1

		case codegen.OpFloatToString:
			v := fiber.Pop().AsFloat()
			s := strconv.FormatFloat(v, 'g', -1, 64)
			idx := vm.Heap.AllocString(s)
			fiber.Push(ObjVal(idx))
			frame.IP = ip + 1

		case codegen.OpStringLen:
			v := fiber.Pop()
			obj := vm.Heap.Get(v.AsObj())
			s := obj.Data.(*StringObj)
			fiber.Push(IntVal(int64(len(s.Value))))
			frame.IP = ip + 1

		case codegen.OpArrayLen:
			v := fiber.Pop()
			obj := vm.Heap.Get(v.AsObj())
			switch o := obj.Data.(type) {
			case *ArrayObj:
				fiber.Push(IntVal(int64(len(o.Elements))))
			case *TupleObj:
				fiber.Push(IntVal(int64(len(o.Elements))))
			default:
				fiber.Push(IntVal(0))
			}
			frame.IP = ip + 1

		// =================================================================
		// Debug
		// =================================================================

		case codegen.OpBreakpoint:
			frame.IP = ip + 1
			// No-op in production; could hook a debugger.

		case codegen.OpSourceLoc:
			vm.lastLine = uint32(readU16(code, ip+1))
			vm.lastCol = uint16(readU16(code, ip+3))
			fiber.LastLine = vm.lastLine
			fiber.LastCol = vm.lastCol
			frame.IP = ip + 5

		default:
			return errInvalidOpcode(byte(op), vm.lastLine, vm.lastCol)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveString extracts a Go string from a Value (handles TagObj→StringObj and primitives).
func (vm *VM) resolveString(v Value) string {
	if v.Tag == TagObj {
		obj := vm.Heap.Get(v.AsObj())
		if s, ok := obj.Data.(*StringObj); ok {
			return s.Value
		}
	}
	return StringValue(v, vm.Heap)
}

// unquoteString strips surrounding quotes from string literals stored by the
// lexer. It handles regular "..." strings (with Go-compatible escape sequences)
// and raw r"..." strings (no escape processing).
func unquoteString(s string) string {
	// Regular string: "..."
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		// Fallback: strip quotes without escape processing
		return s[1 : len(s)-1]
	}
	// Raw string: r"..."
	if len(s) >= 3 && strings.HasPrefix(s, `r"`) && s[len(s)-1] == '"' {
		return s[2 : len(s)-1]
	}
	return s
}

func tagName(tag byte) string {
	switch tag {
	case TagInt:
		return "int"
	case TagFloat:
		return "float"
	case TagBool:
		return "bool"
	case TagChar:
		return "char"
	case TagUnit:
		return "unit"
	case TagFunc:
		return "func"
	case TagObj:
		return "object"
	default:
		return fmt.Sprintf("tag(%d)", tag)
	}
}
