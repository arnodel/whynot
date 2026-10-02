package engine

import (
	"math"
	"time"
)

// DocumentStack is a View's laid-out document: the top-level StackBox, the
// scroll cursor into it, and a height estimate for every slot, so the
// document's total height is known before all of it has been laid out.
//
// It outlives any one StackBox: a rebuild (resize, zoom, StyleSheet change)
// swaps in a new box via setBox, keeping the cursor at the same logical
// position and the per-slot heights as seed estimates.
type DocumentStack struct {
	Box *StackBox // nil until the first setBox

	// Cursor is the scroll position: which slot is at the top of the
	// viewport, and how far into it. Unlike a raw pixel offset, it stays
	// meaningful across a rebuild - reflow can change a slot's height, but
	// not which slot content belongs to.
	Cursor StackCursor

	// Heights holds each slot's last-known height in pixels, or -1 if never
	// known. Kept across invalidation and setBox: a stale height is a better
	// estimate than the document-wide average.
	Heights []float64
}

// LaidOut reports whether setBox has been called yet.
func (s *DocumentStack) LaidOut() bool {
	return s.Box != nil
}

// SetBox replaces the laid-out document, re-anchoring the cursor at the
// same proportion through its slot as before (see reanchor).
func (s *DocumentStack) SetBox(box *StackBox) {
	oldHeight := 0
	if s.Box != nil && s.Cursor.Index < len(s.Box.Slots) {
		oldHeight = s.Box.BoxAt(s.Cursor.Index).Bounds().Dy()
	}
	s.Box = box
	s.reanchor(oldHeight)
}

// reanchor rescales the cursor's offset to the same proportion through its
// slot's current height as oldHeight represented, so the scroll position
// survives a change to that slot's height. oldHeight <= 0 means no ratio is
// known, landing at the slot's top.
func (s *DocumentStack) reanchor(oldHeight int) {
	ratio := 0.0
	if oldHeight > 0 {
		ratio = s.Cursor.Offset / float64(oldHeight)
	}
	if s.Cursor.Index >= len(s.Box.Slots) {
		s.Cursor.Index = len(s.Box.Slots) - 1
	}
	newHeight := s.Box.BoxAt(s.Cursor.Index).Bounds().Dy()
	s.Cursor = s.Box.normalizeCursor(StackCursor{Index: s.Cursor.Index, Offset: ratio * float64(newHeight)})
}

// Scroll moves the cursor by dy pixels, positive toward the end of the
// document.
func (s *DocumentStack) Scroll(dy float64) {
	s.Cursor = s.Box.moveCursor(s.Cursor, dy)
}

// ScrollToSlot puts the top of slot i at the top of the viewport.
func (s *DocumentStack) ScrollToSlot(i int) {
	s.Cursor = s.Box.normalizeCursor(StackCursor{Index: i})
}

// ScrollToRatio puts the cursor at ratio (clamped to [0, 1]) of the
// document's estimated total height.
func (s *DocumentStack) ScrollToRatio(ratio float64) {
	ratio = math.Max(0, math.Min(1, ratio))
	avg := s.refreshHeights()
	y := ratio * s.totalEstimate(avg)
	var c StackCursor
	for i := range s.Box.Slots {
		h := s.estimate(i, avg)
		if y < h || i == len(s.Box.Slots)-1 {
			c = StackCursor{Index: i, Offset: y}
			break
		}
		y -= h
	}
	// Normalizing resolves the landing slot for real, correcting the offset
	// if the estimate was off enough to spill into a neighbor.
	s.Cursor = s.Box.normalizeCursor(c)
}

// At resolves y, relative to the top of the viewport, to a slot and an
// offset into it. Walks from the cursor, so a position far from the current
// scroll position doesn't force-build every slot in between.
func (s *DocumentStack) At(y int) StackCursor {
	return s.Box.moveCursor(s.Cursor, float64(y))
}

// Len returns the number of top-level slots.
func (s *DocumentStack) Len() int {
	return len(s.Box.Slots)
}

// BlockAt returns the Block slot i is built from, or nil for a spacer.
func (s *DocumentStack) BlockAt(i int) Block {
	return s.Box.Slots[i].Block
}

// Invalidate discards slot i's layout; it's rebuilt when next needed.
func (s *DocumentStack) Invalidate(i int) {
	s.Box.invalidate(i)
}

// InvalidateWhere invalidates every resolved slot whose layout satisfies
// stale. A stale slot before the cursor is re-resolved immediately, since
// nothing else revisits already-passed slots and the height estimate would
// otherwise stay stale. If the cursor's own slot is stale, the cursor is
// re-anchored, since its height may change.
func (s *DocumentStack) InvalidateWhere(stale func(BlockLayout) bool) {
	reanchor, oldHeight := false, 0
	for i := range s.Box.Slots {
		box := s.Box.Slots[i].Box
		if box == nil || !stale(box) {
			continue
		}
		if i == s.Cursor.Index {
			reanchor, oldHeight = true, box.Bounds().Dy()
		}
		s.Box.invalidate(i)
		if i < s.Cursor.Index {
			s.Box.BoxAt(i)
		}
	}
	if reanchor {
		s.reanchor(oldHeight)
	}
}

