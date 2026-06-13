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
	if got := fileIDFromPointer("file-service://file_abcdef:preview"); got != "file_abcdef" {
		t.Fatalf("expected file_abcdef, got %q", got)
	}
	if got := fileIDFromPointer("https://example.com/a.png"); got != "" {
		t.Fatalf("expected empty file id, got %q", got)
	}
}

func TestFileIDFromURL(t *testing.T) {
	url := "https://chatgpt.com/backend-api/estuary/content?id=file_000000001a1c7207aab44ceac99cd97c&ts=494822"
	if got := fileIDFromURL(url); got != "file_000000001a1c7207aab44ceac99cd97c" {
		t.Fatalf("expected file id from url, got %q", got)
	}
}

func TestFindImageFileIDFromImageAssetPointerMetadata(t *testing.T) {
	value := map[string]interface{}{
		"message": map[string]interface{}{
			"content": map[string]interface{}{
				"content_type": "multimodal_text",
				"parts": []interface{}{
					map[string]interface{}{
						"content_type":  "image_asset_pointer",
						"asset_pointer": "file-service://file_asset123",
						"metadata": map[string]interface{}{
							"watermarked_asset_pointer": "file-service://file_watermarked456",
						},
					},
				},
			},
		},
	}
	if got := findImageFileID(value); got != "file_asset123" {
		t.Fatalf("expected file_asset123, got %q", got)
	}
}
