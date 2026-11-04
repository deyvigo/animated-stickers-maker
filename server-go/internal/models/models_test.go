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
