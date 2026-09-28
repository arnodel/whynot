package whynot

import (
	"math"
	"time"
)

// documentStack is a View's laid-out document: the top-level StackBox, the
// scroll cursor into it, and a height estimate for every slot, so the
// document's total height is known before all of it has been laid out.
//
// It outlives any one StackBox: a rebuild (resize, zoom, StyleSheet change)
// swaps in a new box via setBox, keeping the cursor at the same logical
// position and the per-slot heights as seed estimates.
type documentStack struct {
	box *StackBox // nil until the first setBox

	// cursor is the scroll position: which slot is at the top of the
	// viewport, and how far into it. Unlike a raw pixel offset, it stays
	// meaningful across a rebuild - reflow can change a slot's height, but
	// not which slot content belongs to.
	cursor stackCursor

	// heights holds each slot's last-known height in pixels, or -1 if never
	// known. Kept across invalidation and setBox: a stale height is a better
	// estimate than the document-wide average.
	heights []float64
}

// laidOut reports whether setBox has been called yet.
func (s *documentStack) laidOut() bool {
	return s.box != nil
}

// setBox replaces the laid-out document, re-anchoring the cursor at the
// same proportion through its slot as before (see reanchor).
func (s *documentStack) setBox(box *StackBox) {
	oldHeight := 0
	if s.box != nil && s.cursor.index < len(s.box.slots) {
		oldHeight = s.box.boxAt(s.cursor.index).Bounds().Dy()
	}
	s.box = box
	s.reanchor(oldHeight)
}

// reanchor rescales the cursor's offset to the same proportion through its
// slot's current height as oldHeight represented, so the scroll position
// survives a change to that slot's height. oldHeight <= 0 means no ratio is
// known, landing at the slot's top.
func (s *documentStack) reanchor(oldHeight int) {
	ratio := 0.0
	if oldHeight > 0 {
		ratio = s.cursor.offset / float64(oldHeight)
	}
	if s.cursor.index >= len(s.box.slots) {
		s.cursor.index = len(s.box.slots) - 1
	}
	newHeight := s.box.boxAt(s.cursor.index).Bounds().Dy()
	s.cursor = s.box.normalizeCursor(stackCursor{index: s.cursor.index, offset: ratio * float64(newHeight)})
}

// scroll moves the cursor by dy pixels, positive toward the end of the
// document.
func (s *documentStack) scroll(dy float64) {
	s.cursor = s.box.moveCursor(s.cursor, dy)
}

// scrollToSlot puts the top of slot i at the top of the viewport.
func (s *documentStack) scrollToSlot(i int) {
	s.cursor = s.box.normalizeCursor(stackCursor{index: i})
}

// scrollToRatio puts the cursor at ratio (clamped to [0, 1]) of the
// document's estimated total height.
func (s *documentStack) scrollToRatio(ratio float64) {
	ratio = math.Max(0, math.Min(1, ratio))
	avg := s.refreshHeights()
	y := ratio * s.totalEstimate(avg)
	var c stackCursor
	for i := range s.box.slots {
		h := s.estimate(i, avg)
		if y < h || i == len(s.box.slots)-1 {
			c = stackCursor{index: i, offset: y}
			break
		}
		y -= h
	}
	// Normalizing resolves the landing slot for real, correcting the offset
	// if the estimate was off enough to spill into a neighbor.
	s.cursor = s.box.normalizeCursor(c)
}

// at resolves y, relative to the top of the viewport, to a slot and an
// offset into it. Walks from the cursor, so a position far from the current
// scroll position doesn't force-build every slot in between.
func (s *documentStack) at(y int) stackCursor {
	return s.box.moveCursor(s.cursor, float64(y))
}

// len returns the number of top-level slots.
func (s *documentStack) len() int {
	return len(s.box.slots)
}

// blockAt returns the Block slot i is built from, or nil for a spacer.
func (s *documentStack) blockAt(i int) Block {
	return s.box.slots[i].block
}

// invalidate discards slot i's layout; it's rebuilt when next needed.
func (s *documentStack) invalidate(i int) {
	s.box.invalidate(i)
}

