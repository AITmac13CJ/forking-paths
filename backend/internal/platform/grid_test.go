package platform

import "testing"

func TestParseArtMapPrefersPortrait(t *testing.T) {
	body := []byte(`{
		"success": true,
		"data": [
			{"url": "https://cdn2.steamgriddb.com/wide.jpg", "width": 660, "height": 930},
			{"url": "https://cdn2.steamgriddb.com/tall.jpg", "width": 600, "height": 900}
		]
	}`)
	got := parseArtMap(body, []string{"1091500"}, 600, 900)
	if got["1091500"] != "https://cdn2.steamgriddb.com/tall.jpg" {
		t.Fatalf("portrait = %q", got["1091500"])
	}
}

func TestParseArtMapBatch(t *testing.T) {
	body := []byte(`{
		"success": true,
		"data": [
			{"success": true, "status": 200, "data": [{"url": "https://cdn2.steamgriddb.com/a.jpg", "width": 600, "height": 900}]},
			{"success": false, "status": 404}
		]
	}`)
	got := parseArtMap(body, []string{"10", "20"}, 600, 900)
	if got["10"] != "https://cdn2.steamgriddb.com/a.jpg" {
		t.Fatalf("first = %q", got["10"])
	}
	if _, ok := got["20"]; ok {
		t.Fatal("missing game should stay empty")
	}
}

func TestPickAssetSkipsFlatMaterial(t *testing.T) {
	body := []byte(`{
		"success": true,
		"data": [
			{"url": "https://cdn2.steamgriddb.com/flat.jpg", "width": 920, "height": 430, "style": "material"},
			{"url": "https://cdn2.steamgriddb.com/drawn.jpg", "width": 920, "height": 430, "style": "alternate"}
		]
	}`)
	got := parseArtMap(body, []string{"1"}, 920, 430)
	if got["1"] != "https://cdn2.steamgriddb.com/drawn.jpg" {
		t.Fatalf("landscape = %q", got["1"])
	}
}

func TestGridURLKeepsOfficialHost(t *testing.T) {
	if got := GridURL("https://cdn2.steamgriddb.com/grid/a.jpg"); got != "https://cdn2.steamgriddb.com/grid/a.jpg" {
		t.Fatalf("url = %q", got)
	}
	if GridURL("http://cdn2.steamgriddb.com/a.jpg") != "" || GridURL("https://evil.example/a.jpg") != "" {
		t.Fatal("rejected host was kept")
	}
}

func TestGridHost(t *testing.T) {
	if !gridHost("cdn2.steamgriddb.com") || gridHost("evil.example") || gridHost("steamgriddb.com.evil") {
		t.Fatal("host check failed")
	}
}
