package optimize

import "github.com/ryx-lang/ryx/pkg/mir"

// BlockMergePass merges blocks with a single predecessor/successor pair.
// If block A has exactly one successor B, and B has exactly one predecessor A,
// then B's content is merged into A and B is removed.
type BlockMergePass struct{}

func (*BlockMergePass) Name() string { return "block-merge" }

func (*BlockMergePass) Run(fn *mir.Function) {
	changed := true
	for changed {
		changed = false
		for _, blk := range fn.Blocks {
			if blk.Term == nil {
				continue
			}

			// Check if block has exactly one successor via Goto.
			gt, ok := blk.Term.(*mir.Goto)
			if !ok {
				continue
			}

			target := fn.Block(gt.Target)

			// Target must have exactly one predecessor (this block).
			if len(target.Preds) != 1 {
				continue
			}

			// Don't merge the entry block into itself.
			if target.ID == blk.ID {
				continue
			}

			// Don't merge if target is the entry block.
			if target.ID == fn.Entry {
				continue
			}

			// Merge target into blk.
			// Resolve phi nodes: since there's only one predecessor,
			// each phi becomes an assignment.
			for _, phi := range target.Phis {
				if v, ok := phi.Args[blk.ID]; ok {
					blk.Stmts = append(blk.Stmts, &mir.Assign{
						Dest: phi.Dest,
						Src:  v,
						Type: phi.Type,
					})
				}
			}

			// Append target's statements.
			blk.Stmts = append(blk.Stmts, target.Stmts...)

			// Take target's terminator.
			blk.Term = target.Term

			// Update successor edges.
			blk.Succs = target.Succs

			// Update predecessor references in successors of target.
			for _, succ := range target.Succs {
				succBlk := fn.Block(succ)
				for i, pred := range succBlk.Preds {
					if pred == target.ID {
						succBlk.Preds[i] = blk.ID
					}
				}
				// Update phi args to reference the merged block.
				for _, phi := range succBlk.Phis {
					if v, ok := phi.Args[target.ID]; ok {
						phi.Args[blk.ID] = v
						delete(phi.Args, target.ID)
					}
				}
			}

			// Also update terminator block references.
			rewriteTermBlockIDSingle(blk, target.ID, blk.ID)

			// Clear the merged block.
			target.Stmts = nil
			target.Phis = nil
			target.Term = nil
			target.Preds = nil
			target.Succs = nil

			changed = true
		}
	}

	// Clean up by removing unreachable blocks.
	removeUnreachableBlocks(fn)
}
