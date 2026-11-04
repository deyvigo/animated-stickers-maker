package sticker

import (
	"context"
	"errors"
	"testing"
)

const budget = 500 * 1024

// model returns a monotonic fake size: bytes grow with fps, duration and
// quality and shrink with blur, like the real encoder.
func model(dur float64) Encoder {
	return func(_ context.Context, a Attempt) (int64, error) {
		return int64(float64(a.FPS) * dur * float64(200+60*a.Quality) * blurSizeFactor(a.Blur)), nil
	}
}

func counting(e Encoder, n *int) Encoder {
	return func(ctx context.Context, a Attempt) (int64, error) { *n++; return e(ctx, a) }
}

func TestShortClipKeepsTopFPSNoBlurMaxQuality(t *testing.T) {
	var n int
	best, err := Search(context.Background(), DefaultPolicy, budget, counting(model(2), &n))
	if err != nil || best == nil {
		t.Fatalf("best=%v err=%v", best, err)
	}
	if *best != (Attempt{15, 0, 90}) {
		t.Fatalf("best = %+v, want 15fps no blur q90", *best)
	}
	if n != 2 {
		t.Errorf("encodes = %d, want 2", n)
	}
}

func TestHeavyClipPrefersBlurOverLosingFPS(t *testing.T) {
	// The model's "duration" is just a size scale. Pick one where 15fps
	// without blur doesn't fit at the floor, but blur recovers it before
	// any fps is given up.
	dur := 10.5
	enc := model(dur)
	if s, _ := enc(context.Background(), Attempt{15, 0, 55}); s <= budget {
		t.Fatalf("test premise broken: 15fps/no blur already fits (%d)", s)
	}
	if s, _ := enc(context.Background(), Attempt{15, 1.0, 55}); s > budget {
		t.Fatalf("test premise broken: 15fps/blur 1.0 doesn't fit (%d)", s)
	}
	best, err := Search(context.Background(), DefaultPolicy, budget, enc)
	if err != nil || best == nil {
		t.Fatal(err)
	}
	if best.FPS != 15 || best.Blur == 0 || best.Quality < DefaultPolicy.QFloor {
		t.Fatalf("best = %+v, want 15fps with some blur and q>=floor", *best)
	}
}

// For many durations: the result must fit, nothing preferred may fit at the
// floor quality, and no more than 2 quality steps may be left on the table.
func TestSearchIsNearOptimal(t *testing.T) {
	p := DefaultPolicy
	for dur := 0.5; dur <= 10; dur += 0.5 {
		enc := model(dur)
		best, err := Search(context.Background(), p, budget, enc)
		if err != nil {
			t.Fatal(err)
		}
		if best == nil {
			t.Fatalf("dur %.1f: no fit", dur)
		}
		if size, _ := enc(context.Background(), *best); size > budget {
			t.Errorf("dur %.1f: %+v is %d bytes, over budget", dur, *best, size)
		}
		for _, c := range p.candidates() {
			a := Attempt{c.fps, c.blur, p.QFloor}
			if better(a, *best) && a.Quality == p.QFloor && (a.FPS != best.FPS || a.Blur != best.Blur) {
				if size, _ := enc(context.Background(), a); size <= budget {
					t.Errorf("dur %.1f: %+v fits at the floor but chose %+v", dur, a, *best)
				}
			}
		}
		if best.Quality < p.QMax {
			up := *best
			up.Quality += 3
			if size, _ := enc(context.Background(), up); size <= budget {
				t.Errorf("dur %.1f: q+3 would fit at %+v", dur, *best)
			}
		}
	}
}

func TestEncodeCountStaysBounded(t *testing.T) {
	for dur := 0.5; dur <= 10; dur += 0.5 {
		var n int
		if _, err := Search(context.Background(), DefaultPolicy, budget, counting(model(dur), &n)); err != nil {
			t.Fatal(err)
		}
		if n > 14 {
			t.Errorf("dur %.1f: %d encodes", dur, n)
		}
	}
}

func TestFallbackBelowFloorOnLastCandidate(t *testing.T) {
	enc := func(_ context.Context, a Attempt) (int64, error) {
		return int64(a.FPS*100000) + int64(a.Quality)*1000, nil
	}
	best, err := Search(context.Background(), DefaultPolicy, 500000+50*1000, enc)
	if err != nil || best == nil {
		t.Fatalf("best=%v err=%v", best, err)
	}
	last := Attempt{FPS: 5, Blur: 1.0}
	if best.FPS != last.FPS || best.Blur != last.Blur ||
		best.Quality >= DefaultPolicy.QFloor || best.Quality < DefaultPolicy.QFallbackMin {
		t.Fatalf("best = %+v", *best)
	}
}

func TestNothingFits(t *testing.T) {
	huge := func(context.Context, Attempt) (int64, error) { return 10 * budget, nil }
	best, err := Search(context.Background(), DefaultPolicy, budget, huge)
	if err != nil || best != nil {
		t.Fatalf("best=%v err=%v, want nil,nil", best, err)
	}
}

func TestEncoderErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	_, err := Search(context.Background(), DefaultPolicy, budget,
		func(context.Context, Attempt) (int64, error) { return 0, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestSearchQuality(t *testing.T) {
	size := func(_ context.Context, q int) (int64, error) { return int64(q) * 1000, nil }
	ctx := context.Background()

	if q, _ := SearchQuality(ctx, 20, 90, 95000, size); q != 90 {
		t.Errorf("everything fits: q=%d", q)
	}
	if q, _ := SearchQuality(ctx, 20, 90, 10000, size); q != -1 {
		t.Errorf("nothing fits: q=%d", q)
	}
	q, _ := SearchQuality(ctx, 20, 90, 63500, size)
	if q < 61 || q > 63 {
		t.Errorf("q=%d, want within 2 of 63", q)
	}
}
