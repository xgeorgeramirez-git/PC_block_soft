package sites

import "testing"

func TestKindStrings(t *testing.T) {
	cases := []struct {
		k Kind
		s string
	}{{KindBlocked, "blocked"}, {KindStudy, "study"}, {KindVideo, "video_social"}}
	for _, c := range cases {
		if got := c.k.String(); got != c.s {
			t.Errorf("%d.String() = %q, want %q", int(c.k), got, c.s)
		}
		if k, ok := KindFromString(c.s); !ok || k != c.k {
			t.Errorf("KindFromString(%q) = %d, %v", c.s, int(k), ok)
		}
	}
	if k, ok := KindFromString("x"); ok || k != 0 {
		t.Errorf("KindFromString(x) = %d, %v; want 0, false", int(k), ok)
	}
	if got := Kind(0).String(); got != "" {
		t.Errorf("Kind(0).String() = %q, want пусто", got)
	}
}

// Самая длинная совпавшая запись своих списков (RM4-5): домен, затем родители.
func TestListsMatch(t *testing.T) {
	l := NewLists([]Rule{
		{"youtube.com", KindVideo}, {"edu.youtube.com", KindStudy}, {"example.com", KindBlocked},
		{"192.168.0.1", KindBlocked}, {"", KindBlocked},
	})
	cases := []struct {
		in     string
		domain string
		kind   Kind
	}{
		{"youtube.com", "youtube.com", KindVideo},
		{"m.youtube.com", "youtube.com", KindVideo},
		{"a.edu.youtube.com", "edu.youtube.com", KindStudy},
		{"edu.youtube.com", "edu.youtube.com", KindStudy},
		{"notyoutube.com", "", 0},
		{"com", "", 0},
		{"x.example.com", "example.com", KindBlocked},
		{"192.168.0.1", "192.168.0.1", KindBlocked},
		{"1.192.168.0.1", "", 0},
		{"", "", 0},
	}
	for _, c := range cases {
		r, ok := l.Match(c.in)
		if ok != (c.kind != 0) || r.Domain != c.domain || r.Kind != c.kind {
			t.Errorf("Match(%q) = %+v, %v; want {%s %d}", c.in, r, ok, c.domain, int(c.kind))
		}
	}
	if r, ok := (Lists{}).Match("youtube.com"); ok || r != (Rule{}) {
		t.Errorf("Lists{}.Match = %+v, %v", r, ok)
	}
}
