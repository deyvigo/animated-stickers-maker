package sticker

import "context"

// Policy controls the fps/quality trade-off for animated stickers.
//
// Animated WebP has no inter-frame prediction, so bytes ≈ frames × bytes
// per frame. For a fixed 500KB budget, long clips can't keep both high fps
// and decent quantization. Crushing the quantizer is what produces visible
// blocking, so QFloor is a quality we refuse to go below while there is
// still fps left to give up: we trade smoothness for sharpness first.
type Policy struct {
	// FPSLadder lists candidate frame rates, highest (preferred) first.
	FPSLadder []int
	// QMax is the best quality worth spending bytes on.
	QMax int
	// QFloor is the lowest quality accepted while fps can still be reduced.
	QFloor int
	// QFallbackMin is how low quality may go at the lowest fps, as a last
	// resort before the caller shortens the clip.
	QFallbackMin int
}

var DefaultPolicy = Policy{
	FPSLadder:    []int{15, 12, 10, 8, 6, 5},
	QMax:         90,
	QFloor:       55,
	QFallbackMin: 35,
}

type Attempt struct {
	FPS     int
	Quality int
}

// better reports whether a is preferable to b: higher fps first (motion
// reads more than compression noise), then higher quality.
func better(a, b Attempt) bool {
	return a.FPS > b.FPS || (a.FPS == b.FPS && a.Quality > b.Quality)
}

// Encoder encodes one attempt and returns the resulting file size.
type Encoder func(ctx context.Context, a Attempt) (int64, error)

// Search finds the best attempt whose encoded size is <= budget, or nil if
// none fits. Size is monotonic in both quality and fps, which makes this
// deterministic and bounded (≈2 encodes in the best case, ≈8 in the worst
// before the fallback phase):
//
//  1. Encode at (top fps, QFloor). If it fits, binary-search the quality
//     upward at that fps to use the whole budget.
//  2. Otherwise jump to the highest fps whose predicted size
//     (size × fps ratio) fits, verify it at QFloor, and step down the
//     ladder until something fits.
//  3. If even the lowest fps doesn't fit at QFloor, search quality below
//     QFloor at the lowest fps, down to QFallbackMin.
func Search(ctx context.Context, p Policy, budget int64, enc Encoder) (*Attempt, error) {
	ladder := p.FPSLadder
	var best *Attempt

	try := func(a Attempt) (size int64, fit bool, err error) {
		size, err = enc(ctx, a)
		if err != nil {
			return 0, false, err
		}
		fit = size <= budget
		if fit && (best == nil || better(a, *best)) {
			c := a
			best = &c
		}
		return size, fit, nil
	}

	// searchQuality finds the highest fitting quality in (lo, hi] at fps,
	// given that lo is known to fit and hi is known not to.
	searchQuality := func(fps, lo, hi int) error {
		for hi-lo > 2 {
			mid := (lo + hi) / 2
			_, fit, err := try(Attempt{fps, mid})
			if err != nil {
				return err
			}
			if fit {
				lo = mid
			} else {
				hi = mid
			}
		}
		return nil
	}

	size, fit, err := try(Attempt{ladder[0], p.QFloor})
	if err != nil {
		return nil, err
	}
	ref := size
	idx := 0
	for {
		if fit {
			// Spend the rest of the budget on quality at this fps.
			_, top, err := try(Attempt{ladder[idx], p.QMax})
			if err != nil {
				return nil, err
			}
			if !top {
				if err := searchQuality(ladder[idx], p.QFloor, p.QMax); err != nil {
					return nil, err
				}
			}
			return best, nil
		}
		if idx == len(ladder)-1 {
			break
		}
		next := idx + 1
		for next < len(ladder)-1 &&
			float64(ref)*float64(ladder[next])/float64(ladder[0]) > float64(budget)*0.97 {
			next++
		}
		idx = next
		if _, fit, err = try(Attempt{ladder[idx], p.QFloor}); err != nil {
			return nil, err
		}
	}

	// Phase 3: below the quality floor, at the lowest fps.
	lowest := ladder[len(ladder)-1]
	_, fit, err = try(Attempt{lowest, p.QFallbackMin})
	if err != nil || !fit {
		return nil, err
	}
	if err := searchQuality(lowest, p.QFallbackMin, p.QFloor); err != nil {
		return nil, err
	}
	return best, nil
}

// SearchQuality finds the highest quality in [lo, hi] whose size fits the
// budget, for still images (single frame, so no fps dimension). Returns -1
// if even lo doesn't fit.
func SearchQuality(ctx context.Context, lo, hi int, budget int64, enc func(ctx context.Context, q int) (int64, error)) (int, error) {
	if size, err := enc(ctx, hi); err != nil {
		return -1, err
	} else if size <= budget {
		return hi, nil
	}
	if size, err := enc(ctx, lo); err != nil {
		return -1, err
	} else if size > budget {
		return -1, nil
	}
	// lo fits, hi doesn't.
	for hi-lo > 2 {
		mid := (lo + hi) / 2
		size, err := enc(ctx, mid)
		if err != nil {
			return -1, err
		}
		if size <= budget {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo, nil
}
