package protocol

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/buildinfo"
)

func TestAIHubKitVideoDocumentedRequest(t *testing.T) {
	a := officialPackageAdapter(t, "aihubkit-video.beeftv-plugin", "aihubkit-video")
	if !a.Metadata().RequiresPublicMediaURLs {
		t.Fatal("local references must be published by the host before submission")
	}
	request := GenerationRequest{
		Capability: CapabilityVideo, Model: "seedance-2.5_v1", Prompt: "Cinematic action scene",
		Duration: 6, AspectRatio: "16:9", Resolution: "720",
		Images: []MediaReference{
			{URL: "https://example.com/second.jpg", Role: "reference_image", Order: 2},
			{URL: "https://example.com/person.jpg", Order: 1},
		},
		Videos: []MediaReference{{URL: "https://example.com/demo.mp4"}},
		Audios: []MediaReference{{URL: "https://example.com/voice.mp3"}},
	}
	spec, err := a.BuildCreate(context.Background(), RequestContext{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"model": "seedance-2.5_v1", "prompt": "Cinematic action scene", "duration": float64(6),
		"ratio": "16:9", "resolution": "720p",
		"referenceImages": []any{"https://example.com/person.jpg", "https://example.com/second.jpg"},
		"referenceVideos": []any{"https://example.com/demo.mp4"},
		"referenceAudios": []any{"https://example.com/voice.mp3"},
	}
	if spec.Method != "POST" || spec.Path != "/v1/videos" || spec.ContentType != "application/json" || len(spec.Files) != 0 || spec.Auth.Type != "bearer" {
		t.Fatalf("unexpected transport: %+v", spec)
	}
	if body := manifestTestBody(t, spec); !reflect.DeepEqual(body, want) {
		t.Fatalf("request differs from documented contract: got %#v, want %#v", body, want)
	}
	request.Images, request.Videos, request.Audios, request.AspectRatio = nil, nil, nil, ""
	request.Resolution = "1080P"
	spec, err = a.BuildCreate(context.Background(), RequestContext{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	body := manifestTestBody(t, spec)
	if len(body) != 4 || body["resolution"] != "1080p" {
		t.Fatalf("text-to-video must omit optional media and ratio: %#v", body)
	}
}

func TestAIHubKitVideoForwardsFramesAndServerParameters(t *testing.T) {
	a := officialPackageAdapter(t, "aihubkit-video.beeftv-plugin", "aihubkit-video")
	request := GenerationRequest{Capability: CapabilityVideo, Model: "server-selected-model", Prompt: "move", Duration: 4, Resolution: "480p",
		Images: []MediaReference{
			{URL: "https://example.com/last.jpg", Role: "last_frame", Order: 2},
			{URL: "https://example.com/first.jpg", Role: "first_frame", Order: 1},
		},
	}
	spec, err := a.BuildCreate(context.Background(), RequestContext{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	body := manifestTestBody(t, spec)
	if body["first_image"] != "https://example.com/first.jpg" || body["last_image"] != "https://example.com/last.jpg" || body["referenceImages"] != nil {
		t.Fatalf("frame roles were lost or duplicated: %#v", body)
	}
	// Limits and combinations are decided by the relay, without client model rules.
	request.Duration = 0
	request.Resolution = "server-specific"
	request.Images = append(request.Images, MediaReference{URL: "https://example.com/ref.jpg", Role: "reference_image"})
	spec, err = a.BuildCreate(context.Background(), RequestContext{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	body = manifestTestBody(t, spec)
	if body["duration"] != float64(0) || body["resolution"] != "server-specific" || !reflect.DeepEqual(body["referenceImages"], []any{"https://example.com/ref.jpg"}) {
		t.Fatalf("server parameters were not preserved: %#v", body)
	}
}

func TestAIHubKitVideoErrorUsesCurrentProductName(t *testing.T) {
	t.Cleanup(func() { buildinfo.SetDisplayName("") })
	a := officialPackageAdapter(t, "aihubkit-video.beeftv-plugin", "aihubkit-video")
	for _, name := range []string{"ZModelTV", "shanzhuTV"} {
		buildinfo.SetDisplayName(name)
		request := GenerationRequest{Capability: CapabilityVideo, Model: "video", Prompt: "move", Duration: 4, Resolution: "480p",
			Images: []MediaReference{{URL: "https://example.com/mask.png", Role: "mask"}},
		}
		_, err := a.BuildCreate(context.Background(), RequestContext{Request: request})
		if err == nil || !strings.HasPrefix(err.Error(), name+" Video ") || a.Metadata().Name != name+" Video" {
			t.Fatalf("missing current product presentation: %v", err)
		}
	}
}

func TestAIHubKitVideoDocumentedLifecycle(t *testing.T) {
	a := officialPackageAdapter(t, "aihubkit-video.beeftv-plugin", "aihubkit-video")
	ctx := context.Background()
	created, err := a.ParseCreate(ctx, []byte(`{"id":"task_example","task_id":"task_example","status":"queued","progress":20}`))
	if err != nil || created.TaskID != "task_example" || created.Status != StatusPending {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	c := PollContext{TaskID: created.TaskID}
	poll, err := a.BuildPoll(ctx, c)
	if err != nil || poll.Method != "GET" || poll.Path != "/v1/videos/task_example" {
		t.Fatalf("poll=%+v err=%v", poll, err)
	}
	for _, tc := range []struct {
		body string
		want Status
	}{
		{`{"status":"in_progress","progress":47}`, StatusProcessing},
		{`{"status":"unknown"}`, StatusPending},
		{`{"status":"failed","error":{"message":"provider rejected request"}}`, StatusFailed},
	} {
		state, err := a.ParsePoll(ctx, c, []byte(tc.body))
		if err != nil || state.Status != tc.want || state.TaskID != c.TaskID {
			t.Fatalf("state=%+v err=%v", state, err)
		}
		if tc.want == StatusFailed && state.Message != "provider rejected request" {
			t.Fatalf("lost failure reason: %+v", state)
		}
	}
	state, err := a.ParsePoll(ctx, c, []byte(`{"id":"task_example","status":"completed","url":"https://apimodel.aihubkit.com/v1/videos/task_example/content","usage":{"total_tokens":40594}}`))
	if err != nil || state.Status != StatusSucceeded || state.Result == nil || len(state.Result.Videos) != 1 {
		t.Fatalf("completed=%+v err=%v", state, err)
	}
	if state.Result.Videos[0].URL != "https://apimodel.aihubkit.com/v1/videos/task_example/content" || !state.Result.Videos[0].Ephemeral || state.Result.Usage["total_tokens"] != float64(40594) {
		t.Fatalf("result=%+v", state.Result)
	}
	download, err := a.(ResultAdapter).BuildResult(ctx, c)
	if err != nil || download.Path != "/v1/videos/task_example/content" || download.Method != "GET" || download.Auth.Type != "bearer" {
		t.Fatalf("download=%+v err=%v", download, err)
	}
}
