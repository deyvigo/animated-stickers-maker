package models

import "testing"

func f(v float64) *float64 { return &v }

func TestCreateRenderRequestValidate(t *testing.T) {
	crop := CropRect{X: 0, Y: 0, Width: 1, Height: 1}

	t.Run("animated needs end > start", func(t *testing.T) {
		r := CreateRenderRequest{Type: Animated, Start: 2, End: f(2), Crop: crop}
		if r.Validate() == nil {
			t.Fatal("expected error for end == start")
		}
		r.End = nil
		if r.Validate() == nil {
			t.Fatal("expected error for missing end")
		}
		r.End = f(5)
		if err := r.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("static defaults frame_at to start", func(t *testing.T) {
		r := CreateRenderRequest{Type: Static, Start: 3.5, Crop: crop}
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
		if r.FrameAt == nil || *r.FrameAt != 3.5 {
			t.Fatalf("frame_at = %v, want 3.5", r.FrameAt)
		}
	})

	t.Run("rejects bad type, crop and emoji count", func(t *testing.T) {
		if (&CreateRenderRequest{Type: "gif", Crop: crop}).Validate() == nil {
			t.Fatal("bad type accepted")
		}
		bad := CreateRenderRequest{Type: Static, Crop: CropRect{Width: 0, Height: 1}}
		if bad.Validate() == nil {
			t.Fatal("zero-width crop accepted")
		}
		many := CreateRenderRequest{Type: Static, Crop: crop, Emojis: []string{"a", "b", "c", "d"}}
		if many.Validate() == nil {
			t.Fatal("4 emojis accepted")
		}
	})

	t.Run("crop may extend outside the frame", func(t *testing.T) {
		r := CreateRenderRequest{Type: Static, Crop: CropRect{X: -0.5, Y: -1, Width: 2, Height: 2.5}}
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCreateJobRequestValidate(t *testing.T) {
	for url, ok := range map[string]bool{
		"https://www.tiktok.com/@a/video/1": true,
		"http://vm.tiktok.com/abc":          true,
		"ftp://tiktok.com/x":                false,
		"not a url":                         false,
		"":                                  false,
	} {
		if err := (CreateJobRequest{URL: url}).Validate(); (err == nil) != ok {
			t.Errorf("Validate(%q) err=%v, want ok=%v", url, err, ok)
		}
	}
}

func TestCreateRenderRequestSegments(t *testing.T) {
	crop := CropRect{X: 0, Y: 0, Width: 1, Height: 1}
	mk := func(segs ...Segment) CreateRenderRequest {
		return CreateRenderRequest{Type: Animated, Segments: segs, Crop: crop}
	}

	r := CreateRenderRequest{Type: Animated, Start: 1, End: f(2), Crop: crop}
	if err := r.Validate(); err != nil || len(r.Segments) != 1 || r.Segments[0] != (Segment{1, 2}) {
		t.Errorf("legacy start/end -> %v, %v", r.Segments, err)
	}

	r = mk(Segment{4, 5}, Segment{1, 2})
	if err := r.Validate(); err != nil || r.Segments[0].Start != 1 || r.Start != 1 || *r.End != 5 {
		t.Errorf("unsorted input must be sorted: %+v %v", r, err)
	}

	for name, r := range map[string]CreateRenderRequest{
		"overlap":   mk(Segment{1, 3}, Segment{2, 4}),
		"empty seg": mk(Segment{1, 1}),
		"negative":  mk(Segment{-1, 1}),
		"too many":  mk(make([]Segment, MaxSegments+1)...),
	} {
		if r.Validate() == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