// refreshHeights records the height of every currently-resolved slot and
// returns the average known height - what estimate substitutes for slots
// never resolved.
//
// Every BlockLayout memoizes its own Bounds(), so re-reading it for slots
// whose box hasn't changed is cheap.
func (s *DocumentStack) refreshHeights() (avg float64) {
	if len(s.Heights) != len(s.Box.Slots) {
		s.Heights = make([]float64, len(s.Box.Slots))
		for i := range s.Heights {
			s.Heights[i] = -1
		}
	}
	var sum float64
	var known int
	for i := range s.Box.Slots {
		if box := s.Box.Slots[i].Box; box != nil {
			s.Heights[i] = float64(box.Bounds().Dy())
		}
		if s.Heights[i] >= 0 {
			sum += s.Heights[i]
			known++
		}
	}
	if known == 0 {
		return 0
	}
	return sum / float64(known)
}

// estimate returns slot i's known height, or avg if it has never been
// resolved.
func (s *DocumentStack) estimate(i int, avg float64) float64 {
	if h := s.Heights[i]; h >= 0 {
		return h
	}
	return avg
}

func (s *DocumentStack) totalEstimate(avg float64) float64 {
	var total float64
	for i := range s.Heights {
		total += s.estimate(i, avg)
	}
	return total
}

// TotalHeight returns the document's estimated total height - exact once
// every slot has been resolved.
func (s *DocumentStack) TotalHeight() float64 {
	return s.totalEstimate(s.refreshHeights())
}

// VisibleRange returns the top and bottom of a viewport of viewportHeight
// at the cursor, in the same estimated document coordinates as
// totalHeight. The bottom is top+viewportHeight clamped to the total, not
// the end of the last visible slot: which slot is last changes discretely
// while scrolling, which would make a scrollbar thumb's size jump.
//
// As a side effect, resolves the slots Draw would draw, which keeps the
// estimate exact near the viewport.
func (s *DocumentStack) VisibleRange(viewportHeight int) (top, bottom float64) {
	avg := s.refreshHeights()
	var before, total float64
	for i := range s.Heights {
		h := s.estimate(i, avg)
		total += h
		if i < s.Cursor.Index {
			before += h
		}
	}
	top = before + s.Cursor.Offset

	// Resolving a slot replaces its estimated contribution to total with
	// its real one - corrected in place rather than with a second pass.
	pos := before
	for i := s.Cursor.Index; i < len(s.Box.Slots) && pos-top <= float64(viewportHeight); i++ {
		real := float64(s.Box.BoxAt(i).Bounds().Dy())
		total += real - s.estimate(i, avg)
		s.Heights[i] = real
		pos += real
	}
	return top, math.Min(top+float64(viewportHeight), total)
}

// PreLayoutHeightRadius is how far beyond the visible viewport preLayout
// lays out content ahead of time, in physical pixels.
const PreLayoutHeightRadius = 3000

// preLayoutTimeBudget bounds how long one preLayout call spends, so a big
// jump (ScrollToRatio, a resize) spreads its catch-up over several frames.
const preLayoutTimeBudget = 2 * time.Millisecond

// PreLayout resolves slots within preLayoutHeightRadius of a viewport of
// viewportHeight at the cursor, in both directions, so a slot's real height
// is usually known before it scrolls into view rather than discovered
// right under the cursor.
func (s *DocumentStack) PreLayout(viewportHeight int) {
	deadline := time.Now().Add(preLayoutTimeBudget)
	s.preLayoutDirection(s.Cursor.Index, 1, float64(viewportHeight), deadline)
	s.preLayoutDirection(s.Cursor.Index-1, -1, 0, deadline)
}

// preLayoutDirection resolves slots starting at i, stepping by dir, until
// their accumulated height, minus skipHeight, reaches preLayoutHeightRadius
// or deadline passes.
func (s *DocumentStack) preLayoutDirection(i, dir int, skipHeight float64, deadline time.Time) {
	height := -skipHeight
	for step := 0; i >= 0 && i < len(s.Box.Slots) && height < PreLayoutHeightRadius; step++ {
		if step&7 == 7 && time.Now().After(deadline) {
			return
		}
		height += float64(s.Box.BoxAt(i).Bounds().Dy())
		i += dir
	}
}

// ForEachNearby calls visit with the Block of every slot within radius
// pixels (by estimated height) of a viewport of viewportHeight at the
// cursor, in both directions. Never lays anything out.
func (s *DocumentStack) ForEachNearby(viewportHeight int, radius float64, visit func(Block)) {
	avg := s.refreshHeights()
	walk := func(i, dir int, height float64) {
		for ; i >= 0 && i < len(s.Box.Slots) && height < radius; i += dir {
			if block := s.Box.Slots[i].Block; block != nil {
				visit(block)
			}
			height += s.estimate(i, avg)
		}
	}
	walk(s.Cursor.Index, 1, -float64(viewportHeight))
	walk(s.Cursor.Index-1, -1, 0)
}
