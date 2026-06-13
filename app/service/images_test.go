package service

import "testing"

func TestFindImageFileIDFromNestedFileID(t *testing.T) {
	value := map[string]interface{}{
		"message": map[string]interface{}{
			"content": map[string]interface{}{
				"parts": []interface{}{
					map[string]interface{}{"file_id": "file_123456"},
				},
			},
		},
	}
	if got := findImageFileID(value); got != "file_123456" {
		t.Fatalf("expected file_123456, got %q", got)
	}
}

func TestFindImageFileIDFromAssetPointer(t *testing.T) {
	value := map[string]interface{}{
		"asset_pointer": "file-service://file_abcdef",
	}
	if got := findImageFileID(value); got != "file_abcdef" {
		t.Fatalf("expected file_abcdef, got %q", got)
	}
}

func TestFileIDFromPointer(t *testing.T) {
	if got := fileIDFromPointer("sediment://file_xyz"); got != "file_xyz" {
		t.Fatalf("expected file_xyz, got %q", got)
	}
	if got := fileIDFromPointer("https://example.com/a.png"); got != "" {
		t.Fatalf("expected empty file id, got %q", got)
	}
}
