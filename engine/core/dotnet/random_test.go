package dotnet

import "testing"

func TestRandomSeedSequence(t *testing.T) {
	cases := []struct {
		seed int32
		want []int
	}{
		{42, []int{66, 14, 12, 52, 16, 26, 72, 51, 17, 76, 23, 25}},
		{161803398, []int{76, 17, 78, 0, 4, 18, 7, 54, 49, 88, 12, 56}},
	}
	for _, c := range cases {
		r := NewRandomSeed(c.seed)
		for i, want := range c.want {
			if got := r.Next(100); got != want {
				t.Fatalf("seed %d Next(100) #%d = %d, want %d", c.seed, i, got, want)
			}
		}
	}
}

func TestRandomSeedSample(t *testing.T) {
	cases := []struct {
		seed int32
		head float64
	}{
		{42, 0.66810646591154232},
		{161803398, 0.76326259028318455},
	}
	for _, c := range cases {
		r := NewRandomSeed(c.seed)
		first := r.Sample()
		if first != c.head {
			t.Fatalf("seed %d first Sample = %.17g, want %.17g", c.seed, first, c.head)
		}
		r = NewRandomSeed(c.seed)
		for i := 0; i < 64; i++ {
			sample := r.Sample()
			if sample < 0 || sample >= 1 {
				t.Fatalf("seed %d Sample #%d = %.17g, want in [0,1)", c.seed, i, sample)
			}
			if double := r.NextDouble(); double < 0 || double >= 1 {
				t.Fatalf("seed %d NextDouble #%d = %.17g, want in [0,1)", c.seed, i, double)
			}
		}
	}
}

func TestRandomNextEdges(t *testing.T) {
	r := NewRandomSeed(42)
	for i := 0; i < 8; i++ {
		if got := r.Next(0); got != 0 {
			t.Fatalf("Next(0) #%d = %d, want 0", i, got)
		}
		if got := r.Next(1); got != 0 {
			t.Fatalf("Next(1) #%d = %d, want 0", i, got)
		}
		if got := r.Next(-7); got != 0 {
			t.Fatalf("Next(-7) #%d = %d, want 0", i, got)
		}
	}
}

func TestRandomSeedRepeatable(t *testing.T) {
	a := NewRandomSeed(42)
	b := NewRandomSeed(42)
	for i := 0; i < 32; i++ {
		if av, bv := a.Next(1000), b.Next(1000); av != bv {
			t.Fatalf("Next(1000) #%d = %d vs %d, want identical", i, av, bv)
		}
		if av, bv := a.NextDouble(), b.NextDouble(); av != bv {
			t.Fatalf("NextDouble #%d = %.17g vs %.17g, want identical", i, av, bv)
		}
	}
}

func TestNewRandomInRange(t *testing.T) {
	r := NewRandom()
	for i := 0; i < 64; i++ {
		sample := r.Sample()
		if sample < 0 || sample >= 1 {
			t.Fatalf("Sample #%d = %.17g, want in [0,1)", i, sample)
		}
		if n := r.Next(100); n < 0 || n >= 100 {
			t.Fatalf("Next(100) #%d = %d, want in [0,100)", i, n)
		}
	}
}
