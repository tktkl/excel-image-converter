package converter

import "testing"

func TestExtractLiteralImageURL(t *testing.T) {
	tests := []struct {
		name    string
		formula string
		want    string
	}{
		{
			name:    "simple",
			formula: `=IMAGE("https://example.com/a.png")`,
			want:    "https://example.com/a.png",
		},
		{
			name:    "with extra args",
			formula: `=_xlfn.IMAGE("https://example.com/a.png","alt",1,100,100)`,
			want:    "https://example.com/a.png",
		},
		{
			name:    "escaped quote",
			formula: `=IMAGE("https://example.com/a""b.png")`,
			want:    `https://example.com/a"b.png`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractLiteralImageURL(tt.formula)
			if err != nil {
				t.Fatalf("extractLiteralImageURL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("extractLiteralImageURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractLiteralImageURLRejectsReferences(t *testing.T) {
	_, err := extractLiteralImageURL(`=IMAGE(A1)`)
	if err == nil {
		t.Fatal("expected reference formula to be rejected")
	}
}

func TestExtractPlainImageURL(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
		ok    bool
	}{
		{
			name:  "plain jpg",
			value: "https://example.com/a.jpg",
			want:  "https://example.com/a.jpg",
			ok:    true,
		},
		{
			name:  "query after extension",
			value: "https://example.com/a.png?x=1&token=abc",
			want:  "https://example.com/a.png?x=1&token=abc",
			ok:    true,
		},
		{
			name:  "fragment after extension",
			value: "https://example.com/a.webp#preview",
			want:  "https://example.com/a.webp#preview",
			ok:    true,
		},
		{
			name:  "not image extension",
			value: "https://example.com/page?id=a.png",
			ok:    false,
		},
		{
			name:  "not http",
			value: "ftp://example.com/a.jpg",
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractPlainImageURL(tt.value)
			if ok != tt.ok {
				t.Fatalf("extractPlainImageURL() ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("extractPlainImageURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