// invalidateWhere invalidates every resolved slot whose layout satisfies
// stale. A stale slot before the cursor is re-resolved immediately, since
// nothing else revisits already-passed slots and the height estimate would
// otherwise stay stale. If the cursor's own slot is stale, the cursor is
// re-anchored, since its height may change.
func (s *documentStack) invalidateWhere(stale func(BlockLayout) bool) {
	reanchor, oldHeight := false, 0
	for i := range s.box.slots {
		box := s.box.slots[i].box
		if box == nil || !stale(box) {
			continue
		}
		if i == s.cursor.index {
			reanchor, oldHeight = true, box.Bounds().Dy()
		}
		s.box.invalidate(i)
		if i < s.cursor.index {
			s.box.boxAt(i)
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
func (s *documentStack) refreshHeights() (avg float64) {
	if len(s.heights) != len(s.box.slots) {
		s.heights = make([]float64, len(s.box.slots))
		for i := range s.heights {
			s.heights[i] = -1
		}
	}
	var sum float64
	var known int
	for i := range s.box.slots {
		if box := s.box.slots[i].box; box != nil {
			s.heights[i] = float64(box.Bounds().Dy())
		}
		if s.heights[i] >= 0 {
			sum += s.heights[i]
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
func (s *documentStack) estimate(i int, avg float64) float64 {
	if h := s.heights[i]; h >= 0 {
		return h
	}
	return avg
}

func (s *documentStack) totalEstimate(avg float64) float64 {
	var total float64
	for i := range s.heights {
		total += s.estimate(i, avg)
	}
	return total
}

// totalHeight returns the document's estimated total height - exact once
// every slot has been resolved.
func (s *documentStack) totalHeight() float64 {
	return s.totalEstimate(s.refreshHeights())
}

// visibleRange returns the top and bottom of a viewport of viewportHeight
// at the cursor, in the same estimated document coordinates as
// totalHeight. The bottom is top+viewportHeight clamped to the total, not
// the end of the last visible slot: which slot is last changes discretely
// while scrolling, which would make a scrollbar thumb's size jump.
//
// As a side effect, resolves the slots Draw would draw, which keeps the
// estimate exact near the viewport.
func (s *documentStack) visibleRange(viewportHeight int) (top, bottom float64) {
	avg := s.refreshHeights()
	var before, total float64
	for i := range s.heights {
		h := s.estimate(i, avg)
		total += h
		if i < s.cursor.index {
			before += h
		}
	}
	top = before + s.cursor.offset

	// Resolving a slot replaces its estimated contribution to total with
	// its real one - corrected in place rather than with a second pass.
	pos := before
	for i := s.cursor.index; i < len(s.box.slots) && pos-top <= float64(viewportHeight); i++ {
		real := float64(s.box.boxAt(i).Bounds().Dy())
		total += real - s.estimate(i, avg)
		s.heights[i] = real
		pos += real
	}
	return top, math.Min(top+float64(viewportHeight), total)
}

// preLayoutHeightRadius is how far beyond the visible viewport preLayout
// lays out content ahead of time, in physical pixels.
const preLayoutHeightRadius = 3000

// preLayoutTimeBudget bounds how long one preLayout call spends, so a big
// jump (ScrollToRatio, a resize) spreads its catch-up over several frames.
const preLayoutTimeBudget = 2 * time.Millisecond

// preLayout resolves slots within preLayoutHeightRadius of a viewport of
// viewportHeight at the cursor, in both directions, so a slot's real height
// is usually known before it scrolls into view rather than discovered
// right under the cursor.
func (s *documentStack) preLayout(viewportHeight int) {
	deadline := time.Now().Add(preLayoutTimeBudget)
	s.preLayoutDirection(s.cursor.index, 1, float64(viewportHeight), deadline)
	s.preLayoutDirection(s.cursor.index-1, -1, 0, deadline)
}

// preLayoutDirection resolves slots starting at i, stepping by dir, until
// their accumulated height, minus skipHeight, reaches preLayoutHeightRadius
// or deadline passes.
func (s *documentStack) preLayoutDirection(i, dir int, skipHeight float64, deadline time.Time) {
	height := -skipHeight
	for step := 0; i >= 0 && i < len(s.box.slots) && height < preLayoutHeightRadius; step++ {
		if step&7 == 7 && time.Now().After(deadline) {
			return
		}
		height += float64(s.box.boxAt(i).Bounds().Dy())
		i += dir
	}
}

// forEachNearby calls visit with the Block of every slot within radius
// pixels (by estimated height) of a viewport of viewportHeight at the
// cursor, in both directions. Never lays anything out.
func (s *documentStack) forEachNearby(viewportHeight int, radius float64, visit func(Block)) {
	avg := s.refreshHeights()
	walk := func(i, dir int, height float64) {
		for ; i >= 0 && i < len(s.box.slots) && height < radius; i += dir {
			if block := s.box.slots[i].block; block != nil {
				visit(block)
			}
			height += s.estimate(i, avg)
		}
	}
	walk(s.cursor.index, 1, -float64(viewportHeight))
	walk(s.cursor.index-1, -1, 0)
}
