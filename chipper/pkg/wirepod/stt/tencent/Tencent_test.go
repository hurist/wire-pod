package wirepod_tencent

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestLoadTencentASRConfigRequiresSecrets(t *testing.T) {
	t.Setenv("TENCENTCLOUD_SECRET_ID", "")
	t.Setenv("TENCENTCLOUD_SECRET_KEY", "")

	_, err := loadTencentASRConfig()
	if err == nil {
		t.Fatal("expected missing SecretId error")
	}
}

func TestLoadTencentASRConfigDefaults(t *testing.T) {
	t.Setenv("TENCENTCLOUD_SECRET_ID", "sid")
	t.Setenv("TENCENTCLOUD_SECRET_KEY", "skey")
	t.Setenv("TENCENT_ASR_REGION", "")
	t.Setenv("TENCENT_ASR_ENGINE_MODEL_TYPE", "")
	t.Setenv("TENCENT_ASR_VOICE_FORMAT", "")
	t.Setenv("TENCENT_ASR_TIMEOUT_SECONDS", "")

	cfg, err := loadTencentASRConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Region != "ap-guangzhou" {
		t.Fatalf("region = %q", cfg.Region)
	}
	if cfg.EngineModelType != "16k_zh" {
		t.Fatalf("engine model = %q", cfg.EngineModelType)
	}
	if cfg.VoiceFormat != "pcm" {
		t.Fatalf("voice format = %q", cfg.VoiceFormat)
	}
	if cfg.Timeout != 15*time.Second {
		t.Fatalf("timeout = %s", cfg.Timeout)
	}
}

func TestBuildSentenceRecognitionRequest(t *testing.T) {
	filterPunc := int64(2)
	cfg := tencentASRConfig{
		EngineModelType: "16k_zh",
		VoiceFormat:     "pcm",
		FilterPunc:      &filterPunc,
	}
	audio := make([]byte, 640)

	req, err := buildSentenceRecognitionRequest(cfg, audio, "bot-session-1")
	if err != nil {
		t.Fatal(err)
	}
	if req.ProjectId == nil || *req.ProjectId != 0 {
		t.Fatal("ProjectId was not set to 0")
	}
	if req.SubServiceType == nil || *req.SubServiceType != 2 {
		t.Fatal("SubServiceType was not set to 2")
	}
	if req.SourceType == nil || *req.SourceType != 1 {
		t.Fatal("SourceType was not set to direct upload")
	}
	if req.DataLen == nil || *req.DataLen != int64(len(audio)) {
		t.Fatal("DataLen did not match raw audio length")
	}
	if req.Data == nil || *req.Data != base64.StdEncoding.EncodeToString(audio) {
		t.Fatal("Data was not base64-encoded audio")
	}
	if req.FilterPunc == nil || *req.FilterPunc != filterPunc {
		t.Fatal("FilterPunc was not copied")
	}
}

func TestBuildSentenceRecognitionRequestRejectsEmptyAudio(t *testing.T) {
	_, err := buildSentenceRecognitionRequest(tencentASRConfig{}, nil, "audio-key")
	if err == nil {
		t.Fatal("expected empty audio error")
	}
}
