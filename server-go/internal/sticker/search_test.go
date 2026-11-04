package sticker

import (
	"context"
	"errors"
	"testing"
)

const budget = 500 * 1024

// model returns a monotonic fake size: bytes grow with fps, duration and
// quality, like the real encoder.
func model(dur float64) Encoder {
	return func(_ context.Context, a Attempt) (int64, error) {
		return int64(float64(a.FPS) * dur * float64(200+60*a.Quality)), nil
	}
}

func counting(e Encoder, n *int) Encoder {
	return func(ctx context.Context, a Attempt) (int64, error) { *n++; return e(ctx, a) }
}

func TestShortClipKeepsTopFPSAndMaxQuality(t *testing.T) {
	var n int
	best, err := Search(context.Background(), DefaultPolicy, budget, counting(model(2), &n))
	if err != nil || best == nil {
		t.Fatalf("best=%v err=%v", best, err)
	}
	if *best != (Attempt{15, 90}) {
		t.Fatalf("best = %+v, want 15fps q90", *best)
	}
	if n != 2 {
		t.Errorf("encodes = %d, want 2", n)
	}
}

func TestLongClipTradesFPSNotQuality(t *testing.T) {
	var n int
	best, err := Search(context.Background(), DefaultPolicy, budget, counting(model(10), &n))
	if err != nil || best == nil {
		t.Fatalf("best=%v err=%v", best, err)
	}
	if best.FPS >= 15 {
		t.Errorf("expected fps reduction, got %+v", *best)
	}
	if best.Quality < DefaultPolicy.QFloor {
		t.Errorf("quality %d below floor", best.Quality)
	}
	if n > 10 {
		t.Errorf("too many encodes: %d", n)
	}
}

// For many durations: the result must fit, and nothing better may fit —
// no higher fps at the floor quality, and no more than 2 quality steps
// left on the table at the chosen fps.
func TestSearchIsNearOptimal(t *testing.T) {
	for dur := 1.0; dur <= 10; dur += 0.5 {
		enc := model(dur)
		best, err := Search(context.Background(), DefaultPolicy, budget, enc)
		if err != nil {
			t.Fatal(err)
		}
		if best == nil {
			t.Fatalf("dur %.1f: no fit", dur)
		}
		if size, _ := enc(context.Background(), *best); size > budget {
			t.Errorf("dur %.1f: %+v is %d bytes, over budget", dur, *best, size)
		}
		for _, fps := range DefaultPolicy.FPSLadder {
			if fps > best.FPS {
				if size, _ := enc(context.Background(), Attempt{fps, DefaultPolicy.QFloor}); size <= budget {
					t.Errorf("dur %.1f: %dfps fits at floor but chose %+v", dur, fps, *best)
				}
			}
		}
		if best.Quality < DefaultPolicy.QMax {
			if size, _ := enc(context.Background(), Attempt{best.FPS, best.Quality + 3}); size <= budget {
				t.Errorf("dur %.1f: q+3 would fit at %+v", dur, *best)
			}
		}
	}
}

func TestFallbackBelowFloorAtLowestFPS(t *testing.T) {
	// Only fits at 5fps and quality below the floor.
	enc := func(_ context.Context, a Attempt) (int64, error) {
		return int64(a.FPS*100000) + int64(a.Quality)*1000, nil
	}
	// 5fps: 500000 + q*1000 <= 512000 -> q <= 12 ... widen budget instead.
	best, err := Search(context.Background(), DefaultPolicy, 500000+50*1000, enc)
	if err != nil || best == nil {
		t.Fatalf("best=%v err=%v", best, err)
	}
	if best.FPS != 5 || best.Quality >= DefaultPolicy.QFloor || best.Quality < DefaultPolicy.QFallbackMin {
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
