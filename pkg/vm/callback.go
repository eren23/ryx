package vm

import (
	"errors"
	"fmt"
)

// InvokeCallback runs a Ryx function or closure from native code (e.g. Ebiten Update).
// It must be called synchronously while the VM is inside execute() on the same fiber
// (e.g. from a stdlib builtin). The callee frame is marked CallbackFrame so OpReturn
// returns to the host instead of resuming bytecode that is blocked in a native call.
func (vm *VM) InvokeCallback(fn Value, args []Value, heap *Heap) (Value, error) {
	_ = heap
	f := vm.curFiber
	if f == nil {
		return UnitVal(), fmt.Errorf("callback invoker: no active fiber")
	}

	// Save SP so we can restore it after the callback returns,
	// preventing stack leak across repeated invocations (e.g. game frames).
	savedSP := f.SP

	for _, a := range args {
		f.Push(a)
	}
	f.Push(fn)

	argCount := len(args)
	funcVal := f.Stack[f.SP-argCount-1]
	var funcIdx uint32
	var closure *ClosureObj

	switch funcVal.Tag {
	case TagFunc:
		funcIdx = funcVal.AsFunc()
	case TagObj:
		obj := vm.Heap.Get(funcVal.AsObj())
		cl, ok := obj.Data.(*ClosureObj)
		if !ok {
			return UnitVal(), fmt.Errorf("callback invoker: expected closure")
		}
		funcIdx = uint32(cl.FuncIdx)
		closure = cl
	default:
		return UnitVal(), fmt.Errorf("callback invoker: expected callable, got %s", tagName(funcVal.Tag))
	}

	if int(funcIdx) >= len(vm.Program.Functions) {
		return UnitVal(), fmt.Errorf("callback invoker: invalid function index %d", funcIdx)
	}
	pfn := vm.Program.Functions[funcIdx]
	newBP := f.SP - argCount

	if !f.PushFrame(CallFrame{
		FuncIdx:       funcIdx,
		IP:            int(pfn.CodeOffset),
		BP:            newBP,
		Closure:       closure,
		CallbackFrame: true,
	}) {
		return UnitVal(), fmt.Errorf("callback invoker: call depth exceeded")
	}

	needed := newBP + int(pfn.LocalsCount)
	if needed > MaxStackSize {
		return UnitVal(), fmt.Errorf("callback invoker: stack overflow")
	}
	for f.SP < needed {
		f.Stack[f.SP] = UnitVal()
		f.SP++
	}

	for {
		err := vm.execute(f)
		if err != nil {
			if errors.Is(err, ErrCallbackReturn) {
				result := f.Pop()
				f.SP = savedSP
				return result, nil
			}
			f.SP = savedSP
			return UnitVal(), err
		}
		if f.State != FiberRunning {
			return UnitVal(), fmt.Errorf("callback invoker: fiber not running")
		}
	}
}
