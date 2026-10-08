package group

import "testing"

func TestLookAlikeEmptyName(t *testing.T) {
	key, caution := LookAlikeKey("", nil)
	if key != "" || caution != "" {
		t.Fatalf("empty name key %q caution %q", key, caution)
	}
	if got := shorten("", nil); got != "" {
		t.Fatalf("shorten empty = %q", got)
	}
	if got := cautionSentence(""); got != "" {
		t.Fatalf("caution empty = %q", got)
	}
}

func TestLookAlikeCutGreenBeans(t *testing.T) {
	names := []string{
		"Kroger Cut Green Beans",
		"Great Value Cut Green Beans 14.5 oz",
		"Del Monte Cut Green Beans 29 oz",
	}
	got := ResolveLookAlikes(names, nil)
	key := got[0].Key
	if key != "cut green beans" {
		t.Fatalf("kroger key %q", key)
	}
	for _, m := range got {
		if m.Key != key {
			t.Fatalf("%s key %q, want %q", m.Name, m.Key, key)
		}
		if m.Caution != "" {
			t.Fatalf("%s caution %q", m.Name, m.Caution)
		}
	}
}

func TestLookAlikeSizesShareAKey(t *testing.T) {
	got := ResolveLookAlikes([]string{
		"Kroger Cut Green Beans 14.5 oz",
		"Kroger Cut Green Beans 29 oz",
	}, nil)
	if got[0].Key == "" || got[0].Key != got[1].Key {
		t.Fatalf("keys %#v", got)
	}
}

func TestLookAlikeCrunchyCaution(t *testing.T) {
	got := ResolveLookAlikes([]string{"Peanut Butter", "Crunchy Peanut Butter"}, nil)
	if got[0].Key != "peanut butter" || got[1].Key != got[0].Key {
		t.Fatalf("keys %#v", got)
	}
	if got[0].Caution != "" {
		t.Fatalf("plain caution %q", got[0].Caution)
	}
	if got[1].Caution != "Crunchy. Probably not the same." {
		t.Fatalf("caution %q", got[1].Caution)
	}
}

func TestLookAlikeFrenchStyleDoesNotShareCut(t *testing.T) {
	got := ResolveLookAlikes([]string{
		"Kroger Cut Green Beans",
		"French Style Green Beans",
		"Green Beans",
	}, nil)
	if got[0].Key != "cut green beans" {
		t.Fatalf("cut key %q", got[0].Key)
	}
	if got[1].Key == got[0].Key {
		t.Fatalf("french style shared cut key %q", got[1].Key)
	}
	if got[1].Key != "green beans" || got[2].Key != "green beans" {
		t.Fatalf("green keys %#v", got)
	}
	if got[1].Caution != "French style. Probably not the same." {
		t.Fatalf("caution %q", got[1].Caution)
	}
	if got[2].Key == got[0].Key {
		t.Fatalf("plain green beans matched cut")
	}
}

func TestLookAlikeUsesExistingGroupKey(t *testing.T) {
	got := ResolveLookAlikes([]string{"Del Monte Cut Green Beans"}, []string{"cut green beans"})
	if got[0].Key != "cut green beans" {
		t.Fatalf("key %q", got[0].Key)
	}
}

func TestLookAlikeKeyAndHalfGallon(t *testing.T) {
	key, caution := LookAlikeKey("Del Monte Cut Green Beans", map[string]struct{}{"cut green beans": {}})
	if key != "cut green beans" || caution != "" {
		t.Fatalf("key %q caution %q", key, caution)
	}
	got := ResolveLookAlikes([]string{"Great Value Milk half gallon"}, nil)
	if got[0].Key != "milk" || got[0].Caution != "" {
		t.Fatalf("%#v", got[0])
	}
}

func TestLookAlikeDoesNotInventAKey(t *testing.T) {
	got := ResolveLookAlikes([]string{"Del Monte Cut Green Beans", "Hunt's Cut Green Beans"}, nil)
	if got[0].Key == got[1].Key {
		t.Fatalf("invented a shared key %q", got[0].Key)
	}
}
