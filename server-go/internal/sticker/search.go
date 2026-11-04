package sticker

import "context"

// Policy controls how an animated sticker is squeezed into its byte budget.
//
// Animated WebP has no inter-frame prediction, so bytes ≈ frames × bytes
// per frame. Heavy footage (fur, water, camera motion: every pixel changes)
// can't keep both a high fps and a decent quantizer inside 500KB. Below
// roughly q=50 libwebp shows flat blocky patches, which reads far worse than
// a slightly soft picture or a slightly lower fps, so there are three knobs,
// spent in this order for a given fps:
//
//  1. quality, never below QFloor while another option is left;
//  2. a light gaussian pre-blur (Blurs), which removes detail the encoder
//     would otherwise spend bytes on and lets quality stay above the floor;
//  3. the frame rate itself.
//
// Candidates are tried highest fps first, sharpest first within an fps.
type Policy struct {
	// FPSLadder lists candidate frame rates, highest (preferred) first.
	FPSLadder []int
	// Blurs lists candidate pre-blur sigmas, ascending; 0 means none.
	Blurs []float64
	// QMax is the best quality worth spending bytes on.
	QMax int
	// QFloor is the lowest quality accepted while a candidate is left.
	QFloor int
	// QFallbackMin is how low quality may go on the last candidate, as a
	// last resort before the caller shortens the clip.
	QFallbackMin int
}

var DefaultPolicy = Policy{
	FPSLadder:    []int{15, 12, 10, 8, 6, 5},
	Blurs:        []float64{0, 0.6, 1.0},
	QMax:         90,
	QFloor:       55,
	QFallbackMin: 35,
}

type Attempt struct {
	FPS     int
	Blur    float64
	Quality int
}

// better reports whether a is preferable to b: higher fps first (motion
// reads more than compression noise), then less blur, then higher quality.
func better(a, b Attempt) bool {
	if a.FPS != b.FPS {
		return a.FPS > b.FPS
	}
	if a.Blur != b.Blur {
		return a.Blur < b.Blur
	}
	return a.Quality > b.Quality
}

// blurSizeFactor estimates how much a pre-blur shrinks the file, from
// measurements on real clips (σ=1.0 saved 8–21%, σ=1.4 saved 13–30%, more
// on heavy footage). It only decides which candidates are worth encoding;
// every fit is verified by actually encoding.
func blurSizeFactor(sigma float64) float64 { return max(0.6, 1-0.16*sigma) }

// skipMargin: a candidate whose predicted size exceeds budget × skipMargin
// is skipped without encoding. Generous, because the prediction is rough.
const skipMargin = 1.25

// Encoder encodes one attempt and returns the resulting file size.
type Encoder func(ctx context.Context, a Attempt) (int64, error)

type candidate struct {
	fps  int
	blur float64
}

func (p Policy) candidates() []candidate {
	var out []candidate
	for _, f := range p.FPSLadder {
		for _, b := range p.Blurs {
			out = append(out, candidate{f, b})
		}
	}
	return out
}

// Search finds the best attempt whose encoded size is <= budget, or nil if
// none fits. It is deterministic and bounded:
//
//  1. Encode the first candidate (top fps, no blur) at QFloor; its size S0
//     calibrates a prediction for every other candidate (size scales with
//     fps, and shrinks with blur).
//  2. Walk the candidates in preference order, skipping those predicted far
//     over budget and encoding the rest at QFloor, until one fits.
//  3. Binary-search quality upward on that candidate to spend the rest of
//     the budget.
//  4. If nothing fits at QFloor, search quality below it on the last
//     candidate (lowest fps, most blur), down to QFallbackMin.
func Search(ctx context.Context, p Policy, budget int64, enc Encoder) (*Attempt, error) {
	cands := p.candidates()
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

	// searchQuality finds the highest fitting quality in (lo, hi] on c,
	// given that lo is known to fit and hi is known not to.
	searchQuality := func(c candidate, lo, hi int) error {
		for hi-lo > 2 {
			mid := (lo + hi) / 2
			_, fit, err := try(Attempt{c.fps, c.blur, mid})
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

	ascend := func(c candidate) error {
		_, top, err := try(Attempt{c.fps, c.blur, p.QMax})
		if err != nil || top {
			return err
		}
		return searchQuality(c, p.QFloor, p.QMax)
	}

	first := cands[0]
	s0, fit, err := try(Attempt{first.fps, first.blur, p.QFloor})
	if err != nil {
		return nil, err
	}
	if fit {
		return best, ascend(first)
	}

	for _, c := range cands[1:] {
		predicted := float64(s0) * float64(c.fps) / float64(first.fps) * blurSizeFactor(c.blur)
		if predicted > float64(budget)*skipMargin {
			continue
		}
		if _, fit, err = try(Attempt{c.fps, c.blur, p.QFloor}); err != nil {
			return nil, err
		}
		if fit {
			return best, ascend(c)
		}
	}

	// Below the quality floor, on the last candidate.
	last := cands[len(cands)-1]
	if _, fit, err = try(Attempt{last.fps, last.blur, p.QFallbackMin}); err != nil || !fit {
		return nil, err
	}
	return best, searchQuality(last, p.QFallbackMin, p.QFloor)
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
