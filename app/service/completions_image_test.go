package service

import (
	"testing"

	"chat2api/app/types/completions"
)

func TestIsChatCompletionsImageRequestByModel(t *testing.T) {
	req := &completions.ApiReq{Model: "gpt-image-2"}
	if !isChatCompletionsImageRequest(req) {
		t.Fatal("expected gpt-image model to route through image bridge")
	}
}

func TestIsChatCompletionsImageRequestByTool(t *testing.T) {
	req := &completions.ApiReq{Tools: []completions.Tool{{Type: "image_generation"}}}
	if !isChatCompletionsImageRequest(req) {
		t.Fatal("expected image_generation tool to route through image bridge")
	}
}

func TestChatCompletionsImagePrompt(t *testing.T) {
	messages := []completions.ApiMessage{{
		Role: "user",
		Content: []interface{}{
			map[string]interface{}{"type": "input_text", "text": "draw a cat"},
			map[string]interface{}{"type": "input_image", "image_url": "data:image/png;base64,abc"},
		},
	}}
	prompt, images := chatCompletionsImagePrompt(messages)
	if prompt != "draw a cat" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
	if len(images) != 1 || images[0] != "data:image/png;base64,abc" {
		t.Fatalf("unexpected images: %#v", images)
	}
}

func TestChatImageTaskFromRawEvent(t *testing.T) {
	event := map[string]interface{}{
		"v": map[string]interface{}{
			"message": map[string]interface{}{
				"metadata": map[string]interface{}{
					"image_gen_task_id":      "task-123",
					"image_gen_multi_stream": true,
				},
			},
		},
	}
	if !chatImageTaskFromRawEvent(event) {
		t.Fatal("expected image task event to be detected")
	}
}
