package pisa

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/jug/engineio"
)

func TestCallStackPush(t *testing.T) {
	stack := make(callstack, 0, 10)

	frame := &callframe{
		scope: "test",
		label: "push",
		point: 0x1234,
	}

	for i := 0; i < MaxCallDepth; i++ {
		ok := stack.push(frame)
		if !ok && i != MaxCallDepth-1 {
			require.Fail(t, fmt.Sprintf("Failed to push frame at depth %d", i))
		}
	}

	require.False(t, stack.push(frame), "Push should fail when stack depth exceeds MaxCallDepth")
}

func TestCallStackPop(t *testing.T) {
	stack := make(callstack, 0, 10)
	stack.push(&callframe{scope: "test", label: "pop1", point: 0x1234})
	stack.push(&callframe{scope: "test", label: "pop2", point: 0x1235})

	stack.pop()
	assert.Equal(t, 1, len(stack), "Expected stack depth to be 1 after pop")

	stack.pop()
	assert.Equal(t, 0, len(stack), "Expected stack depth to be 0 after pop")

	stack.pop()
	assert.Equal(t, 0, len(stack), "Expected stack depth to be 0 after over-pop")
}

func TestCallStackDepth(t *testing.T) {
	stack := make(callstack, 0, 10)

	stack.push(&callframe{scope: "test", label: "depth1", point: 0x1234})
	stack.push(&callframe{scope: "test", label: "depth2", point: 0x1235})
	assert.Equal(t, uint64(2), stack.depth(), "Expected call stack depth to be 2")

	stack.pop()
	assert.Equal(t, uint64(1), stack.depth(), "Expected call stack depth to be 1 after pop")
}

func TestCallStackHead(t *testing.T) {
	stack := make(callstack, 0, 10)

	frame1 := &callframe{scope: "test", label: "head1", point: 0x1234}
	frame2 := &callframe{scope: "test", label: "head2", point: 0x1235}

	stack.push(frame1)
	stack.push(frame2)

	head := stack.head()

	assert.NotNil(t, head, "Expected non-nil head of call stack")
	assert.Equal(t, frame2, head, "Expected head of call stack to be the last frame pushed")
}

func TestCallStackInject(t *testing.T) {
	stack := make(callstack, 0, 10)
	stack.push(&callframe{scope: "test", label: "inject1", point: 0x1234})
	stack.push(&callframe{scope: "test", label: "inject2", point: 0x1235})

	stack.inject(100, Instruction{OBTAIN, []byte{0x1, 0x0}})
	assert.Equal(t, "test.inject2() [0x1235] ... [0x64: OBTAIN 0x1 0x0]", stack.head().String(),
		"Expected injected instruction at head of call stack",
	)
}

func TestCallStackTrace(t *testing.T) {
	stack := callstack{}

	frame1 := &callframe{scope: "root", label: "start"}
	frame2 := &callframe{scope: "foo", label: "bar", point: 0x2000}

	stack.push(frame1)
	stack.push(frame2)

	stack.inject(0x1234, Instruction{Op: ADD, Args: []byte{0x0, 0x1, 0x2}})

	trace := stack.trace()

	assert.Equal(t, []string{
		"root.start()",
		"foo.bar() [0x2000] ... [0x1234: ADD 0x0 0x1 0x2]",
	}, trace, "call stack trace should match expected value")
}

func TestCallScopeThrow(t *testing.T) {
	scope := &callscope{
		engine: &Engine{callstack: make(callstack, 0)},
	}

	scope.engine.callstack.push(&callframe{scope: "root", label: "start"})
	scope.engine.callstack.push(&callframe{scope: "foo", label: "bar", point: 0x2000})

	except := scope.throw(exception(RuntimeError, "forced exit"))
	require.Equal(t, &Exception{
		Class: "builtin.RuntimeError",
		Error: "forced exit",
		Trace: []string{
			"root.start()",
			"foo.bar() [0x2000]",
		},
	}, except)

	except = scope.throw(exceptionf(RuntimeError, "forced exit: [%v, %v]", "out", "kick"))
	require.Equal(t, &Exception{
		Class: "builtin.RuntimeError",
		Error: "forced exit: [out, kick]",
		Trace: []string{
			"root.start()",
			"foo.bar() [0x2000]",
		},
	}, except)
}

func TestCallScopeRaise(t *testing.T) {
	t.Run("raiseException", func(t *testing.T) {
		scope := &callscope{
			engine: &Engine{callstack: make(callstack, 0)},
		}

		except := &Exception{Class: RuntimeError.Name(), Error: "test exception"}
		except = except.traced([]string{})

		continuity := scope.raise(except.traced(nil))

		require.NotNil(t, continuity)
		assert.Equal(t, continueModeExcept, continuity.mode())
		assert.Equal(t, engineio.Fuel(0), continuity.fuel())
		assert.Equal(t, except, continuity.exception)
	})

	t.Run("raiseExceptionWithConsumption", func(t *testing.T) {
		scope := &callscope{
			engine: &Engine{callstack: make(callstack, 0)},
		}

		except := &Exception{Class: RuntimeError.Name(), Error: "test exception"}
		except = except.traced([]string{})

		continuity := scope.raise(except).withConsumption(42)

		require.NotNil(t, continuity)
		assert.Equal(t, continueModeExcept, continuity.mode())
		assert.Equal(t, engineio.Fuel(42), continuity.fuel())
		assert.Equal(t, except, continuity.exception)
	})
}

func TestCallScopeGetPtrValue(t *testing.T) {
	scope := &callscope{
		memory: make(RegisterSet),
		engine: &Engine{callstack: make(callstack, 0)},
	}

	scope.memory.Set(0, StringValue("foo"))
	scope.memory.Set(1, PtrValue(24))

	reg, except := scope.getPtrValue(0)
	require.Equal(t, PtrValue(0), reg)
	require.Equal(t, &Exception{
		Class: "builtin.ReferenceError",
		Error: "not a pointer: $0",
		Trace: []string{},
	}, except)

	reg, except = scope.getPtrValue(1)
	require.Equal(t, PtrValue(24), reg)
	require.Nil(t, except)
}

func TestCallScopeGetSymmetricValues(t *testing.T) {
	scope := &callscope{
		memory: make(RegisterSet),
		engine: &Engine{callstack: make(callstack, 0)},
	}

	scope.memory.Set(0, StringValue("foo"))
	scope.memory.Set(1, U64Value(24))
	scope.memory.Set(2, U64Value(87))

	regA, regB, except := scope.getSymmetricValues(0, 1)
	require.Nil(t, regA)
	require.Nil(t, regB)
	require.Equal(t, &Exception{
		Class: "builtin.ValueError",
		Error: "not symmetric: [$0, $1]",
		Trace: []string{},
	}, except)

	regA, regB, except = scope.getSymmetricValues(1, 2)
	require.Equal(t, U64Value(24), regA)
	require.Equal(t, U64Value(87), regB)
	require.Nil(t, except)
}
