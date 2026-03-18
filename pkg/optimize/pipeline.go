package optimize

import "github.com/ryx-lang/ryx/pkg/mir"

// Pass is an optimization pass that transforms a MIR function in place.
type Pass interface {
	Name() string
	Run(fn *mir.Function)
}

// OptLevel controls which passes are enabled.
type OptLevel int

const (
	O0 OptLevel = 0 // no optimization
	O1 OptLevel = 1 // basic optimizations
	O2 OptLevel = 2 // full optimization
)

// Pipeline runs a sequence of optimization passes on a MIR program.
func Pipeline(prog *mir.Program, level OptLevel) {
	if level == O0 {
		return
	}

	passes := buildPipeline(level)
	for _, fn := range prog.Functions {
		for _, pass := range passes {
			pass.Run(fn)
		}
	}
}

// RunPass runs a single named pass on a function. Useful for testing.
func RunPass(fn *mir.Function, pass Pass) {
	pass.Run(fn)
}

func buildPipeline(level OptLevel) []Pass {
	var passes []Pass

	if level >= O1 {
		passes = append(passes,
			&ConstantFoldPass{},
			&ConstPropPass{},
			&CopyPropPass{},
			&ConstantFoldPass{}, // Re-fold after propagation.
			&ConstPropPass{},    // Re-propagate after re-fold.
			&CopyPropPass{},     // Chase new copy chains.
			&DeadCodePass{},
			&BlockMergePass{},
		)
	}

	if level >= O2 {
		passes = append(passes,
			&CSEPass{},
			&InlinePass{MaxStmts: 8},
			// Re-run cleanup after inlining.
			&ConstantFoldPass{},
			&ConstPropPass{},
			&CopyPropPass{},
			&DeadCodePass{},
			&LICMPass{},
			&TCOPass{},
			&EscapePass{},
			&BlockMergePass{},
			// Final cleanup.
			&DeadCodePass{},
			&BlockMergePass{},
		)
	}

	return passes
}
