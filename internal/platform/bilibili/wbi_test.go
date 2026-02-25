package bilibili

import "testing"

func TestCalcMixinKey(t *testing.T) {
	imgKey := "7cd084941338484aae1ad9425b84077c"
	subKey := "4932caff0ff746eab6f01bf08b70ac45"
	got := calcMixinKey(imgKey, subKey)
	want := "ea1db124af3c7062474693fa704f4ff8"
	if got != want {
		t.Fatalf("mixin key mismatch: got %s want %s", got, want)
	}
}

func TestSignParamsExample(t *testing.T) {
	params := map[string]string{
		"foo": "114",
		"bar": "514",
		"zab": "1919810",
	}
	mixinKey := "ea1db124af3c7062474693fa704f4ff8"
	wts := int64(1702204169)
	query := signParams(params, mixinKey, wts)
	want := "bar=514&foo=114&wts=1702204169&zab=1919810&w_rid=8f6f2b5b3d485fe1886cec6a0be8c5d4"
	if query != want {
		t.Fatalf("signed query mismatch:\n got: %s\nwant: %s", query, want)
	}
}

func TestEncodeComponent(t *testing.T) {
	input := "测试 123!()"
	encoded := encodeComponent(sanitizeValue(input))
	if encoded == input {
		t.Fatalf("expected encoded output")
	}
}
